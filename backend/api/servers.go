package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/dell-manager/backend/crypto"
	"github.com/dell-manager/backend/models"
	"github.com/dell-manager/backend/redfish"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type ServerHandler struct {
	db  *sqlx.DB
	hub *Hub
}

func NewServerHandler(db *sqlx.DB, hub *Hub) *ServerHandler {
	return &ServerHandler{db: db, hub: hub}
}

func (h *ServerHandler) List(c *gin.Context) {
	var servers []models.Server
	if err := h.db.Select(&servers, `SELECT * FROM servers ORDER BY name`); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, servers)
}

func (h *ServerHandler) Get(c *gin.Context) {
	var s models.Server
	if err := h.db.Get(&s, `SELECT * FROM servers WHERE id = ?`, c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}
	c.JSON(http.StatusOK, s)
}

func (h *ServerHandler) Create(c *gin.Context) {
	var req models.AddServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Port == 0 {
		req.Port = 443
	}
	if req.Tags == "" {
		req.Tags = "[]"
	}

	encrypted, err := crypto.Encrypt(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "encrypt password"})
		return
	}

	id := uuid.New().String()
	now := time.Now()
	s := models.Server{
		ID: id, Name: req.Name, Hostname: req.Hostname, Port: req.Port,
		Username: req.Username, Password: encrypted, TLSVerify: req.TLSVerify,
		Tags: req.Tags, CreatedAt: now, UpdatedAt: now,
	}

	_, err = h.db.NamedExec(`INSERT INTO servers
		(id, name, hostname, port, username, password, tls_verify, tags, created_at, updated_at)
		VALUES (:id, :name, :hostname, :port, :username, :password, :tls_verify, :tags, :created_at, :updated_at)`, s)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Create empty cache row
	h.db.Exec(`INSERT OR IGNORE INTO server_cache (server_id, status) VALUES (?, 'unknown')`, id)

	c.JSON(http.StatusCreated, s)
}

func (h *ServerHandler) Update(c *gin.Context) {
	id := c.Param("id")
	var s models.Server
	if err := h.db.Get(&s, `SELECT * FROM servers WHERE id = ?`, id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	var req models.UpdateServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != nil {
		s.Name = *req.Name
	}
	if req.Hostname != nil {
		s.Hostname = *req.Hostname
	}
	if req.Port != nil {
		s.Port = *req.Port
	}
	if req.Username != nil {
		s.Username = *req.Username
	}
	if req.Password != nil {
		enc, err := crypto.Encrypt(*req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "encrypt password"})
			return
		}
		s.Password = enc
	}
	if req.TLSVerify != nil {
		s.TLSVerify = *req.TLSVerify
	}
	if req.Tags != nil {
		s.Tags = *req.Tags
	}
	s.UpdatedAt = time.Now()

	_, err := h.db.NamedExec(`UPDATE servers SET
		name=:name, hostname=:hostname, port=:port, username=:username,
		password=:password, tls_verify=:tls_verify, tags=:tags, updated_at=:updated_at
		WHERE id=:id`, s)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, s)
}

func (h *ServerHandler) Delete(c *gin.Context) {
	res, err := h.db.Exec(`DELETE FROM servers WHERE id = ?`, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}
	c.JSON(http.StatusNoContent, nil)
}

func (h *ServerHandler) TestConnection(c *gin.Context) {
	var s models.Server
	if err := h.db.Get(&s, `SELECT * FROM servers WHERE id = ?`, c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}
	password, err := crypto.Decrypt(s.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "decrypt password"})
		return
	}
	client := redfish.NewClient(s.Hostname, s.Port, s.Username, password, s.TLSVerify)
	if err := client.Ping(); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *ServerHandler) GetSummary(c *gin.Context) {
	id := c.Param("id")
	var cache models.ServerCache
	if err := h.db.Get(&cache, `SELECT * FROM server_cache WHERE server_id = ?`, id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no cache for server"})
		return
	}
	c.JSON(http.StatusOK, cache)
}

func (h *ServerHandler) GetCacheField(field string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		row := h.db.QueryRow(fmt.Sprintf(`SELECT %s FROM server_cache WHERE server_id = ?`, field), id)
		var val *string
		if err := row.Scan(&val); err != nil || val == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not available"})
			return
		}
		c.Data(http.StatusOK, "application/json", []byte(*val))
	}
}
