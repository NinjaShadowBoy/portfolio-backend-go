package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
)

func (h *api) ListProjects(c *gin.Context) {
	projects, err := h.svc.ListProjects(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, projects)
}

func (h *api) GetProject(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	project, err := h.svc.GetProject(c.Request.Context(), id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, project)
}

func (h *api) CreateProject(c *gin.Context) {
	var req dto.ProjectCreate
	if !bindJSON(c, &req) {
		return
	}
	project, err := h.svc.CreateProject(c.Request.Context(), req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, project)
}

func (h *api) UpdateProject(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req dto.ProjectUpdate
	if !bindJSON(c, &req) {
		return
	}
	project, err := h.svc.UpdateProject(c.Request.Context(), id, req)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, project)
}

func (h *api) DeleteProject(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteProject(c.Request.Context(), id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *api) ReorderProjects(c *gin.Context) {
	var req dto.ProjectOrder
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.ReorderProjects(c.Request.Context(), req.IDs); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
