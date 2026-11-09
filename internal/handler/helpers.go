package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/middleware"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/sanitize"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/usecase"
)

func parseID(c *gin.Context, names ...string) (int64, bool) {
	raw := ""
	for _, name := range names {
		if value := c.Param(name); value != "" {
			raw = value
			break
		}
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		middleware.WriteError(c, http.StatusBadRequest, "TYPE_MISMATCH", "Invalid ID")
		return 0, false
	}
	return id, true
}

// maxBodyBytes caps a JSON request body; no endpoint takes anything near it.
const maxBodyBytes = 1 << 20

func init() {
	// maxbytes limits a string by bytes rather than runes. bcrypt rejects
	// passwords over 72 bytes, so the rune-based max tag is not enough.
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		_ = v.RegisterValidation("maxbytes", func(fl validator.FieldLevel) bool {
			limit, err := strconv.Atoi(fl.Param())
			return err == nil && len(fl.Field().String()) <= limit
		})
		// http_url_or_empty accepts "" so an empty string can clear a link. With
		// omitempty, a non-nil *string holding "" still runs the tags after it,
		// so http_url alone would reject the clear.
		_ = v.RegisterValidation("http_url_or_empty", func(fl validator.FieldLevel) bool {
			value := fl.Field().String()
			return value == "" || v.Var(value, "http_url") == nil
		})
	}
}

// bindJSON decodes the request body into dst, sanitizes the fields tagged
// `sanitize`, then validates the `binding` tags. Sanitizing first means a
// value that is only whitespace or markup fails `required`. gin's own JSON
// binding validates while it decodes, so it cannot be used here.
func bindJSON(c *gin.Context, dst any) bool { return bindJSONLimit(c, dst, maxBodyBytes) }

// bindJSONLimit is bindJSON with the body cap the endpoint needs.
func bindJSONLimit(c *gin.Context, dst any, limit int64) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, limit))
	if binding.EnableDecoderDisallowUnknownFields {
		decoder.DisallowUnknownFields()
	}
	err := decoder.Decode(dst)
	if err == nil {
		sanitize.Struct(dst)
		err = binding.Validator.ValidateStruct(dst)
	}
	if err != nil {
		middleware.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
		return false
	}
	return true
}

func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		middleware.WriteError(c, http.StatusNotFound, "RESOURCE_NOT_FOUND", "Resource not found")
	case errors.Is(err, usecase.ErrUnauthorized):
		middleware.WriteError(c, http.StatusUnauthorized, "AUTHENTICATION_FAILED", "Authentication failed")
	case errors.Is(err, usecase.ErrForbidden):
		middleware.WriteError(c, http.StatusForbidden, "ACCESS_DENIED", "Access denied")
	case errors.Is(err, usecase.ErrConflict) || errors.Is(err, usecase.ErrAlreadyExists):
		middleware.WriteError(c, http.StatusConflict, "RESOURCE_ALREADY_EXISTS", "Resource already exists")
	case errors.Is(err, usecase.ErrBadRequest):
		middleware.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		middleware.WriteError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
	}
}
