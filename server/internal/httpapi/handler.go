package httpapi

import (
	"encoding/json"
	"net/http"
)

type healthResponse struct {
	Status string `json:"status"`
}

func NewHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealth)
	return securityHeaders(mux)
}

func handleHealth(response http.ResponseWriter, _ *http.Request) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(response).Encode(healthResponse{Status: "ok"})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Cache-Control", "no-store")
		response.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(response, request)
	})
}
