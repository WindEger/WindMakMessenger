package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var JWT_SECRET = "test"
var jwtSecret = []byte(JWT_SECRET)

//var jwtSecret = []byte(os.Getenv("JWT_SECRET")) //[]byte(JWT_SECRET)

var (
	MaxLoginAttempts = 5
	LoginBanDuration = 15 * time.Minute
)

func init() {
	if len(jwtSecret) == 0 {
		log.Fatal("JWT_SECRET environment variable is not set")
	}

	cleanAttempts()
}

func RegisterHandler(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return fmt.Errorf("method not allowed")
	}

	var req struct {
		Login          string `json:"login"`
		Nickname       string `json:"nickname"`
		Password       string `json:"password"`
		RepeatPassword string `json:"repeatPassword"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	if req.Password != req.RepeatPassword {
		return fmt.Errorf("passwords do not match")
	}

	if req.Login == "" {
		return fmt.Errorf("login is required")
	}

	if len(req.Password) < MinPasswordLength {
		return fmt.Errorf("password must be at least %d characters", MinPasswordLength)
	}

	if req.Nickname == "" {
		req.Nickname = req.Login
	}

	_, err := CreateUser(req.Login, req.Nickname, req.Password)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	SendSuccess(w, http.StatusCreated, "User registered successfully", map[string]interface{}{
		"login":    req.Login,
		"nickname": req.Nickname,
	})
	return nil
}

var loginAttempts = sync.Map{}

type attempt struct {
	mu    sync.Mutex
	count int
	last  time.Time
}

func LoginHandler(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return fmt.Errorf("method not allowed")
	}

	var req struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	if req.Login == "" {
		return fmt.Errorf("login is required")
	}

	if val, ok := loginAttempts.Load(req.Login); ok {
		a := val.(*attempt)
		a.mu.Lock()
		count := a.count
		last := a.last
		a.mu.Unlock()

		if count >= MaxLoginAttempts && time.Since(last) < LoginBanDuration {
			return fmt.Errorf("too many failed attempts")
		}
	}

	storedHash, err := GetUserPasswordHash(req.Login)
	if err != nil {
		incrementAttempts(req.Login)
		return fmt.Errorf("invalid login or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(req.Password)); err != nil {
		incrementAttempts(req.Login)
		return fmt.Errorf("invalid login or password")
	}

	loginAttempts.Delete(req.Login)

	user, err := GetUserByLogin(req.Login)
	if err != nil {
		return fmt.Errorf("user not found")
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": user.ID,
		"exp":     time.Now().Add(15 * time.Minute).Unix(),
		"iat":     time.Now().Unix(),
		"iss":     "messenger-app",
		"jti":     uuid.New().String(),
	})

	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		return fmt.Errorf("failed to generate token")
	}

	SendSuccess(w, http.StatusOK, "Login successful", map[string]interface{}{
		"token":    tokenString,
		"login":    user.Login,
		"nickname": user.Nickname,
	})
	return nil
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

func checkTokenAndGetUserIDFromRequest(r *http.Request) (int, error) {
	// Извлекает токен из query параметра или заголовка
	token := r.URL.Query().Get("token")
	if token == "" {
		token = r.Header.Get("Authorization")
	}

	if token == "" {
		return 0, fmt.Errorf("missing token")
	}

	token = strings.TrimPrefix(token, "Bearer ")

	claims, err := ValidateToken(token)
	if err != nil {
		return 0, fmt.Errorf("invalid token: %w", err)
	}

	rawID, ok := claims["user_id"]
	if !ok {
		return 0, fmt.Errorf("invalid token claims: user_id missing")
	}

	userIDFloat, ok := rawID.(float64)
	if !ok {
		return 0, fmt.Errorf("invalid user_id type in token")
	}

	return int(userIDFloat), nil
}
