package httpapi

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/kowerkoint/partner-watch/server/internal/store"
)

type fakeImageBackend struct {
	fakeEnroller
	savedData []byte
	taken     store.Image
}

type fakeCaptureBackend struct {
	fakeEnroller
	created   store.CaptureRequest
	completed store.CaptureRequest
}

func (f *fakeCaptureBackend) AuthenticateDevice(_ context.Context, credential string) (string, error) {
	if credential == "requester-token" {
		return "requester", nil
	}
	if credential == "target-token" {
		return "target", nil
	}
	return "", store.ErrUnauthorized
}

func (f *fakeCaptureBackend) CreateCaptureRequest(_ context.Context, requester string) (store.CaptureRequest, error) {
	result := f.created
	result.RequesterDeviceID = requester
	return result, nil
}

func (f *fakeCaptureBackend) CompleteCaptureRequest(_ context.Context, target, requestID, status, imageID, failure string) (store.CaptureRequest, error) {
	result := f.completed
	result.ID, result.TargetDeviceID = requestID, target
	result.Status, result.ImageID, result.Failure = status, imageID, failure
	return result, nil
}

func (f *fakeImageBackend) AuthenticateDevice(_ context.Context, credential string) (string, error) {
	if credential != "valid-credential" {
		return "", store.ErrUnauthorized
	}
	return "device-id", nil
}

func (f *fakeImageBackend) SaveImage(_ context.Context, _ string, data []byte, _, _ int) (store.Image, error) {
	f.savedData = append([]byte(nil), data...)
	return store.Image{ID: "image-id", CreatedAt: time.Unix(1, 0).UTC(), ExpiresAt: time.Unix(3601, 0).UTC()}, nil
}

func (f *fakeImageBackend) TakeImage(_ context.Context, _, imageID string) (store.Image, error) {
	if imageID != "image-id" {
		return store.Image{}, store.ErrImageNotFound
	}
	return f.taken, nil
}

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
	publicKey := testPublicKey(t)
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
	publicKey := testPublicKey(t)
	body := `{"invitationToken":"1234567890123456789012345678901234567890123","deviceName":"Pixel","publicKey":"` + publicKey + `"}`
	request := httptest.NewRequest(http.MethodPost, "/v1/enrollments", strings.NewReader(body))
	response := httptest.NewRecorder()

	NewHandler(fakeEnroller{err: store.ErrInvitationNotFound}).ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func testPublicKey(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	encoded, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func TestMalformedEnrollmentReturnsBadRequest(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/v1/enrollments", strings.NewReader(`{"unknown":true}`))
	response := httptest.NewRecorder()

	NewHandler(fakeEnroller{err: errors.New("must not be called")}).ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestUploadAndDownloadImage(t *testing.T) {
	jpegData := testJPEG(t, 16, 9)
	backend := &fakeImageBackend{taken: store.Image{ID: "image-id", Data: jpegData}}
	upload := httptest.NewRequest(http.MethodPost, "/v1/images", bytes.NewReader(jpegData))
	upload.Header.Set("Authorization", "Bearer valid-credential")
	upload.Header.Set("Content-Type", "image/jpeg")
	uploadResponse := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(uploadResponse, upload)
	if uploadResponse.Code != http.StatusCreated {
		t.Fatalf("upload status = %d; body = %s", uploadResponse.Code, uploadResponse.Body.String())
	}
	if !bytes.Equal(backend.savedData, jpegData) {
		t.Fatal("uploaded JPEG was not passed to storage")
	}

	download := httptest.NewRequest(http.MethodGet, "/v1/images/image-id", nil)
	download.Header.Set("Authorization", "Bearer valid-credential")
	downloadResponse := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(downloadResponse, download)
	if downloadResponse.Code != http.StatusOK || !bytes.Equal(downloadResponse.Body.Bytes(), jpegData) {
		t.Fatalf("download status = %d, bytes = %d", downloadResponse.Code, downloadResponse.Body.Len())
	}
}

func TestImageEndpointsRequireAuthenticationAndJPEG(t *testing.T) {
	backend := &fakeImageBackend{}
	unauthorized := httptest.NewRequest(http.MethodPost, "/v1/images", strings.NewReader("data"))
	unauthorizedResponse := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(unauthorizedResponse, unauthorized)
	if unauthorizedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorizedResponse.Code)
	}

	invalid := httptest.NewRequest(http.MethodPost, "/v1/images", strings.NewReader("not jpeg"))
	invalid.Header.Set("Authorization", "Bearer valid-credential")
	invalid.Header.Set("Content-Type", "image/jpeg")
	invalidResponse := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(invalidResponse, invalid)
	if invalidResponse.Code != http.StatusBadRequest {
		t.Fatalf("invalid JPEG status = %d", invalidResponse.Code)
	}
}

func testJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	bitmap := image.NewRGBA(image.Rect(0, 0, width, height))
	bitmap.Set(0, 0, color.White)
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, bitmap, nil); err != nil {
		t.Fatalf("encode JPEG: %v", err)
	}
	return buffer.Bytes()
}

func TestCaptureRequestIsDeliveredOverWebSocket(t *testing.T) {
	hub := newEventHub()
	backend := &fakeCaptureBackend{created: store.CaptureRequest{
		ID: "request-id", TargetDeviceID: "target", Status: "PENDING",
		CreatedAt: time.Unix(1, 0).UTC(), ExpiresAt: time.Unix(61, 0).UTC(),
	}}
	server := httptest.NewServer(newHandler(backend, hub))
	defer server.Close()

	header := http.Header{"Authorization": []string{"Bearer target-token"}}
	connection, _, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(server.URL, "http")+"/v1/events", &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	defer func() { _ = connection.Close(websocket.StatusNormalClosure, "") }()
	deadline := time.Now().Add(time.Second)
	for !hub.online("target") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/capture-requests", nil)
	request.Header.Set("Authorization", "Bearer requester-token")
	response := httptest.NewRecorder()
	newHandler(backend, hub).ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("capture status = %d; body = %s", response.Code, response.Body.String())
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	var event deviceEvent
	if err := wsjson.Read(ctx, connection, &event); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if event.Type != "capture.requested" || event.RequestID != "request-id" {
		t.Fatalf("event = %+v", event)
	}
}

func TestCaptureResultIsDeliveredToRequester(t *testing.T) {
	hub := newEventHub()
	backend := &fakeCaptureBackend{completed: store.CaptureRequest{RequesterDeviceID: "requester"}}
	channel, unsubscribe := hub.subscribe("requester")
	defer unsubscribe()
	body := strings.NewReader(`{"status":"FAILED","imageId":"","failure":"DISABLED"}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/capture-requests/request-id/result", body)
	request.Header.Set("Authorization", "Bearer target-token")
	response := httptest.NewRecorder()
	newHandler(backend, hub).ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("result status = %d; body = %s", response.Code, response.Body.String())
	}
	select {
	case event := <-channel:
		if event.Type != "capture.completed" || event.Failure != "DISABLED" {
			t.Fatalf("event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("capture result event was not delivered")
	}
}
