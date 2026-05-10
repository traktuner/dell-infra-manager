package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/dell-infra-manager/backend/config"
	"github.com/dell-infra-manager/backend/models"
	"github.com/dell-infra-manager/backend/redfish"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type FirmwareHandler struct {
	db          *sqlx.DB
	hub         *Hub
	catalogPath string
	catalogURL  string
}

func NewFirmwareHandler(db *sqlx.DB, hub *Hub, cfg *config.Config) *FirmwareHandler {
	return &FirmwareHandler{
		db:          db,
		hub:         hub,
		catalogPath: cfg.Dell.CachePath,
		catalogURL:  cfg.Dell.CatalogURL,
	}
}

// missingSoftwareIds returns true if any cached firmware component lacks a
// SoftwareId — happens for caches written by an older build. We use this as
// a signal to force a fresh inventory poll so catalog matching has real data.
func missingSoftwareIds(comps []redfish.FirmwareComponent) bool {
	for _, c := range comps {
		if c.SoftwareId == "" {
			return true
		}
	}
	return false
}

// loadCatalogWithSelfHeal parses the cached catalog. On parse failure it
// assumes the cache is corrupt (truncated download, encoding mismatch from a
// previous buggy version, etc.), forces an unconditional re-download, and
// retries once. This is what makes "Check for Updates" robust without
// requiring the user to manually delete /data/catalog.xml.gz.
func (h *FirmwareHandler) loadCatalogWithSelfHeal() ([]redfish.CatalogComponent, error) {
	catalog, err := redfish.LoadCatalog(h.catalogPath)
	if err == nil {
		return catalog, nil
	}
	log.Printf("catalog parse failed (%v) — forcing fresh download", err)

	_ = os.Remove(h.catalogPath)
	if dlErr := redfish.DownloadCatalog(h.catalogURL, h.catalogPath); dlErr != nil {
		return nil, fmt.Errorf("re-download after parse failure: %w", dlErr)
	}
	catalog, err = redfish.LoadCatalog(h.catalogPath)
	if err != nil {
		return nil, fmt.Errorf("still unparseable after fresh download: %w", err)
	}
	return catalog, nil
}

// GetCatalogInfo exposes the locally-cached catalog's dateTime/version so the
// UI can show "checking against catalog from <date>".
func (h *FirmwareHandler) GetCatalogInfo(c *gin.Context) {
	info, err := redfish.ReadCatalogInfo(h.catalogPath)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"available": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"available":  true,
		"date_time":  info.DateTime,
		"version":    info.Version,
		"fetched_at": info.FetchedAt,
	})
}

// RefreshCatalog does a conditional GET against Dell — cheap if nothing
// changed on the server (304 Not Modified), full download otherwise.
func (h *FirmwareHandler) RefreshCatalog(c *gin.Context) {
	updated, err := redfish.DownloadCatalogIfModified(h.catalogURL, h.catalogPath)
	if err != nil {
		log.Printf("catalog refresh failed: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	info, _ := redfish.ReadCatalogInfo(h.catalogPath)
	resp := gin.H{"updated": updated}
	if info != nil {
		resp["date_time"] = info.DateTime
		resp["version"] = info.Version
		resp["fetched_at"] = info.FetchedAt
	}
	c.JSON(http.StatusOK, resp)
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

// AvailableUpdate is the JSON-serialised row returned by /firmware/available.
// It mirrors redfish.ComponentStatus exactly — kept as a separate type so the
// API contract stays in this package.
type AvailableUpdate = redfish.ComponentStatus

func (h *FirmwareHandler) GetAvailable(c *gin.Context) {
	id := c.Param("id")
	refresh := c.Query("refresh") == "1"

	if refresh {
		// Refresh Dell catalog (cheap if nothing changed; 304 Not Modified).
		if _, err := redfish.DownloadCatalogIfModified(h.catalogURL, h.catalogPath); err != nil {
			log.Printf("catalog refresh during GetAvailable failed: %v", err)
		}
	}

	var firmwareVal *string
	h.db.QueryRow(`SELECT firmware_json FROM server_cache WHERE server_id = ?`, id).Scan(&firmwareVal)
	if firmwareVal == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "firmware cache not available"})
		return
	}

	var installed []redfish.FirmwareComponent
	json.Unmarshal([]byte(*firmwareVal), &installed)

	// If refresh=1 OR cached entries lack SoftwareId (e.g. cache from older
	// build), re-fetch live inventory so the catalog match by SoftwareId actually
	// has the data it needs. Costs one Redfish round-trip but the caller is
	// explicitly asking for fresh data via the Re-check button.
	if refresh || missingSoftwareIds(installed) {
		if client, err := buildClient(h.db, id); err == nil {
			if fresh, fErr := client.GetFirmwareInventory(); fErr == nil && len(fresh) > 0 {
				installed = fresh
				if data, mErr := json.Marshal(fresh); mErr == nil {
					h.db.Exec(`UPDATE server_cache SET firmware_json=? WHERE server_id=?`, string(data), id)
				}
			}
		}
	}

	catalog, err := h.loadCatalogWithSelfHeal()
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

	c.JSON(http.StatusOK, redfish.CompareInventory(installed, catalog, model))
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

	results := make([]map[string]interface{}, 0, len(req.ServerIDs))
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

