package main

import (
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/register", RegisterHandler)
	http.HandleFunc("/login", LoginHandler)

	log.Println("Server started on :50505")
	log.Fatal(http.ListenAndServe(":50505", nil))
}
