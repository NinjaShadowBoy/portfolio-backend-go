package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
)

func (h *api) SaveProjectPhoto(c *gin.Context) {
	projectID, ok := parseID(c, "projectId")
	if !ok {
		return
	}
	var req dto.PhotoCreate
	if !bindJSON(c, &req) {
		return
	}
	photo, err := h.svc.SaveProjectPhoto(c.Request.Context(), req.PhotoURL, projectID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, photo)
}

func (h *api) SaveProfilePhoto(c *gin.Context) {
	var req dto.PhotoCreate
	if !bindJSON(c, &req) {
		return
	}
	photo, err := h.svc.SaveProfilePhoto(c.Request.Context(), req.PhotoURL)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, photo)
}

func (h *api) DeletePhoto(c *gin.Context) {
	id, ok := parseID(c, "photoId")
	if !ok {
		return
	}
	if err := h.svc.DeletePhoto(c.Request.Context(), id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
