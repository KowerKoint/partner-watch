package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/kowerkoint/partner-watch/server/internal/store"
)

type healthResponse struct {
	Status string `json:"status"`
}

type Enroller interface {
	EnrollDevice(ctx context.Context, invitationToken, deviceName, publicKey string) (store.Enrollment, error)
}

type enrollmentRequest struct {
	InvitationToken string `json:"invitationToken"`
	DeviceName      string `json:"deviceName"`
	PublicKey       string `json:"publicKey"`
}

type enrollmentResponse struct {
	DeviceID   string `json:"deviceId"`
	PairID     string `json:"pairId"`
	Slot       int    `json:"slot"`
	Credential string `json:"credential"`
}

type errorResponse struct {
	Code string `json:"code"`
}

func NewHandler(enroller Enroller) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("POST /v1/enrollments", handleEnrollment(enroller))
	return securityHeaders(mux)
}

func handleHealth(response http.ResponseWriter, _ *http.Request) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(response).Encode(healthResponse{Status: "ok"})
}

func handleEnrollment(enroller Enroller) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if enroller == nil {
			writeError(response, http.StatusInternalServerError, "internal_error")
			return
		}
		request.Body = http.MaxBytesReader(response, request.Body, 16*1024)
		decoder := json.NewDecoder(request.Body)
		decoder.DisallowUnknownFields()

		var body enrollmentRequest
		if err := decoder.Decode(&body); err != nil {
			writeError(response, http.StatusBadRequest, "invalid_request")
			return
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			writeError(response, http.StatusBadRequest, "invalid_request")
			return
		}
		body.DeviceName = strings.TrimSpace(body.DeviceName)
		if !validEnrollment(body) {
			writeError(response, http.StatusBadRequest, "invalid_request")
			return
		}

		enrollment, err := enroller.EnrollDevice(
			request.Context(),
			body.InvitationToken,
			body.DeviceName,
			body.PublicKey,
		)
		if errors.Is(err, store.ErrInvitationNotFound) {
			http.NotFound(response, request)
			return
		}
		if err != nil {
			writeError(response, http.StatusInternalServerError, "internal_error")
			return
		}

		writeJSON(response, http.StatusCreated, enrollmentResponse{
			DeviceID:   enrollment.DeviceID,
			PairID:     enrollment.PairID,
			Slot:       enrollment.Slot,
			Credential: enrollment.Credential,
		})
	}
}

func validEnrollment(request enrollmentRequest) bool {
	if len(request.InvitationToken) < 43 || len(request.InvitationToken) > 128 {
		return false
	}
	if !utf8.ValidString(request.DeviceName) || utf8.RuneCountInString(request.DeviceName) < 1 || utf8.RuneCountInString(request.DeviceName) > 80 {
		return false
	}
	publicKey, err := base64.RawURLEncoding.DecodeString(request.PublicKey)
	return err == nil && len(publicKey) == 32
}

func writeError(response http.ResponseWriter, status int, code string) {
	writeJSON(response, status, errorResponse{Code: code})
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Cache-Control", "no-store")
		response.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(response, request)
	})
}
