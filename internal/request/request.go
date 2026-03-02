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

// ContentType returns a normalized content type string for the request.
// It returns "json", "multipart", "form", or "unknown".
func ContentType(r *http.Request) string {
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		return "unknown"
	}

	mediaType, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return "unknown"
	}

	switch mediaType {
	case "application/json":
		return "json"
	case "multipart/form-data":
		return "multipart"
	case "application/x-www-form-urlencoded":
		return "form"
	default:
		return "unknown"
	}
}

// ParseJSON decodes a JSON request body into the provided struct pointer.
// It verifies the Content-Type is application/json, rejects empty bodies,
// and returns errors for malformed JSON.
func ParseJSON(r *http.Request, dest interface{}) error {
	if ContentType(r) != "json" {
		return fmt.Errorf("expected application/json content type, got %q", r.Header.Get("Content-Type"))
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("reading request body: %w", err)
	}
	defer func() {
		r.Body = io.NopCloser(bytes.NewReader(body))
	}()

	if len(body) == 0 {
		return fmt.Errorf("empty request body")
	}

	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("decoding JSON: %w", err)
	}
	return nil
}

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

// ParseForm parses the request body into a flat map[string]string.
// For JSON, it unmarshals into map[string]interface{} and converts top-level
// string and numeric values. For form/multipart, it uses r.PostForm.
func ParseForm(r *http.Request) (map[string]string, error) {
	ct := r.Header.Get("Content-Type")

	mediaType := ""
	if ct != "" {
		var err error
		mediaType, _, err = mime.ParseMediaType(ct)
		if err != nil {
			return nil, fmt.Errorf("invalid content type: %w", err)
		}
	}

	switch mediaType {
	case "application/json":
		return parseFormJSON(r)
	case "multipart/form-data":
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			return nil, fmt.Errorf("parsing multipart form: %w", err)
		}
		return flattenPostForm(r), nil
	case "application/x-www-form-urlencoded":
		if err := r.ParseForm(); err != nil {
			return nil, fmt.Errorf("parsing form: %w", err)
		}
		return flattenPostForm(r), nil
	default:
		return nil, fmt.Errorf("unsupported content type: %s", mediaType)
	}
}

// parseFormJSON reads a JSON body into map[string]interface{} and converts
// top-level scalar values to strings, skipping nested objects/arrays.
func parseFormJSON(r *http.Request) (map[string]string, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("reading request body: %w", err)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	if len(body) == 0 {
		return nil, fmt.Errorf("empty request body")
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decoding JSON: %w", err)
	}

	result := make(map[string]string, len(raw))
	for k, v := range raw {
		switch val := v.(type) {
		case string:
			result[k] = val
		case float64:
			// Format without trailing zeros for integers.
			if val == float64(int64(val)) {
				result[k] = strconv.FormatInt(int64(val), 10)
			} else {
				result[k] = strconv.FormatFloat(val, 'f', -1, 64)
			}
		case bool:
			result[k] = strconv.FormatBool(val)
		case nil:
			// skip nil
		default:
			// Skip nested objects/arrays.
		}
	}
	return result, nil
}

// flattenPostForm converts r.PostForm into a flat map[string]string,
// taking the first value of each key.
func flattenPostForm(r *http.Request) map[string]string {
	result := make(map[string]string, len(r.PostForm))
	for k, vals := range r.PostForm {
		if len(vals) > 0 {
			result[k] = vals[0]
		}
	}
	return result
}

// ParseFormValue extracts a single form value by key from any content type.
// For JSON bodies, it reads and parses the body. For form/multipart, it uses
// r.FormValue. Returns empty string if the key is not found or on error.
func ParseFormValue(r *http.Request, key string) string {
	ct := r.Header.Get("Content-Type")

	mediaType := ""
	if ct != "" {
		parsed, _, err := mime.ParseMediaType(ct)
		if err != nil {
			return ""
		}
		mediaType = parsed
	}

	switch mediaType {
	case "application/json":
		return parseFormValueJSON(r, key)
	case "multipart/form-data", "application/x-www-form-urlencoded":
		_ = r.ParseMultipartForm(32 << 20)
		_ = r.ParseForm()
		return r.FormValue(key)
	default:
		// Unknown or missing content type — try FormValue as a fallback.
		_ = r.ParseForm()
		return r.FormValue(key)
	}
}

// parseFormValueJSON reads the JSON body, parses it into map[string]string,
// and returns the value for the given key. It replaces r.Body so it can be
// read again.
func parseFormValueJSON(r *http.Request, key string) string {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return ""
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	if len(body) == 0 {
		return ""
	}

	var raw map[string]string
	if err := json.Unmarshal(body, &raw); err != nil {
		return ""
	}

	return raw[key]
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
