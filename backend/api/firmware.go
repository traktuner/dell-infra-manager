package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
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
	catalogMu   sync.Mutex
	catalog     []redfish.CatalogComponent
	catalogTime time.Time
	catalogSize int64
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
	h.catalogMu.Lock()
	defer h.catalogMu.Unlock()

	if stat, err := osStat(h.catalogPath); err == nil && h.catalog != nil &&
		stat.ModTime().Equal(h.catalogTime) && stat.Size() == h.catalogSize {
		return h.catalog, nil
	}

	catalog, err := redfish.LoadCatalog(h.catalogPath)
	if err != nil || len(catalog) == 0 {
		if err == nil {
			err = fmt.Errorf("catalog contains no software components")
		}
		log.Printf("catalog parse failed (%v) — downloading a validated replacement", err)
		if dlErr := redfish.DownloadCatalog(h.catalogURL, h.catalogPath); dlErr != nil {
			return nil, fmt.Errorf("replace invalid catalog: %w", dlErr)
		}
		catalog, err = redfish.LoadCatalog(h.catalogPath)
		if err != nil {
			return nil, fmt.Errorf("parse validated replacement catalog: %w", err)
		}
	}
	if stat, statErr := osStat(h.catalogPath); statErr == nil {
		h.catalogTime = stat.ModTime()
		h.catalogSize = stat.Size()
	}
	h.catalog = catalog
	return catalog, nil
}

// osStat is a variable so catalog-cache behavior can be tested without
// changing the process working directory.
var osStat = os.Stat

func (h *FirmwareHandler) refreshCatalog() (bool, error) {
	h.catalogMu.Lock()
	defer h.catalogMu.Unlock()
	updated, err := redfish.DownloadCatalogIfModified(h.catalogURL, h.catalogPath)
	if err != nil {
		return false, err
	}
	if updated {
		h.catalog = nil
		h.catalogTime = time.Time{}
		h.catalogSize = 0
	}
	return updated, nil
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
	updated, err := h.refreshCatalog()
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
		if _, err := h.refreshCatalog(); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "catalog refresh failed: " + err.Error()})
			return
		}
	}
	comparison, err := h.comparisonForServer(id, refresh)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, comparison)
}

