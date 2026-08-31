package httpapi

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kowerkoint/partner-watch/server/internal/store"
)

type fakeEnroller struct {
	result store.Enrollment
	err    error
}

func (f fakeEnroller) EnrollDevice(context.Context, string, string, string) (store.Enrollment, error) {
	return f.result, f.err
}

func TestHealth(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	NewHandler(nil).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if got := response.Body.String(); got != "{\"status\":\"ok\"}\n" {
		t.Fatalf("body = %q", got)
	}
}

func TestUnknownRouteDoesNotExposeDetails(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	response := httptest.NewRecorder()

	NewHandler(nil).ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestEnrollDevice(t *testing.T) {
	publicKey := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	body := `{"invitationToken":"1234567890123456789012345678901234567890123","deviceName":"Pixel 8a","publicKey":"` + publicKey + `"}`
	request := httptest.NewRequest(http.MethodPost, "/v1/enrollments", strings.NewReader(body))
	response := httptest.NewRecorder()
	enroller := fakeEnroller{result: store.Enrollment{
		DeviceID: "device-id", PairID: "pair-id", Slot: 1, Credential: "credential",
	}}

	NewHandler(enroller).ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if got := response.Body.String(); got != "{\"deviceId\":\"device-id\",\"pairId\":\"pair-id\",\"slot\":1,\"credential\":\"credential\"}\n" {
		t.Fatalf("body = %q", got)
	}
}

func TestInvalidInvitationReturnsNotFound(t *testing.T) {
	publicKey := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	body := `{"invitationToken":"1234567890123456789012345678901234567890123","deviceName":"Pixel","publicKey":"` + publicKey + `"}`
	request := httptest.NewRequest(http.MethodPost, "/v1/enrollments", strings.NewReader(body))
	response := httptest.NewRecorder()

	NewHandler(fakeEnroller{err: store.ErrInvitationNotFound}).ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestMalformedEnrollmentReturnsBadRequest(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/v1/enrollments", strings.NewReader(`{"unknown":true}`))
	response := httptest.NewRecorder()

	NewHandler(fakeEnroller{err: errors.New("must not be called")}).ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}
