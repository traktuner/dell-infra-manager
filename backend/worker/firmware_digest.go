package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/dell-infra-manager/backend/models"
	"github.com/dell-infra-manager/backend/redfish"
)

// firmwareDigestWorker runs a daily scan across all servers and sends ONE
// summary email if any component has an outdated catalog version. The
// `on_firmware_updates` toggle in Settings gates the alert; the per-event
// dedup window (1 h) plus the daily scan cadence ensures users get at most
// one email per day even if the scan triggers multiple times.
//
// Schedule: catalog updates at cfg.Polling.CatalogUpdateHour (3 AM by default);
// we scan one hour later so the catalog is guaranteed fresh.
func (p *Pool) firmwareDigestWorker(ctx context.Context) {
	for {
		next := nextDigestRun(p.cfg.Polling.CatalogUpdateHour + 1)
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(next)):
			p.runFirmwareDigest()
		}
	}
}

// nextDigestRun returns the next clock time at the given hour-of-day.
func nextDigestRun(hour int) time.Time {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

// RunFirmwareDigest is the public entry point for triggering the digest on
// demand (Settings UI "Send digest now" button). The scheduled worker calls
// the same function once per day.
func (p *Pool) RunFirmwareDigest() { p.runFirmwareDigest() }

// runFirmwareDigest scans every server's cached inventory against the catalog
// and aggregates any outdated components into a single email body.
func (p *Pool) runFirmwareDigest() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("firmware digest PANIC: %v", r)
		}
	}()

	settings, err := p.notif.LoadSettings()
	if err != nil || !settings.Enabled || !settings.OnFirmwareUpdates {
		return
	}

	catalog, err := redfish.LoadCatalog(p.cfg.Dell.CachePath)
	if err != nil {
		log.Printf("firmware digest: catalog unreadable: %v", err)
		return
	}

	var servers []models.Server
	if err := p.db.Select(&servers, `SELECT * FROM servers ORDER BY name`); err != nil {
		log.Printf("firmware digest: db error: %v", err)
		return
	}

	type serverDigest struct {
		Name     string
		Hostname string
		Outdated []redfish.ComponentStatus
	}
	var fleet []serverDigest
	totalOutdated := 0

	for _, s := range servers {
		var fwJSON, sysJSON *string
		p.db.QueryRow(`SELECT firmware_json, system_json FROM server_cache WHERE server_id = ?`, s.ID).
			Scan(&fwJSON, &sysJSON)
		if fwJSON == nil {
			continue
		}

		var installed []redfish.FirmwareComponent
		if err := json.Unmarshal([]byte(*fwJSON), &installed); err != nil {
			continue
		}
		var model string
		if sysJSON != nil {
			var sys map[string]any
			if json.Unmarshal([]byte(*sysJSON), &sys) == nil {
				if m, ok := sys["Model"].(string); ok {
					model = m
				}
			}
		}

		all := redfish.CompareInventory(installed, catalog, model)
		var outdated []redfish.ComponentStatus
		for _, c := range all {
			if c.Outdated {
				outdated = append(outdated, c)
			}
		}
		if len(outdated) == 0 {
			continue
		}
		// Stable order: most-recently-released update first.
		sort.Slice(outdated, func(i, j int) bool {
			return outdated[i].ReleaseDate > outdated[j].ReleaseDate
		})
		fleet = append(fleet, serverDigest{Name: s.Name, Hostname: s.Hostname, Outdated: outdated})
		totalOutdated += len(outdated)
	}

	if totalOutdated == 0 {
		return // nothing to report; don't email "all green"
	}

	subject := fmt.Sprintf("[Dell iDRAC Manager] %d firmware update(s) available across %d server(s)",
		totalOutdated, len(fleet))

	var body strings.Builder
	for _, d := range fleet {
		fmt.Fprintf(&body, "Server: %s (%s)\n", d.Name, d.Hostname)
		for _, c := range d.Outdated {
			fmt.Fprintf(&body, "  ↑ %-28s %s → %s   (released %s)\n",
				c.Component, c.InstalledVersion, c.AvailableVersion, c.ReleaseDate)
		}
		body.WriteString("\n")
	}
	body.WriteString("Open the Firmware page in the dashboard and click \"Check All Updates\" to queue updates.\n")

	p.notif.Send("firmware_updates", "", subject, body.String())
}
