package api

import (
	"net/http"
	"sync"

	"github.com/dell-infra-manager/backend/redfish"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

type ActionHandler struct {
	db  *sqlx.DB
	hub *Hub
}

func NewActionHandler(db *sqlx.DB, hub *Hub) *ActionHandler {
	return &ActionHandler{db: db, hub: hub}
}

type powerRequest struct {
	Action string `json:"action" binding:"required"`
}

type bulkPowerRequest struct {
	ServerIDs []string `json:"server_ids" binding:"required"`
	Action    string   `json:"action"     binding:"required"`
}

type bulkResult struct {
	ServerID string `json:"server_id"`
	Name     string `json:"name"`
	OK       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`
}

func (h *ActionHandler) PowerAction(c *gin.Context) {
	var req powerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rt, err := redfish.ValidResetType(req.Action)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	id := c.Param("id")
	client, err := buildClient(h.db, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if err := client.ResetSystem(rt); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	h.hub.Emit("power_action", id, gin.H{"action": req.Action})
	c.JSON(http.StatusAccepted, gin.H{"ok": true, "action": req.Action})
}

func (h *ActionHandler) BulkPowerAction(c *gin.Context) {
	var req bulkPowerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rt, err := redfish.ValidResetType(req.Action)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	results := make([]bulkResult, len(req.ServerIDs))
	var wg sync.WaitGroup
	for i, id := range req.ServerIDs {
		wg.Add(1)
		go func(idx int, serverID string) {
			defer wg.Done()
			s, client, err := loadServerAndClient(h.db, serverID)
			if err != nil {
				results[idx] = bulkResult{ServerID: serverID, Error: err.Error()}
				return
			}
			if err := client.ResetSystem(rt); err != nil {
				results[idx] = bulkResult{ServerID: serverID, Name: s.Name, Error: err.Error()}
				return
			}
			h.hub.Emit("power_action", serverID, gin.H{"action": string(rt)})
			results[idx] = bulkResult{ServerID: serverID, Name: s.Name, OK: true}
		}(i, id)
	}
	wg.Wait()
	c.JSON(http.StatusOK, results)
}
