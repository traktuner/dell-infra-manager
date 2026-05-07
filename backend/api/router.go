package api

import (
	"net/http"

	"github.com/dell-manager/backend/config"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

func NewRouter(db *sqlx.DB, hub *Hub, cfg *config.Config) *gin.Engine {
	r := gin.Default()

	servers := NewServerHandler(db, hub)
	actions := NewActionHandler(db, hub)
	bios := NewBiosHandler(db, hub)
	vm := NewVirtualMediaHandler(db, hub)
	fw := NewFirmwareHandler(db, hub, cfg.Dell.CachePath)
	storage := NewStorageHandler(db)
	eventlog := NewEventLogHandler(db)
	jobs := NewJobsHandler(db, hub)

	api := r.Group("/api/v1")
	{
		// Server CRUD
		api.GET("/servers", servers.List)
		api.POST("/servers", servers.Create)
		api.GET("/servers/:id", servers.Get)
		api.PUT("/servers/:id", servers.Update)
		api.DELETE("/servers/:id", servers.Delete)
		api.POST("/servers/:id/test", servers.TestConnection)

		// Server data (cached)
		api.GET("/servers/:id/summary", servers.GetSummary)
		api.GET("/servers/:id/thermal", servers.GetCacheField("thermal_json"))
		api.GET("/servers/:id/power", servers.GetCacheField("power_json"))
		api.GET("/servers/:id/storage", storage.GetStorage)
		api.GET("/servers/:id/firmware", fw.GetInventory)
		api.GET("/servers/:id/eventlog", eventlog.GetLog)
		api.DELETE("/servers/:id/eventlog", eventlog.ClearLog)

		// Power actions
		api.POST("/servers/:id/power", actions.PowerAction)
		api.POST("/servers/bulk/power", actions.BulkPowerAction)

		// BIOS
		api.GET("/servers/:id/bios", bios.GetBios)
		api.GET("/servers/:id/bios/registry", bios.GetBiosRegistry)
		api.PATCH("/servers/:id/bios/settings", bios.SetBiosSettings)
		api.GET("/servers/:id/bios/pending", bios.GetPending)

		// Virtual Media
		api.GET("/servers/:id/virtualmedia", vm.GetStatus)
		api.POST("/servers/:id/virtualmedia/insert", vm.Insert)
		api.POST("/servers/:id/virtualmedia/eject", vm.Eject)

		// Firmware
		api.GET("/servers/:id/firmware/available", fw.GetAvailable)
		api.POST("/servers/:id/firmware/update", fw.QueueUpdate)
		api.POST("/servers/bulk/firmware/update", fw.BulkQueueUpdate)

		// iDRAC Jobs
		api.GET("/servers/:id/jobs", jobs.GetServerJobs)
		api.DELETE("/servers/:id/jobs/:jid", jobs.DeleteJob)
		api.DELETE("/servers/:id/jobs", jobs.ClearAllJobs)

		// Global views
		api.GET("/jobs", jobs.GetAllJobs)
		api.GET("/dashboard", getDashboard(db))
	}

	// WebSocket
	r.GET("/ws", hub.HandleWS)

	// Serve embedded frontend — all non-API routes serve the SPA
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	})

	return r
}

func getDashboard(db *sqlx.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var total, online, offline int
		db.QueryRow(`SELECT COUNT(*) FROM servers`).Scan(&total)
		db.QueryRow(`SELECT COUNT(*) FROM server_cache WHERE status = 'online'`).Scan(&online)
		db.QueryRow(`SELECT COUNT(*) FROM server_cache WHERE status = 'offline'`).Scan(&offline)
		var activeJobs int
		db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE status IN ('queued','running')`).Scan(&activeJobs)

		c.JSON(http.StatusOK, gin.H{
			"total_servers":  total,
			"online":         online,
			"offline":        offline,
			"error":          total - online - offline,
			"active_jobs":    activeJobs,
		})
	}
}
