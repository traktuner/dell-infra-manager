package api

import (
	"net/http"
	"strconv"

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

	client, err := buildClient(h.db, id)
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
	client, err := buildClient(h.db, c.Param("id"))
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
