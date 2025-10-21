package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"example.com/user2025/internal/auth"
	"example.com/user2025/internal/database"
	"example.com/user2025/internal/models"
)

type AuthHandler struct {
	users     database.UserRepository
	jwtSecret string
	tokenTTL  time.Duration
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
}

type AuthResponse struct {
	Token string            `json:"token"`
	User  models.PublicUser `json:"user"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func NewAuthHandler(users database.UserRepository, secret string) *AuthHandler {
	return &AuthHandler{users: users, jwtSecret: secret, tokenTTL: 24 * time.Hour}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.FullName = strings.TrimSpace(req.FullName)

	if req.Email == "" || req.Password == "" || req.FullName == "" {
		WriteError(w, http.StatusBadRequest, "email, password and full_name are required")
		return
	}

	if len(req.Password) < 8 {
		WriteError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	user := &models.User{
		Email:        req.Email,
		FullName:     req.FullName,
		PasswordHash: hash,
	}

	if err := h.users.CreateUser(r.Context(), user); err != nil {
		if errors.Is(err, database.ErrDuplicate) {
			WriteError(w, http.StatusConflict, "email already registered")
			return
		}
		WriteError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	token, err := h.generateToken(user)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}

	WriteJSON(w, http.StatusCreated, AuthResponse{Token: token, User: user.Public()})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" || req.Password == "" {
		WriteError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	user, err := h.users.GetUserByEmail(r.Context(), email)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			WriteError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		WriteError(w, http.StatusInternalServerError, "failed to load user")
		return
	}

	if !auth.VerifyPassword(user.PasswordHash, req.Password) {
		WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := h.generateToken(user)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}

	WriteJSON(w, http.StatusOK, AuthResponse{Token: token, User: user.Public()})
}

func (h *AuthHandler) Profile(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		WriteError(w, http.StatusUnauthorized, "missing token")
		return
	}

	user, err := h.users.GetUserByID(r.Context(), claims.UserID)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "user not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "failed to load profile")
		return
	}

	WriteJSON(w, http.StatusOK, user.Public())
}

func (h *AuthHandler) generateToken(user *models.User) (string, error) {
	now := time.Now().UTC()
	claims := auth.Claims{
		UserID:   user.ID,
		Email:    user.Email,
		IssuedAt: now.Unix(),
		Expires:  now.Add(h.tokenTTL).Unix(),
	}
	return auth.GenerateToken(h.jwtSecret, claims)
}

func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]string{"error": message})
}
