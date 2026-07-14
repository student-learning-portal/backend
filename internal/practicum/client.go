// Package practicum is the integration client for the practicum team's
// SEHRIYO backend (course ratings/comments). We deliberately do not
// reimplement their enrollment- and progress-gated review logic here — every
// call in this package proxies to their already-running API, which is the
// source of truth. See ReviewRepository for how a course gets mirrored so the
// two independently-generated course IDs can be linked.
package practicum

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/student-learning-portal/backend/internal/domain"
)

const requestTimeout = 5 * time.Second

// mintedTokenTTL only needs to survive a single outgoing request — these
// tokens are minted fresh for every call, never stored or reused.
const mintedTokenTTL = 1 * time.Minute

// Client is a thin HTTP client for the practicum-team service's course API.
// Authentication relies on a JWT_SECRET shared between the two independently
// written services: Client mints its own short-lived tokens rather than
// running their login flow, since the only trust mechanism on their side is
// HMAC signature + a {user_id, role} claim shape (see claims below).
type Client struct {
	baseURL    string
	httpClient *http.Client
	secret     []byte
}

// NewClient builds a Client. baseURL is the practicum-team API's root, e.g.
// "http://practicum-backend:8080/api/v1" locally or their deployed server in
// prod. jwtSecret must equal the value their service's own JWT_SECRET is
// configured with, or every call will 401.
func NewClient(baseURL, jwtSecret string) *Client {
	return &Client{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{Timeout: requestTimeout},
		secret:     []byte(jwtSecret),
	}
}

// claims matches the practicum team's pkg/jwt.Claims shape exactly (json
// tag "user_id", not "sub") — see practicum_project-main/pkg/jwt/jwt.go.
type claims struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

func (c *Client) mintToken(userID, role string) (string, error) {
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(mintedTokenTTL)),
		},
	})
	signed, err := tok.SignedString(c.secret)
	if err != nil {
		return "", fmt.Errorf("mint practicum token: %w", err)
	}
	return signed, nil
}

// errorDetail / errorResponse match their internal/transport/http/courses.ErrorResponse.
type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorResponse struct {
	Error errorDetail `json:"error"`
}

// mapError translates one of their error codes (internal/transport/http/courses/errors.go)
// into the matching domain sentinel, so callers can errors.Is against the same
// errors they'd get from a local repository.
func mapError(status int, body []byte) error {
	var er errorResponse
	if err := json.Unmarshal(body, &er); err != nil || er.Error.Code == "" {
		return fmt.Errorf("practicum: unexpected %d response: %s", status, strings.TrimSpace(string(body)))
	}
	switch er.Error.Code {
	case "COURSE_NOT_FOUND", "INVALID_COURSE_ID", "TEACHER_NOT_FOUND":
		return domain.ErrCourseNotFound
	case "NOT_ENROLLED":
		return domain.ErrNotEnrolled
	case "INSUFFICIENT_PROGRESS":
		return domain.ErrInsufficientProgress
	case "COMMENT_ALREADY_EXISTS":
		return domain.ErrReviewAlreadyExists
	case "INVALID_RATING", "COMMENT_TEXT_TOO_LONG", "BAD_REQUEST":
		return domain.ErrInvalidReview
	default:
		return fmt.Errorf("practicum: %s: %s", er.Error.Code, er.Error.Message)
	}
}

// do executes a JSON request against the practicum service. token is omitted
// (no Authorization header) when empty, matching the rating endpoint's public
// access. out is left untouched for 204/empty responses.
func (c *Client) do(ctx context.Context, method, path, token string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("practicum: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("practicum: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("practicum: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("practicum: read response: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return mapError(resp.StatusCode, respBody)
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("practicum: decode response: %w", err)
	}
	return nil
}

var errMissingIntegrationTeacher = errors.New("practicum: PRACTICUM_INTEGRATION_TEACHER_ID is not configured")
