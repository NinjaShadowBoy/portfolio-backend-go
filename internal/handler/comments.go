package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/middleware"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/usecase"
)

func (h *api) ListProjectComments(c *gin.Context) {
	id, ok := parseID(c, "id", "projectId")
	if !ok {
		return
	}
	comments, err := h.svc.ListProjectComments(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, comments)
}

// ListArticleComments serves both /articles/:slug/comments and
// /comments/article/:slug. The slug is a free-form string, so it is read
// directly from the route param rather than through parseID (int64-only).
func (h *api) ListArticleComments(c *gin.Context) {
	slug := c.Param("slug")
	comments, err := h.svc.ListArticleComments(c.Request.Context(), slug)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, comments)
}

func (h *api) CreateComment(c *gin.Context) {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		writeError(c, usecase.ErrUnauthorized)
		return
	}
	var req dto.CommentCreate
	if !bindJSON(c, &req) {
		return
	}
	comment, err := h.svc.CreateComment(c.Request.Context(), identity.UserID, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, comment)
}

func (h *api) UpdateComment(c *gin.Context) {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		writeError(c, usecase.ErrUnauthorized)
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req dto.CommentUpdate
	if !bindJSON(c, &req) {
		return
	}
	comment, err := h.svc.UpdateComment(c.Request.Context(), id, identity.UserID, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, comment)
}

func (h *api) DeleteComment(c *gin.Context) {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		writeError(c, usecase.ErrUnauthorized)
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteComment(c.Request.Context(), id, identity.UserID); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *api) GetMyComments(c *gin.Context) {
	claims := middleware.ClaimsFromContext(c)
	comments, err := h.svc.ListUserComments(c.Request.Context(), claims.UserID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, comments)
}
