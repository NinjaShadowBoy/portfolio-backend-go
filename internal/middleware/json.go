package middleware

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
)

func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func DecodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst)
}

func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, dto.ErrorResponse{
		Timestamp: time.Now(),
		Status:    status,
		Error:     http.StatusText(status),
		Code:      code,
		Message:   message,
	})
}