func (h *FirmwareHandler) comparisonForServer(id string, refresh bool) ([]redfish.ComponentStatus, error) {
	var firmwareVal *string
	if err := h.db.QueryRow(`SELECT firmware_json FROM server_cache WHERE server_id = ?`, id).Scan(&firmwareVal); err != nil || firmwareVal == nil {
		return nil, fmt.Errorf("firmware cache not available")
	}
	var installed []redfish.FirmwareComponent
	if err := json.Unmarshal([]byte(*firmwareVal), &installed); err != nil {
		return nil, fmt.Errorf("decode firmware cache: %w", err)
	}

	// If refresh=1 OR cached entries lack SoftwareId (e.g. cache from older
	// build), re-fetch live inventory so the catalog match by SoftwareId actually
	// has the data it needs. Costs one Redfish round-trip but the caller is
	// explicitly asking for fresh data via the Re-check button.
	if refresh || missingSoftwareIds(installed) {
		client, clientErr := buildClient(h.db, id)
		if clientErr == nil {
			fresh, fetchErr := client.GetFirmwareInventory()
			if fetchErr == nil && len(fresh) > 0 {
				installed = fresh
				if data, marshalErr := json.Marshal(fresh); marshalErr == nil {
					_, _ = h.db.Exec(`UPDATE server_cache SET firmware_json=? WHERE server_id=?`, string(data), id)
				}
			} else if refresh {
				if fetchErr != nil {
					return nil, fmt.Errorf("refresh firmware inventory: %w", fetchErr)
				}
				return nil, fmt.Errorf("refresh firmware inventory: iDRAC returned no components")
			}
		} else if refresh {
			return nil, fmt.Errorf("refresh firmware inventory: %w", clientErr)
		}
	}

	catalog, err := h.loadCatalogWithSelfHeal()
	if err != nil {
		return nil, fmt.Errorf("catalog not available: %w", err)
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

	return redfish.CompareInventory(installed, catalog, model), nil
}

type firmwareUpdateRequest struct {
	Component   string `json:"component"    binding:"required"`
	InventoryID string `json:"inventory_id"`
	SoftwareID  string `json:"software_id"`
	CatalogPath string `json:"catalog_path" binding:"required"`
	Version     string `json:"version"`
	ApplyTime   string `json:"apply_time"`
}

var errFirmwareAlreadyQueued = errors.New("this firmware package is already queued or running for the server")

func (h *FirmwareHandler) QueueUpdate(c *gin.Context) {
	id := c.Param("id")
	var req firmwareUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.ApplyTime != "" && req.ApplyTime != "OnReset" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only OnReset is allowed; this application never requests an immediate firmware apply"})
		return
	}
	if err := h.validateFirmwareRequest(id, req); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	jobID, err := h.enqueueFirmwareUpdate(id, req.Component, req.CatalogPath, req.Version)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errFirmwareAlreadyQueued) {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": err.Error()})
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

func (h *FirmwareHandler) validateFirmwareRequest(serverID string, req firmwareUpdateRequest) error {
	comparison, err := h.comparisonForServer(serverID, false)
	if err != nil {
		return err
	}
	for _, item := range comparison {
		identityMatches := req.InventoryID != "" && item.InventoryID == req.InventoryID
		if !identityMatches && req.InventoryID == "" {
			identityMatches = item.Component == req.Component &&
				(req.SoftwareID == "" || item.SoftwareID == req.SoftwareID)
		}
		if identityMatches && item.Updateable && item.Outdated && item.CatalogPath == req.CatalogPath &&
			(req.Version == "" || item.AvailableVersion == req.Version) {
			return nil
		}
	}
	return fmt.Errorf("the requested package is not the current outdated match for this server; run the firmware check again")
}

func (h *FirmwareHandler) enqueueFirmwareUpdate(serverID, component, catalogPath, version string) (string, error) {
	payload, err := json.Marshal(models.FirmwareUpdatePayload{
		Component: component, CatalogPath: catalogPath, Version: version,
	})
	if err != nil {
		return "", err
	}
	jobID := uuid.New().String()
	result, err := h.db.Exec(`
		INSERT INTO jobs (id, server_id, type, status, payload, created_at)
		SELECT ?, ?, ?, ?, ?, ?
		WHERE NOT EXISTS (
			SELECT 1 FROM jobs
			WHERE server_id = ? AND type = ? AND status IN ('queued','running')
			AND json_extract(payload, '$.catalog_path') = ?
		)`,
		jobID, serverID, string(models.JobTypeFirmwareUpdate), string(models.JobStatusQueued), string(payload), time.Now(),
		serverID, string(models.JobTypeFirmwareUpdate), catalogPath,
	)
	if err != nil {
		return "", err
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return "", errFirmwareAlreadyQueued
	}
	return jobID, nil
}

func (h *FirmwareHandler) BulkQueueUpdate(c *gin.Context) {
	var req bulkFirmwareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	results := make([]map[string]interface{}, 0, len(req.ServerIDs))
	seen := make(map[string]struct{}, len(req.ServerIDs))
	for _, id := range req.ServerIDs {
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		validation := firmwareUpdateRequest{
			Component: req.Component, CatalogPath: req.CatalogPath,
			Version: req.Version, ApplyTime: req.ApplyTime,
		}
		if req.ApplyTime != "" && req.ApplyTime != "OnReset" {
			results = append(results, map[string]interface{}{"server_id": id, "ok": false, "error": "only OnReset is allowed"})
			continue
		}
		if err := h.validateFirmwareRequest(id, validation); err != nil {
			results = append(results, map[string]interface{}{"server_id": id, "ok": false, "error": err.Error()})
			continue
		}
		jobID, err := h.enqueueFirmwareUpdate(id, req.Component, req.CatalogPath, req.Version)
		if err != nil {
			results = append(results, map[string]interface{}{"server_id": id, "ok": false, "error": err.Error()})
		} else {
			h.hub.Emit("job_created", id, gin.H{"job_id": jobID, "type": "firmware_update"})
			results = append(results, map[string]interface{}{"server_id": id, "ok": true, "job_id": jobID})
		}
	}
	c.JSON(http.StatusAccepted, results)
}
