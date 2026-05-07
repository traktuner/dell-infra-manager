package api

import (
	"net/http"
	"sync"

	"github.com/dell-infra-manager/backend/crypto"
	"github.com/dell-infra-manager/backend/models"
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

	s, client, err := h.loadClient(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	if err := client.ResetSystem(rt); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	h.hub.Emit("power_action", s.ID, gin.H{"action": req.Action})
	c.JSON(http.StatusAccepted, gin.H{"ok": true, "action": req.Action})
}

type bulkResult struct {
	ServerID string `json:"server_id"`
	Name     string `json:"name"`
	OK       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`
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
			s, client, err := h.loadClient(serverID)
			if err != nil {
				results[idx] = bulkResult{ServerID: serverID, OK: false, Error: err.Error()}
				return
			}
			if err := client.ResetSystem(rt); err != nil {
				results[idx] = bulkResult{ServerID: serverID, Name: s.Name, OK: false, Error: err.Error()}
				return
			}
			h.hub.Emit("power_action", serverID, gin.H{"action": string(rt)})
			results[idx] = bulkResult{ServerID: serverID, Name: s.Name, OK: true}
		}(i, id)
	}
	wg.Wait()
	c.JSON(http.StatusOK, results)
}

func (h *ActionHandler) loadClient(serverID string) (*models.Server, *redfish.Client, error) {
	var s models.Server
	if err := h.db.Get(&s, `SELECT * FROM servers WHERE id = ?`, serverID); err != nil {
		return nil, nil, err
	}
	password, err := crypto.Decrypt(s.Password)
	if err != nil {
		return nil, nil, err
	}
	return &s, redfish.NewClient(s.Hostname, s.Port, s.Username, password, s.TLSVerify), nil
}
