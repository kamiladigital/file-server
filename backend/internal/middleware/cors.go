package middleware

import (
	"net/http"
)

// ApplyCORS sets necessary CORS headers and handles preflight requests.
// Returns true when the request was a preflight (OPTIONS) and already handled.
func ApplyCORS(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	// expose ETag so browser clients can read it
	w.Header().Set("Access-Control-Expose-Headers", "ETag")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}
