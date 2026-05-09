package api

import (
	"net/http"

	"github.com/dell-infra-manager/backend/models"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

type JobsHandler struct {
	db  *sqlx.DB
	hub *Hub
}

func NewJobsHandler(db *sqlx.DB, hub *Hub) *JobsHandler {
	return &JobsHandler{db: db, hub: hub}
}

func (h *JobsHandler) GetServerJobs(c *gin.Context) {
	var jobs []models.Job
	h.db.Select(&jobs, `SELECT * FROM jobs WHERE server_id = ? ORDER BY created_at DESC`, c.Param("id"))
	c.JSON(http.StatusOK, jobs)
}

func (h *JobsHandler) GetAllJobs(c *gin.Context) {
	var jobs []models.Job
	h.db.Select(&jobs, `SELECT * FROM jobs ORDER BY created_at DESC`)
	c.JSON(http.StatusOK, jobs)
}

// GetIDRACJobs returns the live iDRAC job queue (not our local DB) — Lifecycle
// Controller jobs, config jobs, etc. — fetched in a single Redfish call via $expand.
func (h *JobsHandler) GetIDRACJobs(c *gin.Context) {
	client, err := buildClient(h.db, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}
	jobs, err := client.GetJobs()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, jobs)
}

// DeleteJob removes a local job and best-effort deletes the matching iDRAC job too.
func (h *JobsHandler) DeleteJob(c *gin.Context) {
	id, jid := c.Param("id"), c.Param("jid")

	if client, err := buildClient(h.db, id); err == nil {
		var j models.Job
		if h.db.Get(&j, `SELECT * FROM jobs WHERE id = ? AND server_id = ?`, jid, id) == nil &&
			j.IDRACJobID != nil {
			_ = client.DeleteJob(*j.IDRACJobID)
		}
	}

	res, err := h.db.Exec(`DELETE FROM jobs WHERE id = ? AND server_id = ?`, jid, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	c.JSON(http.StatusNoContent, nil)
}

// ClearAllJobs clears iDRAC's queue (JID_CLEARALL) and our local queued/running jobs.
func (h *JobsHandler) ClearAllJobs(c *gin.Context) {
	id := c.Param("id")
	if client, err := buildClient(h.db, id); err == nil {
		_ = client.ClearJobQueue(nil) // nil → JID_CLEARALL
	}
	h.db.Exec(`DELETE FROM jobs WHERE server_id = ? AND status IN ('queued','running')`, id)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
