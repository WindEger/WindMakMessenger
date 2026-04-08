package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func HandleErrors(next func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := next(w, r)
		if err != nil {
			log.Printf("Error handling %s %s: %v", r.Method, r.URL.Path, err)
			SendError(w, err.Error(), http.StatusBadRequest)
		}
	}
}

func SendSuccess(w http.ResponseWriter, statusCode int, message string, data interface{}) {
	SendJSONResponse(w, statusCode, OutgoingMessage{
		Success: true,
		Action:  message,
		Data:    data,
	})
}

func SendError(w http.ResponseWriter, message string, statusCode int) {
	SendJSONResponse(w, statusCode, OutgoingMessage{
		Success: false,
		Error:   message,
	})
}

func SendJSONResponse(w http.ResponseWriter, statusCode int, response OutgoingMessage) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
	}
}
