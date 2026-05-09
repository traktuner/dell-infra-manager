package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/dell-infra-manager/backend/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type BiosHandler struct {
	db  *sqlx.DB
	hub *Hub
}

func NewBiosHandler(db *sqlx.DB, hub *Hub) *BiosHandler {
	return &BiosHandler{db: db, hub: hub}
}

func (h *BiosHandler) GetBios(c *gin.Context) {
	id := c.Param("id")
	var val *string
	row := h.db.QueryRow(`SELECT bios_json FROM server_cache WHERE server_id = ?`, id)
	if err := row.Scan(&val); err != nil || val == nil {
		// cache miss — fetch live
		client, err := buildClient(h.db, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
			return
		}
		bios, err := client.GetBios()
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		data, _ := json.Marshal(bios)
		s := string(data)
		h.db.Exec(`UPDATE server_cache SET bios_json = ? WHERE server_id = ?`, s, id)
		c.Data(http.StatusOK, "application/json", data)
		return
	}
	c.Data(http.StatusOK, "application/json", []byte(*val))
}

func (h *BiosHandler) GetBiosRegistry(c *gin.Context) {
	id := c.Param("id")
	var val *string
	row := h.db.QueryRow(`SELECT bios_registry_json FROM server_cache WHERE server_id = ?`, id)
	if err := row.Scan(&val); err != nil || val == nil {
		client, err := buildClient(h.db, id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
			return
		}
		reg, err := client.GetBiosRegistry()
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		data, _ := json.Marshal(reg)
		s := string(data)
		h.db.Exec(`UPDATE server_cache SET bios_registry_json = ? WHERE server_id = ?`, s, id)
		c.Data(http.StatusOK, "application/json", data)
		return
	}
	c.Data(http.StatusOK, "application/json", []byte(*val))
}

func (h *BiosHandler) SetBiosSettings(c *gin.Context) {
	id := c.Param("id")
	var req models.BiosSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.ApplyTime == "" {
		req.ApplyTime = "OnReset"
	}

	client, err := buildClient(h.db, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	location, err := client.SetBiosAttributes(req.Attributes, req.ApplyTime)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	// Record job in local DB
	payload, _ := json.Marshal(models.BiosConfigPayload{
		Attributes: req.Attributes,
		ApplyTime:  req.ApplyTime,
	})
	jobID := uuid.New().String()
	now := time.Now()
	h.db.Exec(`INSERT INTO jobs (id, server_id, type, status, payload, idrac_job_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		jobID, id, string(models.JobTypeBiosConfig), string(models.JobStatusQueued),
		string(payload), location, now)

	h.hub.Emit("bios_job_created", id, gin.H{"job_id": jobID, "idrac_location": location})
	c.JSON(http.StatusAccepted, gin.H{"job_id": jobID, "idrac_location": location})
}

func (h *BiosHandler) GetPending(c *gin.Context) {
	id := c.Param("id")
	var jobs []models.Job
	h.db.Select(&jobs, `SELECT * FROM jobs WHERE server_id = ? AND type = ? AND status IN ('queued','running')
		ORDER BY created_at DESC`, id, string(models.JobTypeBiosConfig))
	c.JSON(http.StatusOK, jobs)
}

