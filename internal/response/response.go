package response

import (
	"encoding/json"
	"net/http"
)

// RespondJSON writes data as JSON with the given status code.
func RespondJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(data)
}

// RespondError writes an error message as JSON with the given status code.
func RespondError(w http.ResponseWriter, statusCode int, message string) {
	RespondJSON(w, statusCode, map[string]string{"message": message})
}

// RespondSuccess writes a success message as JSON with a 200 status code.
func RespondSuccess(w http.ResponseWriter, message string) {
	RespondJSON(w, http.StatusOK, map[string]string{"message": message})
}
