package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// var JWT_SECRET = "test"
var jwtSecret = []byte(os.Getenv("JWT_SECRET")) //[]byte(JWT_SECRET)

func init() {
	if len(jwtSecret) == 0 {
		log.Fatal("JWT_SECRET environment variable is not set")
	}

	cleanAttempts()
}

func RegisterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Login          string `json:"login"`
		Nickname       string `json:"nickname"`
		Password       string `json:"password"`
		RepeatPassword string `json:"repeatPassword"`
	}

	if req.Password != req.RepeatPassword {
		http.Error(w, "Passwords do not match", http.StatusBadRequest)
		return
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Login == "" {
		http.Error(w, "login is required", http.StatusBadRequest)
		return
	}

	if req.Password == "" {
		http.Error(w, "password is required", http.StatusBadRequest)
		return
	}

	_, err := CreateUser(req.Login, req.Nickname, req.Password)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "User registered successfully",
	})
}

var loginAttempts = sync.Map{}

type attempt struct {
	mu    sync.Mutex
	count int
	last  time.Time
}

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Login == "" {
		http.Error(w, "Login required", http.StatusBadRequest)
		return
	}

	if val, ok := loginAttempts.Load(req.Login); ok {
		a := val.(*attempt)
		a.mu.Lock()
		count := a.count
		last := a.last
		a.mu.Unlock()

		if count >= 5 && time.Since(last) < 15*time.Minute {
			http.Error(w, "Too many attempts for this login", http.StatusTooManyRequests)
			return
		}
	}

	storedHash, err := GetUserPasswordHash(req.Login)
	if err != nil {
		incrementAttempts(req.Login)
		http.Error(w, "Invalid login or password", http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(req.Password)); err != nil {
		incrementAttempts(req.Login)
		http.Error(w, "Invalid login or password", http.StatusUnauthorized)
		return
	}

	loginAttempts.Delete(req.Login)

	user, err := GetUserByLogin(req.Login)
	if err != nil {
		http.Error(w, "Invalid login or password", http.StatusUnauthorized)
		return
	}

	//token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
	//	"user_id": user.ID,
	//	"exp":     time.Now().Add(15 * time.Minute).Unix(),
	//})
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": user.ID,
		"exp":     time.Now().Add(15 * time.Minute).Unix(),
		"iat":     time.Now().Unix(),
		"iss":     "messenger-app",
		"jti":     uuid.New().String(),
	})

	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token": tokenString,
	})
}

func incrementAttempts(login string) {
	val, _ := loginAttempts.LoadOrStore(login, &attempt{count: 0, last: time.Now()})
	a := val.(*attempt)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.count++
	a.last = time.Now()
}

func cleanAttempts() {
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		for range ticker.C {
			loginAttempts.Range(func(key, value interface{}) bool {
				a := value.(*attempt)
				a.mu.Lock()
				shouldDelete := time.Since(a.last) > 1*time.Hour
				a.mu.Unlock()

				if shouldDelete {
					loginAttempts.Delete(key)
				}
				return true
			})
		}
	}()
}

func ValidateToken(tokenString string) (jwt.MapClaims, error) {
	tokenString = strings.TrimPrefix(tokenString, "Bearer ")

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return jwtSecret, nil
	})

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("failed to parse claims")
	}

	return claims, nil
}
