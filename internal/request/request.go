package request

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strconv"
	"strings"
)

// Parse detects the content type from the request and parses the body into dst.
// dst must be a pointer to a struct.
func Parse(r *http.Request, dst interface{}) error {
	if dst == nil {
		return fmt.Errorf("dst must be a non-nil pointer")
	}
	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Ptr {
		return fmt.Errorf("dst must be a pointer, got %T", dst)
	}

	contentType := r.Header.Get("Content-Type")

	// Default to JSON when no Content-Type header is present.
	mediaType := "application/json"
	if contentType != "" {
		var err error
		mediaType, _, err = mime.ParseMediaType(contentType)
		if err != nil {
			return fmt.Errorf("invalid content type: %w", err)
		}
	}

	switch mediaType {
	case "application/json":
		return parseJSON(r, dst)
	case "multipart/form-data":
		return parseMultipartForm(r, dst)
	case "application/x-www-form-urlencoded":
		return parseURLEncodedForm(r, dst)
	default:
		return fmt.Errorf("unsupported content type: %s", mediaType)
	}
}

// ParseFormValue extracts a single form value by key from any content type.
// Returns empty string if the key is not found.
func ParseFormValue(r *http.Request, key string) string {
	_ = r.ParseMultipartForm(32 << 20)
	_ = r.ParseForm()
	return r.FormValue(key)
}

func parseJSON(r *http.Request, dst interface{}) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("reading request body: %w", err)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	if len(body) == 0 {
		return fmt.Errorf("empty request body")
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("decoding JSON: %w", err)
	}
	return nil
}

func parseMultipartForm(r *http.Request, dst interface{}) error {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return fmt.Errorf("parsing multipart form: %w", err)
	}
	return populateStructFromForm(r, dst)
}

func parseURLEncodedForm(r *http.Request, dst interface{}) error {
	if err := r.ParseForm(); err != nil {
		return fmt.Errorf("parsing URL-encoded form: %w", err)
	}
	return populateStructFromForm(r, dst)
}

// populateStructFromForm uses reflection to iterate over the struct fields of
// dst and populate them from form values in the request.
func populateStructFromForm(r *http.Request, dst interface{}) error {
	v := reflect.ValueOf(dst).Elem()
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldVal := v.Field(i)

		if !field.IsExported() {
			continue
		}

		key := formKey(field)

		// Check if the key is present in the form at all.
		_, present := r.Form[key]

		value := r.FormValue(key)

		// If the form value is absent and the field is a pointer type, leave it nil.
		if !present && fieldVal.Kind() == reflect.Ptr {
			continue
		}

		// If the key is not present at all, skip non-pointer fields too
		// (they keep their zero values).
		if !present {
			continue
		}

		if err := setFieldValue(fieldVal, value); err != nil {
			return fmt.Errorf("setting field %s: %w", field.Name, err)
		}
	}

	return nil
}

// formKey determines the form key name for a struct field by checking the form
// tag, then json tag, then falling back to the field name.
func formKey(field reflect.StructField) string {
	if tag := field.Tag.Get("form"); tag != "" {
		name := strings.SplitN(tag, ",", 2)[0]
		if name != "-" {
			return name
		}
	}
	if tag := field.Tag.Get("json"); tag != "" {
		name := strings.SplitN(tag, ",", 2)[0]
		if name != "-" {
			return name
		}
	}
	return field.Name
}

// setFieldValue sets a struct field's value from a string form value,
// handling the supported types.
func setFieldValue(fieldVal reflect.Value, value string) error {
	switch fieldVal.Kind() {
	case reflect.String:
		fieldVal.SetString(value)

	case reflect.Bool:
		fieldVal.SetBool(parseBool(value))

	case reflect.Int:
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parsing int %q: %w", value, err)
		}
		fieldVal.SetInt(int64(n))

	case reflect.Float64:
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("parsing float64 %q: %w", value, err)
		}
		fieldVal.SetFloat(f)

	case reflect.Ptr:
		return setPtrFieldValue(fieldVal, value)
	}

	return nil
}

// setPtrFieldValue handles setting pointer field types (*string, *bool, *int, *float64).
func setPtrFieldValue(fieldVal reflect.Value, value string) error {
	elemType := fieldVal.Type().Elem()

	switch elemType.Kind() {
	case reflect.String:
		fieldVal.Set(reflect.ValueOf(&value))

	case reflect.Bool:
		b := parseBool(value)
		fieldVal.Set(reflect.ValueOf(&b))

	case reflect.Int:
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parsing *int %q: %w", value, err)
		}
		fieldVal.Set(reflect.ValueOf(&n))

	case reflect.Float64:
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("parsing *float64 %q: %w", value, err)
		}
		fieldVal.Set(reflect.ValueOf(&f))
	}

	return nil
}

// parseBool parses a string as a boolean: "true", "yes", "1", and "on" are true;
// everything else is false.
func parseBool(s string) bool {
	switch strings.ToLower(s) {
	case "true", "yes", "1", "on":
		return true
	default:
		return false
	}
}
