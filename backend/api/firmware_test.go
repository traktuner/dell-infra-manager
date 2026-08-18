package api

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/dell-infra-manager/backend/database"
)

func TestEnqueueFirmwareUpdateDeduplicatesActivePackage(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		INSERT INTO servers (id, name, hostname, username, password)
		VALUES ('server-1', 'Server 1', 'idrac.example', 'root', 'encrypted')`); err != nil {
		t.Fatal(err)
	}

	h := &FirmwareHandler{db: db}
	first, err := h.enqueueFirmwareUpdate("server-1", "BIOS", "FOLDER/bios.EXE", "2.0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.enqueueFirmwareUpdate("server-1", "BIOS", "FOLDER/bios.EXE", "2.0"); !errors.Is(err, errFirmwareAlreadyQueued) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
	if _, err := db.Exec(`UPDATE jobs SET status='done' WHERE id=?`, first); err != nil {
		t.Fatal(err)
	}
	if _, err := h.enqueueFirmwareUpdate("server-1", "BIOS", "FOLDER/bios.EXE", "2.0"); err != nil {
		t.Fatalf("completed package must not block a later update: %v", err)
	}
}
