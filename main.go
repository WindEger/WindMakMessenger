package main

import (
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/register", HandleErrors(RegisterHandler))
	http.HandleFunc("/login", HandleErrors(LoginHandler))
	http.HandleFunc("/ws", WSHandler)

	http.Handle("/", http.FileServer(http.Dir("./frontend")))

	log.Println("Server started on :50505")
	log.Fatal(http.ListenAndServe(":50505", nil))
}
