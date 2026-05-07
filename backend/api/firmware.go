package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/dell-manager/backend/crypto"
	"github.com/dell-manager/backend/models"
	"github.com/dell-manager/backend/redfish"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type FirmwareHandler struct {
	db          *sqlx.DB
	hub         *Hub
	catalogPath string
}

func NewFirmwareHandler(db *sqlx.DB, hub *Hub, catalogPath string) *FirmwareHandler {
	return &FirmwareHandler{db: db, hub: hub, catalogPath: catalogPath}
}

func (h *FirmwareHandler) GetInventory(c *gin.Context) {
	id := c.Param("id")
	var val *string
	row := h.db.QueryRow(`SELECT firmware_json FROM server_cache WHERE server_id = ?`, id)
	if err := row.Scan(&val); err != nil || val == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "firmware cache not available yet"})
		return
	}
	c.Data(http.StatusOK, "application/json", []byte(*val))
}

type AvailableUpdate struct {
	Component        string `json:"component"`
	InstalledVersion string `json:"installed_version"`
	AvailableVersion string `json:"available_version"`
	ReleaseDate      string `json:"release_date"`
	CatalogPath      string `json:"catalog_path"`
}

func (h *FirmwareHandler) GetAvailable(c *gin.Context) {
	id := c.Param("id")
	var firmwareVal *string
	h.db.QueryRow(`SELECT firmware_json FROM server_cache WHERE server_id = ?`, id).Scan(&firmwareVal)
	if firmwareVal == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "firmware cache not available"})
		return
	}

	var installed []redfish.FirmwareComponent
	json.Unmarshal([]byte(*firmwareVal), &installed)

	catalog, err := redfish.LoadCatalog(h.catalogPath)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "catalog not available: " + err.Error()})
		return
	}

	var model string
	var systemVal *string
	h.db.QueryRow(`SELECT system_json FROM server_cache WHERE server_id = ?`, id).Scan(&systemVal)
	if systemVal != nil {
		var sys map[string]interface{}
		json.Unmarshal([]byte(*systemVal), &sys)
		if m, ok := sys["Model"].(string); ok {
			model = m
		}
	}

	catalogForModel := redfish.FilterByModel(catalog, model)

	var updates []AvailableUpdate
	for _, inst := range installed {
		for _, cat := range catalogForModel {
			if strings.EqualFold(cat.ComponentType, inst.Name) ||
				strings.Contains(strings.ToUpper(cat.Name), strings.ToUpper(inst.Name)) {
				if cat.Version != inst.Version {
					updates = append(updates, AvailableUpdate{
						Component:        inst.Name,
						InstalledVersion: inst.Version,
						AvailableVersion: cat.Version,
						ReleaseDate:      cat.ReleaseDate,
						CatalogPath:      cat.Path,
					})
				}
				break
			}
		}
	}
	c.JSON(http.StatusOK, updates)
}

type firmwareUpdateRequest struct {
	Component   string `json:"component"    binding:"required"`
	CatalogPath string `json:"catalog_path" binding:"required"`
	Version     string `json:"version"`
	ApplyTime   string `json:"apply_time"`
}

func (h *FirmwareHandler) QueueUpdate(c *gin.Context) {
	id := c.Param("id")
	var req firmwareUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.ApplyTime == "" {
		req.ApplyTime = "OnReset"
	}

	payload, _ := json.Marshal(models.FirmwareUpdatePayload{
		Component:   req.Component,
		CatalogPath: req.CatalogPath,
		Version:     req.Version,
	})
	jobID := uuid.New().String()
	now := time.Now()
	_, err := h.db.Exec(`INSERT INTO jobs (id, server_id, type, status, payload, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		jobID, id, string(models.JobTypeFirmwareUpdate), string(models.JobStatusQueued), string(payload), now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.hub.Emit("job_created", id, gin.H{"job_id": jobID, "type": "firmware_update"})
	c.JSON(http.StatusAccepted, gin.H{"job_id": jobID})
}

type bulkFirmwareRequest struct {
	ServerIDs   []string `json:"server_ids"   binding:"required"`
	Component   string   `json:"component"    binding:"required"`
	CatalogPath string   `json:"catalog_path" binding:"required"`
	Version     string   `json:"version"`
	ApplyTime   string   `json:"apply_time"`
}

func (h *FirmwareHandler) BulkQueueUpdate(c *gin.Context) {
	var req bulkFirmwareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var results []map[string]interface{}
	for _, id := range req.ServerIDs {
		payload, _ := json.Marshal(models.FirmwareUpdatePayload{
			Component:   req.Component,
			CatalogPath: req.CatalogPath,
			Version:     req.Version,
		})
		jobID := uuid.New().String()
		now := time.Now()
		_, err := h.db.Exec(`INSERT INTO jobs (id, server_id, type, status, payload, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			jobID, id, string(models.JobTypeFirmwareUpdate), string(models.JobStatusQueued), string(payload), now)
		if err != nil {
			results = append(results, map[string]interface{}{"server_id": id, "ok": false, "error": err.Error()})
		} else {
			h.hub.Emit("job_created", id, gin.H{"job_id": jobID, "type": "firmware_update"})
			results = append(results, map[string]interface{}{"server_id": id, "ok": true, "job_id": jobID})
		}
	}
	c.JSON(http.StatusAccepted, results)
}

func (h *FirmwareHandler) buildClient(serverID string) (*redfish.Client, error) {
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
