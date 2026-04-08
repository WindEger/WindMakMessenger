package main

import (
	"encoding/json"
	"log"
	"net/http"
)

type Response struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func HandleErrors(next func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := next(w, r)
		if err != nil {
			log.Printf("Error handling %s %s: %v", r.Method, r.URL.Path, err)
			SendError(w, http.StatusBadRequest, err.Error())
		}
	}
}

func SendJSONResponse(w http.ResponseWriter, statusCode int, response Response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

func SendSuccess(w http.ResponseWriter, statusCode int, message string, data interface{}) {
	SendJSONResponse(w, statusCode, Response{
		Success: true,
		Message: message,
		Data:    data,
	})
}

func SendError(w http.ResponseWriter, statusCode int, message string) {
	SendJSONResponse(w, statusCode, Response{
		Success: false,
		Error:   message,
	})
}
