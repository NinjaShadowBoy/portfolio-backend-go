package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
)

func (h *api) SubmitContact(c *gin.Context) {
	var req dto.Contact
	if !bindJSON(c, &req) {
		return
	}
	message, err := h.svc.SubmitContact(c.Request.Context(), req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, dto.ContactResponse{Success: true, Message: "Thank you for your message! I'll get back to you soon.", Data: &message})
}

func (h *api) ListContactMessages(c *gin.Context) {
	messages, err := h.svc.ListContactMessages(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, messages)
}

func (h *api) ListUnreadContactMessages(c *gin.Context) {
	messages, err := h.svc.ListUnreadContactMessages(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, messages)
}

func (h *api) MarkContactRead(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	message, err := h.svc.MarkContactRead(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, message)
}

func (h *api) MarkContactReplied(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	message, err := h.svc.MarkContactReplied(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, message)
}

func (h *api) DeleteContactMessage(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteContactMessage(c.Request.Context(), id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
