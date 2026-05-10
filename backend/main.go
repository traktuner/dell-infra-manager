package main

import (
	"context"
	"embed"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dell-infra-manager/backend/api"
	"github.com/dell-infra-manager/backend/config"
	"github.com/dell-infra-manager/backend/crypto"
	"github.com/dell-infra-manager/backend/database"
	"github.com/dell-infra-manager/backend/notifier"
	"github.com/dell-infra-manager/backend/worker"
)

//go:embed all:frontend/dist
var staticFiles embed.FS

func main() {
	cfg := config.Load()

	// Initialize encryption key
	keyEnv := cfg.Security.MasterKeyEnv
	if keyEnv == "" {
		keyEnv = "MASTER_KEY"
	}
	if err := crypto.Init(cfg.Security.MasterKeyPath, keyEnv); err != nil {
		log.Fatalf("crypto init: %v", err)
	}

	// Open database
	db, err := database.Open(cfg.Database.Path)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	// WebSocket hub
	hub := api.NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	// Notifier (SMTP) — settings live in DB, single instance reused everywhere.
	notif := notifier.New(db)

	// Background workers
	pool := worker.New(db, hub, cfg, notif)
	go pool.Run(ctx)

	// Wire the "Send digest now" Settings button to the worker's digest
	// runner. Done here (not in api.NewRouter) to avoid an api → worker
	// import cycle.
	api.SetFirmwareDigestRunner(pool.RunFirmwareDigest)

	// HTTP router
	router := api.NewRouter(db, hub, cfg, notif, staticFiles)

	srv := &http.Server{
		Addr:    cfg.Server.Host + ":" + cfg.Server.Port,
		Handler: router,
	}

	go func() {
		log.Printf("dell-infra-manager listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	cancel()
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	log.Println("dell-infra-manager stopped")
}
