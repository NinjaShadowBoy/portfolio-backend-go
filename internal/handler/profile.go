package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/middleware"
)

func (h *api) UpdateProfile(c *gin.Context) {
	claims := middleware.ClaimsFromContext(c)
	var req dto.ProfileUpdate
	if !bindJSON(c, &req) {
		return
	}
	user, err := h.svc.UpdateProfile(c.Request.Context(), claims.UserID, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, user)
}

func (h *api) ChangePassword(c *gin.Context) {
	claims := middleware.ClaimsFromContext(c)
	var req dto.PasswordChange
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.ChangePassword(c.Request.Context(), claims.UserID, req); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.MessageResponse{Message: "Password updated successfully"})
}

func (h *api) DeleteAccount(c *gin.Context) {
	claims := middleware.ClaimsFromContext(c)
	if err := h.svc.DeleteAccount(c.Request.Context(), claims.UserID); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
