package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/student-learning-portal/backend/internal/domain"
	"github.com/student-learning-portal/backend/internal/usecase"
)

const maxAvatarBytes = 5 << 20 // 5 MB

var allowedAvatarTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

type ProfileHandler struct {
	authUseCase *usecase.AuthUseCase
	uploadsDir  string
}

func NewProfileHandler(uc *usecase.AuthUseCase, uploadsDir string) *ProfileHandler {
	return &ProfileHandler{authUseCase: uc, uploadsDir: uploadsDir}
}

// PatchEmail handles PATCH /api/v1/users/me/email.
// Requires current_password to prevent account takeover.
func (h *ProfileHandler) PatchEmail(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewEmail        string `json:"new_email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CurrentPassword == "" || req.NewEmail == "" {
		writeError(w, http.StatusBadRequest, "current_password and new_email are required")
		return
	}

	user, err := h.authUseCase.ChangeEmail(r.Context(), claims.UserID, req.CurrentPassword, req.NewEmail)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidLogin):
			writeError(w, http.StatusUnauthorized, "current password is incorrect")
		case errors.Is(err, usecase.ErrValidation):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, domain.ErrEmailTaken):
			writeError(w, http.StatusConflict, "email already registered")
		default:
			writeError(w, http.StatusInternalServerError, "failed to update email")
		}
		return
	}
	writeJSON(w, http.StatusOK, toUserPayload(user))
}

// PatchPassword handles PATCH /api/v1/users/me/password.
// Requires current_password; returns 204 on success.
func (h *ProfileHandler) PatchPassword(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "current_password and new_password are required")
		return
	}

	err := h.authUseCase.ChangePassword(r.Context(), claims.UserID, req.CurrentPassword, req.NewPassword)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidLogin):
			writeError(w, http.StatusUnauthorized, "current password is incorrect")
		case errors.Is(err, usecase.ErrValidation):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "failed to update password")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PatchName handles PATCH /api/v1/users/me/name.
func (h *ProfileHandler) PatchName(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}

	var req struct {
		FullName string `json:"full_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.FullName == "" {
		writeError(w, http.StatusBadRequest, "full_name is required")
		return
	}

	user, err := h.authUseCase.ChangeName(r.Context(), claims.UserID, req.FullName)
	if err != nil {
		if errors.Is(err, usecase.ErrValidation) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update name")
		return
	}
	writeJSON(w, http.StatusOK, toUserPayload(user))
}

// PostAvatar handles POST /api/v1/users/me/avatar.
// Accepts multipart/form-data with an "avatar" file field (max 5 MB).
// Validates the MIME type, saves the file to uploadsDir/avatars/, and stores
// the server-relative URL in the database.
func (h *ProfileHandler) PostAvatar(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing authentication")
		return
	}

	if err := r.ParseMultipartForm(maxAvatarBytes); err != nil { //nolint:gosec // size is bounded by maxAvatarBytes constant
		writeError(w, http.StatusBadRequest, "file too large or invalid multipart form")
		return
	}

	file, _, err := r.FormFile("avatar")
	if err != nil {
		writeError(w, http.StatusBadRequest, "avatar file is required")
		return
	}
	defer file.Close()

	// Read a small header to detect the real MIME type, then reassemble the
	// stream so the full file is still available for writing.
	header := make([]byte, 512) //nolint:mnd // 512 is the minimum for http.DetectContentType
	n, err := file.Read(header)
	if err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "could not read file")
		return
	}
	mimeType := http.DetectContentType(header[:n])
	ext, ok := allowedAvatarTypes[mimeType]
	if !ok {
		writeError(w, http.StatusUnsupportedMediaType,
			fmt.Sprintf("unsupported image type %q; allowed: jpeg, png, gif, webp", mimeType))
		return
	}

	destDir := filepath.Join(h.uploadsDir, "avatars")
	if err = os.MkdirAll(destDir, 0o755); err != nil { //nolint:mnd // 0755 = rwxr-xr-x, standard directory permission
		writeError(w, http.StatusInternalServerError, "failed to prepare upload directory")
		return
	}

	filename := claims.UserID + ext
	destPath := filepath.Join(destDir, filename) //nolint:gosec // filename is {uuid}.{ext}, no user-controlled path components
	out, err := os.Create(destPath)              //nolint:gosec // same as above
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save file")
		return
	}
	defer out.Close()

	// Write the already-read header bytes, then stream the rest.
	if _, err = out.Write(header[:n]); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save file")
		return
	}
	if _, err = io.Copy(out, file); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save file")
		return
	}

	avatarURL := "/uploads/avatars/" + filename
	user, err := h.authUseCase.ChangeAvatar(r.Context(), claims.UserID, avatarURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update avatar")
		return
	}
	writeJSON(w, http.StatusOK, toUserPayload(user))
}
