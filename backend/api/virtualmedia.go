package api

import (
	"net/http"

	"github.com/dell-infra-manager/backend/crypto"
	"github.com/dell-infra-manager/backend/models"
	"github.com/dell-infra-manager/backend/redfish"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

type VirtualMediaHandler struct {
	db  *sqlx.DB
	hub *Hub
}

func NewVirtualMediaHandler(db *sqlx.DB, hub *Hub) *VirtualMediaHandler {
	return &VirtualMediaHandler{db: db, hub: hub}
}

func (h *VirtualMediaHandler) GetStatus(c *gin.Context) {
	client, err := h.buildClient(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}
	media, err := client.GetVirtualMedia()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, media)
}

type insertRequest struct {
	ImageURL string `json:"image_url" binding:"required"`
	Slot     string `json:"slot"` // default "CD"
}

func (h *VirtualMediaHandler) Insert(c *gin.Context) {
	var req insertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Slot == "" {
		req.Slot = "CD"
	}

	client, err := h.buildClient(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}
	if err := client.InsertVirtualMedia(req.Slot, req.ImageURL); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	h.hub.Emit("virtualmedia", c.Param("id"), gin.H{"inserted": true, "image": req.ImageURL, "slot": req.Slot})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type ejectRequest struct {
	Slot string `json:"slot"` // default "CD"
}

func (h *VirtualMediaHandler) Eject(c *gin.Context) {
	var req ejectRequest
	c.ShouldBindJSON(&req)
	if req.Slot == "" {
		req.Slot = "CD"
	}

	client, err := h.buildClient(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}
	if err := client.EjectVirtualMedia(req.Slot); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	h.hub.Emit("virtualmedia", c.Param("id"), gin.H{"inserted": false, "slot": req.Slot})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *VirtualMediaHandler) buildClient(serverID string) (*redfish.Client, error) {
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
