package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/dto"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/middleware"
)

func (h *api) GetAdminStats(c *gin.Context) {
	stats, err := h.svc.GetAdminStats(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (h *api) ListUsersForAdmin(c *gin.Context) {
	limit, offset, ok := parsePage(c)
	if !ok {
		return
	}
	page, err := h.svc.ListUsersForAdmin(c.Request.Context(), dto.AdminUserQuery{
		Search:           c.Query("search"),
		IncludeAnonymous: c.Query("includeAnonymous") == "true",
		Limit:            limit,
		Offset:           offset,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, page)
}

func (h *api) UpdateUserRole(c *gin.Context) {
	claims := middleware.ClaimsFromContext(c)
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req dto.UserRoleUpdate
	if !bindJSON(c, &req) {
		return
	}
	user, err := h.svc.UpdateUserRole(c.Request.Context(), claims.UserID, id, req.Role)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, user)
}

func (h *api) DeleteUserAsAdmin(c *gin.Context) {
	claims := middleware.ClaimsFromContext(c)
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteUserAsAdmin(c.Request.Context(), claims.UserID, id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *api) ListCommentsForAdmin(c *gin.Context) {
	limit, offset, ok := parsePage(c)
	if !ok {
		return
	}
	page, err := h.svc.ListCommentsForAdmin(c.Request.Context(), dto.AdminCommentQuery{
		Type:   c.Query("type"),
		Target: c.Query("target"),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, page)
}

func (h *api) DeleteCommentAsAdmin(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeleteCommentAsAdmin(c.Request.Context(), id); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// parsePage reads the optional limit and offset query params. Absent values
// are 0; the use case applies the default page size.
func parsePage(c *gin.Context) (int, int, bool) {
	values := [2]int{}
	for i, name := range []string{"limit", "offset"} {
		raw := c.Query(name)
		if raw == "" {
			continue
		}
		// Bit size 32: the queries take int32, so a larger value must be a 400
		// rather than wrap around in the use case.
		value, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || value < 0 {
			middleware.WriteError(c, http.StatusBadRequest, "TYPE_MISMATCH", "Invalid "+name)
			return 0, 0, false
		}
		values[i] = int(value)
	}
	return values[0], values[1], true
}
