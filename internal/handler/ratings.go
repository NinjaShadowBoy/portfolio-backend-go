package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/middleware"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/usecase"
)

// GetProjectRatings serves both /projects/:id/ratings and /ratings/project/:projectId.
func (h *api) GetProjectRatings(c *gin.Context) {
	id, ok := parseID(c, "id", "projectId")
	if !ok {
		return
	}
	ratings, err := h.svc.GetProjectRatings(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, ratings)
}

func (h *api) GetAverageRating(c *gin.Context) {
	id, ok := parseID(c, "id", "projectId")
	if !ok {
		return
	}
	average, err := h.svc.GetProjectAverageRating(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"averageRating": average})
}

func (h *api) GetRatingCount(c *gin.Context) {
	id, ok := parseID(c, "id", "projectId")
	if !ok {
		return
	}
	count, err := h.svc.GetProjectRatingCount(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": count})
}

func (h *api) GetRatingDistribution(c *gin.Context) {
	id, ok := parseID(c, "id", "projectId")
	if !ok {
		return
	}
	distribution, err := h.svc.GetProjectRatingDistribution(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, distribution)
}

func (h *api) DeleteProjectRatings(c *gin.Context) {
	id, ok := parseID(c, "id", "projectId")
	if !ok {
		return
	}
	if err := h.svc.ResetProjectRatings(c.Request.Context(), id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *api) ListRatings(c *gin.Context) {
	ratings, err := h.svc.ListRatings(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, ratings)
}

func (h *api) GetRating(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	rating, err := h.svc.GetRating(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, rating)
}

func (h *api) GetMyRatings(c *gin.Context) {
	claims := middleware.ClaimsFromContext(c)
	ratings, err := h.svc.GetRatingsByUserID(c.Request.Context(), claims.UserID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, ratings)
}

func (h *api) HasUserRatedProject(c *gin.Context) {
	claims := middleware.ClaimsFromContext(c)
	projectID, ok := parseID(c, "projectId")
	if !ok {
		return
	}
	hasRated, err := h.svc.HasUserRatedProject(c.Request.Context(), claims.UserID, projectID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"hasRated": hasRated})
}

func (h *api) CreateRating(c *gin.Context) {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		writeError(c, usecase.ErrUnauthorized)
		return
	}
	var req dto.RatingCreate
	if !bindJSON(c, &req) {
		return
	}
	rating, err := h.svc.CreateRating(c.Request.Context(), identity.UserID, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, rating)
}

func (h *api) UpdateRating(c *gin.Context) {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		writeError(c, usecase.ErrUnauthorized)
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req dto.RatingUpdate
	if !bindJSON(c, &req) {
		return
	}
	rating, err := h.svc.UpdateRating(c.Request.Context(), id, identity.UserID, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, rating)
}

func (h *api) DeleteRating(c *gin.Context) {
	identity, ok := middleware.IdentityFromContext(c)
	if !ok {
		writeError(c, usecase.ErrUnauthorized)
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteRating(c.Request.Context(), id, identity.UserID); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
