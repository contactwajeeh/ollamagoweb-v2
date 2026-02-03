package main

import (
	"encoding/json"
	"net/http"
)

// WriteError writes a consistent JSON error response
func WriteError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error":   true,
		"message": message,
	})
}

// WriteJSON writes a consistent JSON success response
func WriteJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
