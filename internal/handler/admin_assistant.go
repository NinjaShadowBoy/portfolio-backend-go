package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/domain"
	"github.com/ninjashadowboy/portfolio-backend-go/internal/middleware"
)

// AdminChatUsage is GET /admin/chat/usage?days=N. Absent days lets the use
// case pick its default; the range check happens there too, so the "days
// must be between 1 and 90" rule has one source of truth.
func (h *api) AdminChatUsage(c *gin.Context) {
	days := 0
	if raw := c.Query("days"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			middleware.WriteError(c, http.StatusBadRequest, "TYPE_MISMATCH", "Invalid days")
			return
		}
		days = value
	}
	report, err := h.svc.ChatUsageReport(c.Request.Context(), days)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, report)
}

func (h *api) AdminKnowledge(c *gin.Context) {
	status, err := h.svc.KnowledgeStatus(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, status)
}

// ReindexKnowledge is POST /admin/knowledge/reindex. A running sync is a 409,
// not an error result; the response body carries the last finished run, whose
// error field (when set) travels as 200.
func (h *api) ReindexKnowledge(c *gin.Context) {
	status, err := h.svc.ReindexKnowledgeNow(c.Request.Context())
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrKnowledgeSyncRunning):
			middleware.WriteError(c, http.StatusConflict, "KNOWLEDGE_SYNC_RUNNING", "A knowledge index sync is already running")
		case errors.Is(err, domain.ErrKnowledgeDisabled):
			middleware.WriteError(c, http.StatusServiceUnavailable, "KNOWLEDGE_DISABLED", "The knowledge index is disabled")
		default:
			writeError(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, status)
}
