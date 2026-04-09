package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/register", HandleErrors(RegisterHandler))
	mux.HandleFunc("/login", HandleErrors(LoginHandler))
	mux.HandleFunc("/ws", WSHandler)
	mux.Handle("/", http.FileServer(http.Dir("./frontend")))

	//port := os.Getenv("PORT")
	port := "50505"
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		log.Println("Server starting on port:", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	gracefulShutdown(server, 5*time.Second)
}

func gracefulShutdown(server *http.Server, shutdownTimeout time.Duration) {
	log.Printf("Shutting down server at %v...", time.Now())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	go startShutdownCountdown(shutdownCtx, shutdownTimeout)

	closeWebSocketConnections()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	closeDatabaseConnection()

	log.Println("Server exited")
}

func startShutdownCountdown(ctx context.Context, timeout time.Duration) {
	for i := int(timeout.Seconds()) - 1; i > 0; i-- {
		select {
		case <-ctx.Done():
			return
		default:
			log.Printf("Shutting down in %d seconds...", i)
			time.Sleep(1 * time.Second)
		}
	}
}

func closeWebSocketConnections() {
	hub.mu.RLock()
	defer hub.mu.RUnlock()

	totalClosed := 0
	for userID, clients := range hub.clients {
		for _, c := range clients {
			err := c.conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseGoingAway, "Server shutdown"))
			if err != nil {
				log.Printf("Failed to send close message to user %d: %v", userID, err)
			}

			err = c.conn.Close()
			if err != nil {
				log.Printf("Failed to close connection for user %d: %v", userID, err)
			}
			totalClosed++
		}
	}

	if totalClosed > 0 {
		log.Printf("Closed %d WebSocket connections", totalClosed)
	}
}

func closeDatabaseConnection() {
	if db != nil {
		if err := db.Close(); err != nil {
			log.Printf("Error closing database connection: %v", err)
		} else {
			log.Println("Database connection closed successfully")
		}
	}
}
