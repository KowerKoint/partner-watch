package httpapi

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kowerkoint/partner-watch/server/internal/store"
)

type healthResponse struct {
	Status string `json:"status"`
}

type Enroller interface {
	EnrollDevice(ctx context.Context, invitationToken, deviceName, publicKey string) (store.Enrollment, error)
}

type ImageStore interface {
	AuthenticateDevice(ctx context.Context, credential string) (string, error)
	SaveImage(ctx context.Context, deviceID string, data []byte, width, height int) (store.Image, error)
	TakeImage(ctx context.Context, deviceID, imageID string) (store.Image, error)
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

type imageResponse struct {
	ImageID   string    `json:"imageId"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

const maxImageBytes = 10 * 1024 * 1024
const maxImagePixels = 5_000_000

func NewHandler(enroller Enroller) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealth)
	mux.HandleFunc("POST /v1/enrollments", handleEnrollment(enroller))
	if images, ok := enroller.(ImageStore); ok {
		mux.HandleFunc("POST /v1/images", authenticate(images, handleImageUpload(images)))
		mux.HandleFunc("GET /v1/images/{imageID}", authenticate(images, handleImageDownload(images)))
	}
	return securityHeaders(mux)
}

type authenticatedHandler func(http.ResponseWriter, *http.Request, string)

func authenticate(images ImageStore, next authenticatedHandler) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		authorization := request.Header.Get("Authorization")
		if !strings.HasPrefix(authorization, "Bearer ") || strings.Contains(strings.TrimPrefix(authorization, "Bearer "), " ") {
			writeError(response, http.StatusUnauthorized, "unauthorized")
			return
		}
		deviceID, err := images.AuthenticateDevice(request.Context(), strings.TrimPrefix(authorization, "Bearer "))
		if errors.Is(err, store.ErrUnauthorized) {
			writeError(response, http.StatusUnauthorized, "unauthorized")
			return
		}
		if err != nil {
			writeError(response, http.StatusInternalServerError, "internal_error")
			return
		}
		next(response, request, deviceID)
	}
}

func handleImageUpload(images ImageStore) authenticatedHandler {
	return func(response http.ResponseWriter, request *http.Request, deviceID string) {
		if request.Header.Get("Content-Type") != "image/jpeg" {
			writeError(response, http.StatusUnsupportedMediaType, "jpeg_required")
			return
		}
		data, err := io.ReadAll(io.LimitReader(request.Body, maxImageBytes+1))
		if err != nil || len(data) == 0 || len(data) > maxImageBytes {
			writeError(response, http.StatusRequestEntityTooLarge, "image_too_large")
			return
		}
		config, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || format != "jpeg" || config.Width <= 0 || config.Height <= 0 || config.Width > maxImagePixels/config.Height {
			writeError(response, http.StatusBadRequest, "invalid_image")
			return
		}
		stored, err := images.SaveImage(request.Context(), deviceID, data, config.Width, config.Height)
		if err != nil {
			writeError(response, http.StatusInternalServerError, "internal_error")
			return
		}
		writeJSON(response, http.StatusCreated, imageResponse{ImageID: stored.ID, CreatedAt: stored.CreatedAt, ExpiresAt: stored.ExpiresAt})
	}
}

func handleImageDownload(images ImageStore) authenticatedHandler {
	return func(response http.ResponseWriter, request *http.Request, deviceID string) {
		imageID := request.PathValue("imageID")
		if imageID == "" || strings.ContainsAny(imageID, "/\\") {
			http.NotFound(response, request)
			return
		}
		stored, err := images.TakeImage(request.Context(), deviceID, imageID)
		if errors.Is(err, store.ErrImageNotFound) {
			http.NotFound(response, request)
			return
		}
		if err != nil {
			writeError(response, http.StatusInternalServerError, "internal_error")
			return
		}
		response.Header().Set("Content-Type", "image/jpeg")
		response.Header().Set("Content-Length", fmt.Sprint(len(stored.Data)))
		response.WriteHeader(http.StatusOK)
		_, _ = response.Write(stored.Data)
	}
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
	publicKeyBytes, err := base64.RawURLEncoding.DecodeString(request.PublicKey)
	if err != nil {
		return false
	}
	parsed, err := x509.ParsePKIXPublicKey(publicKeyBytes)
	publicKey, ok := parsed.(*ecdsa.PublicKey)
	return err == nil && ok && publicKey.Curve == elliptic.P256()
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
