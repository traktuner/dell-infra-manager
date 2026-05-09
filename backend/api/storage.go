package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

type StorageHandler struct {
	db *sqlx.DB
}

func NewStorageHandler(db *sqlx.DB) *StorageHandler {
	return &StorageHandler{db: db}
}

// GetStorage serves the cached storage payload, falling back to a live Redfish
// fetch on cache miss (e.g., before the first poll has run).
func (h *StorageHandler) GetStorage(c *gin.Context) {
	id := c.Param("id")
	var val *string
	h.db.QueryRow(`SELECT storage_json FROM server_cache WHERE server_id = ?`, id).Scan(&val)
	if val != nil {
		c.Data(http.StatusOK, "application/json", []byte(*val))
		return
	}
	client, err := buildClient(h.db, id)
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
