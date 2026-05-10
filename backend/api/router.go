package api

import (
	"io/fs"
	"net/http"

	"github.com/dell-infra-manager/backend/config"
	"github.com/dell-infra-manager/backend/notifier"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

func NewRouter(db *sqlx.DB, hub *Hub, cfg *config.Config, n *notifier.Notifier, staticFiles fs.FS) *gin.Engine {
	r := gin.Default()

	servers := NewServerHandler(db, hub)
	actions := NewActionHandler(db, hub)
	bios := NewBiosHandler(db, hub)
	vm := NewVirtualMediaHandler(db, hub)
	fw := NewFirmwareHandler(db, hub, cfg)
	storage := NewStorageHandler(db)
	eventlog := NewEventLogHandler(db)
	jobs := NewJobsHandler(db, hub)
	console := NewConsoleHandler(db)
	vnc := NewVNCHandler(db)
	settings := NewSettingsHandler(n)

	api := r.Group("/api/v1")
	api.Use(AuthMiddleware(cfg.Auth))
	{
		api.GET("/me", MeHandler(cfg.Auth))
		// Server CRUD
		api.GET("/servers", servers.List)
		api.POST("/servers", servers.Create)
		api.POST("/servers/test", servers.TestCredentials)
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

		// Catalog (Dell DUP catalog metadata + manual refresh)
		api.GET("/catalog/info", fw.GetCatalogInfo)
		api.POST("/catalog/refresh", fw.RefreshCatalog)

		// Jobs
		api.GET("/servers/:id/jobs", jobs.GetServerJobs)         // local jobs we queued
		api.GET("/servers/:id/jobs/idrac", jobs.GetIDRACJobs)    // live iDRAC job queue
		api.DELETE("/servers/:id/jobs/:jid", jobs.DeleteJob)
		api.DELETE("/servers/:id/jobs", jobs.ClearAllJobs)

		// Console: SSH/SOL (fallback)
		api.GET("/servers/:id/console", console.Connect)

		// Console: VNC/KVM via noVNC. /enable is idempotent — call it on every
		// open; it reads iDRAC state and only PATCHes when (re)configuration is needed.
		api.POST("/servers/:id/vnc/enable",   vnc.Enable)
		api.GET("/servers/:id/vnc/proxy",    vnc.Proxy)    // WebSocket TCP-tunnel
		api.GET("/servers/:id/vnc/password", vnc.Password) // RFB auth password
		api.POST("/servers/:id/vnc/reset",    vnc.Reset)   // force re-configure on next /enable

		// Settings (SMTP / notifications)
		api.GET("/settings/notifications",            settings.GetNotifications)
		api.PUT("/settings/notifications",            settings.UpdateNotifications)
		api.POST("/settings/notifications/test",      settings.TestNotifications)
		api.POST("/settings/notifications/digest-now", settings.SendDigestNow)

		// Global views
		api.GET("/jobs", jobs.GetAllJobs)
		api.GET("/dashboard", getDashboard(db))
	}

	// WebSocket
	r.GET("/ws", hub.HandleWS)

	// Serve embedded SvelteKit SPA — static assets directly, unknown paths → index.html
	sub, _ := fs.Sub(staticFiles, "frontend/dist")
	r.NoRoute(spaHandler(sub))

	return r
}

func spaHandler(fsys fs.FS) gin.HandlerFunc {
	fileServer := http.FileServer(http.FS(fsys))
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/" || path == "" {
			path = "index.html"
		} else {
			// strip leading slash for fs.Stat
			path = path[1:]
		}
		if _, err := fs.Stat(fsys, path); err != nil {
			// unknown path → SPA fallback
			c.Request.URL.Path = "/"
		}
		fileServer.ServeHTTP(c.Writer, c.Request)
	}
}

func getDashboard(db *sqlx.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Join through servers so an orphaned cache row (left over from a
		// deleted server before fk enforcement was on) can't inflate the
		// online/offline counters past the total.
		var total, online, offline int
		db.QueryRow(`SELECT COUNT(*) FROM servers`).Scan(&total)
		db.QueryRow(`
			SELECT COUNT(*) FROM servers s
			JOIN server_cache c ON c.server_id = s.id
			WHERE c.status = 'online'`).Scan(&online)
		db.QueryRow(`
			SELECT COUNT(*) FROM servers s
			JOIN server_cache c ON c.server_id = s.id
			WHERE c.status = 'offline'`).Scan(&offline)
		var activeJobs int
		db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE status IN ('queued','running')`).Scan(&activeJobs)

		errCount := total - online - offline
		if errCount < 0 {
			errCount = 0
		}
		c.JSON(http.StatusOK, gin.H{
			"total_servers": total,
			"online":        online,
			"offline":        offline,
			"error":         errCount,
			"active_jobs":   activeJobs,
		})
	}
}
