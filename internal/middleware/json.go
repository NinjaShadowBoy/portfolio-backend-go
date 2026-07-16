package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
)

func WriteError(c *gin.Context, status int, code, message string) {
	c.JSON(status, dto.ErrorResponse{
		Timestamp: time.Now(),
		Status:    status,
		Error:     http.StatusText(status),
		Code:      code,
		Message:   message,
	})
}
