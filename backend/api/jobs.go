package api

import (
	"net/http"

	"github.com/dell-manager/backend/crypto"
	"github.com/dell-manager/backend/models"
	"github.com/dell-manager/backend/redfish"
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

func (h *JobsHandler) DeleteJob(c *gin.Context) {
	id := c.Param("id")
	jid := c.Param("jid")

	// Try to also delete from iDRAC
	client, err := h.buildClient(id)
	if err == nil {
		var j models.Job
		if h.db.Get(&j, `SELECT * FROM jobs WHERE id = ? AND server_id = ?`, jid, id) == nil {
			if j.IDRACJobID != nil {
				_ = client.DeleteJob(*j.IDRACJobID)
			}
		}
	}

	res, err := h.db.Exec(`DELETE FROM jobs WHERE id = ? AND server_id = ?`, jid, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	c.JSON(http.StatusNoContent, nil)
}

func (h *JobsHandler) ClearAllJobs(c *gin.Context) {
	id := c.Param("id")

	client, err := h.buildClient(id)
	if err == nil {
		_ = client.ClearJobQueue(nil) // nil = JID_CLEARALL
	}

	h.db.Exec(`DELETE FROM jobs WHERE server_id = ? AND status IN ('queued','running')`, id)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *JobsHandler) buildClient(serverID string) (*redfish.Client, error) {
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
