package api

import (
	"net/http"
	"strconv"

	"github.com/dell-manager/backend/crypto"
	"github.com/dell-manager/backend/models"
	"github.com/dell-manager/backend/redfish"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

type EventLogHandler struct {
	db *sqlx.DB
}

func NewEventLogHandler(db *sqlx.DB) *EventLogHandler {
	return &EventLogHandler{db: db}
}

func (h *EventLogHandler) GetLog(c *gin.Context) {
	id := c.Param("id")
	top, _ := strconv.Atoi(c.DefaultQuery("top", "100"))
	skip, _ := strconv.Atoi(c.DefaultQuery("skip", "0"))

	client, err := h.buildClient(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}
	entries, err := client.GetEventLog(top, skip)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, entries)
}

func (h *EventLogHandler) ClearLog(c *gin.Context) {
	id := c.Param("id")
	client, err := h.buildClient(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}
	if err := client.ClearEventLog(); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *EventLogHandler) buildClient(serverID string) (*redfish.Client, error) {
	var s models.Server
	if err := h.db.Get(&s, `SELECT * FROM servers WHERE id = ?`, serverID); err != nil {
		return nil, err
	}
	password, err := crypto.Decrypt(s.Password)
	if err != nil {
		return nil, err
	}
	return redfish.NewClient(s.Hostname, s.Port, s.Username, password, s.TLSVerify), nil
}
