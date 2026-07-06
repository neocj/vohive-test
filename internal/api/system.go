package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func disabledSystemEndpoint(c *gin.Context, code string, message string) {
	c.JSON(http.StatusGone, gin.H{
		"status":     "error",
		"code":       code,
		"message":    message,
		"request_id": requestID(c),
	})
}

func (s *Server) handleCheckUpdate(c *gin.Context) {
	disabledSystemEndpoint(
		c,
		"online_update_disabled",
		"Online self-update is disabled in this build. Build and deploy a new container image instead.",
	)
}

func (s *Server) handleApplyUpdate(c *gin.Context) {
	disabledSystemEndpoint(
		c,
		"online_update_disabled",
		"Online self-update is disabled in this build. The service will never download a remote binary or replace itself.",
	)
}

func (s *Server) handleUninstall(c *gin.Context) {
	disabledSystemEndpoint(
		c,
		"uninstall_disabled",
		"Self-uninstall is disabled in this build. Configuration, data, logs, and executable files are left untouched.",
	)
}
