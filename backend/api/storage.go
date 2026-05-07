package api

import (
	"net/http"

	"github.com/dell-manager/backend/crypto"
	"github.com/dell-manager/backend/models"
	"github.com/dell-manager/backend/redfish"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

type StorageHandler struct {
	db  *sqlx.DB
}

func NewStorageHandler(db *sqlx.DB) *StorageHandler {
	return &StorageHandler{db: db}
}

func (h *StorageHandler) GetStorage(c *gin.Context) {
	id := c.Param("id")
	var val *string
	h.db.QueryRow(`SELECT storage_json FROM server_cache WHERE server_id = ?`, id).Scan(&val)
	if val != nil {
		c.Data(http.StatusOK, "application/json", []byte(*val))
		return
	}
	// Live fetch on cache miss
	client, err := h.buildClient(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}
	storage, err := client.GetStorage()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, storage)
}

func (h *StorageHandler) buildClient(serverID string) (*redfish.Client, error) {
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
