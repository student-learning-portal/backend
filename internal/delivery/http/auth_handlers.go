package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/usecase"
)

type AuthHandler struct {
	authUseCase *usecase.AuthUseCase
}

func NewAuthHandler(uc *usecase.AuthUseCase) *AuthHandler {
	return &AuthHandler{authUseCase: uc}
}

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	FullName    string `json:"full_name"`
	Role        string `json:"role"`
	AnonymousID string `json:"anonymous_id,omitempty"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	Token string     `json:"token"`
	User  userPayload `json:"user"`
}

type userPayload struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Role     string `json:"role"`
}

func toUserPayload(u domain.User) userPayload {
	return userPayload{ID: u.ID, Email: u.Email, FullName: u.FullName, Role: string(u.Role)}
}

// Register handles POST /api/v1/auth/register
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	token, user, err := h.authUseCase.Register(domain.RegisterInput{
		Email:       req.Email,
		Password:    req.Password,
		FullName:    req.FullName,
		Role:        domain.Role(req.Role),
		AnonymousID: req.AnonymousID,
	})
	if err != nil {
		switch {
		case errors.Is(err, usecase.ErrValidation):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, domain.ErrEmailTaken):
			writeError(w, http.StatusConflict, "email already registered")
		default:
			writeError(w, http.StatusInternalServerError, "failed to register")
		}
		return
	}

	writeJSON(w, http.StatusCreated, authResponse{Token: token, User: toUserPayload(user)})
}

// Login handles POST /api/v1/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	token, user, err := h.authUseCase.Login(domain.LoginInput{Email: req.Email, Password: req.Password})
	if err != nil {
		if errors.Is(err, domain.ErrInvalidLogin) {
			writeError(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to login")
		return
	}

	writeJSON(w, http.StatusOK, authResponse{Token: token, User: toUserPayload(user)})
}

// Me handles GET /api/v1/auth/me — requires RequireAuth middleware.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}

	user, err := h.authUseCase.CurrentUser(claims)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	writeJSON(w, http.StatusOK, toUserPayload(user))
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": strings.TrimSpace(message)})
}
