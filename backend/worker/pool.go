package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/dell-infra-manager/backend/api"
	"github.com/dell-infra-manager/backend/config"
	"github.com/dell-infra-manager/backend/crypto"
	"github.com/dell-infra-manager/backend/models"
	"github.com/dell-infra-manager/backend/redfish"
	"github.com/jmoiron/sqlx"
)

type Pool struct {
	db  *sqlx.DB
	hub *api.Hub
	cfg *config.Config
}

func New(db *sqlx.DB, hub *api.Hub, cfg *config.Config) *Pool {
	return &Pool{db: db, hub: hub, cfg: cfg}
}

// Run starts all background workers and blocks until ctx is cancelled.
func (p *Pool) Run(ctx context.Context) {
	go p.catalogUpdater(ctx)
	go p.firmwareJobWorker(ctx)
	p.serverWatcher(ctx)
}

// ── Server lifecycle ─────────────────────────────────────────────────────────

type serverWorkerState struct {
	cancel    context.CancelFunc
	updatedAt time.Time
	alive     chan struct{} // closed when the worker goroutine exits
}

// serverWatcher polls the servers table every 10 s and (re)starts a per-server
// goroutine for each row. Restart triggers: new server, server config changed
// (updated_at moved), worker died (panic).
func (p *Pool) serverWatcher(ctx context.Context) {
	running := make(map[string]*serverWorkerState)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	start := func(s models.Server) {
		sCtx, cancel := context.WithCancel(ctx)
		alive := make(chan struct{})
		running[s.ID] = &serverWorkerState{cancel: cancel, updatedAt: s.UpdatedAt, alive: alive}
		go func() {
			defer close(alive)
			defer func() {
				if r := recover(); r != nil {
					log.Printf("poller[%s] PANIC: %v — worker died, will be restarted on next scan", s.Name, r)
				}
			}()
			p.serverWorker(sCtx, s)
		}()
	}

	isDead := func(s *serverWorkerState) bool {
		select {
		case <-s.alive:
			return true
		default:
			return false
		}
	}

	scan := func() {
		var servers []models.Server
		if err := p.db.Select(&servers, `SELECT * FROM servers`); err != nil {
			log.Printf("serverWatcher: db error: %v", err)
			return
		}
		active := make(map[string]bool)
		for _, s := range servers {
			active[s.ID] = true
			existing, ok := running[s.ID]
			switch {
			case !ok:
				log.Printf("poller[%s] starting", s.Name)
				start(s)
			case isDead(existing):
				log.Printf("poller[%s] previous worker exited, restarting", s.Name)
				delete(running, s.ID)
				start(s)
			case !existing.updatedAt.Equal(s.UpdatedAt):
				log.Printf("poller[%s] config changed, restarting", s.Name)
				existing.cancel()
				<-existing.alive
				start(s)
			}
		}
		for id, state := range running {
			if !active[id] {
				log.Printf("poller: server %s removed, stopping", id)
				state.cancel()
				delete(running, id)
			}
		}
	}

	scan() // immediate first scan so existing servers start polling at boot
	for {
		select {
		case <-ctx.Done():
			for _, s := range running {
				s.cancel()
			}
			return
		case <-ticker.C:
			scan()
		}
	}
}

// serverWorker runs the per-server polling loop until ctx is cancelled.
// It tries SSE first (event-driven, low latency) and falls back to ticker-based
// polling if SSE fails repeatedly (older firmware, network issues).
func (p *Pool) serverWorker(ctx context.Context, s models.Server) {
	password, err := crypto.Decrypt(s.Password)
	if err != nil {
		log.Printf("poller[%s] decrypt error: %v", s.Name, err)
		return
	}
	client := redfish.NewClient(s.Hostname, s.Port, s.Username, password, s.TLSVerify)

	sseCh := make(chan redfish.SSEEvent, 64)
	sseCtx, cancelSSE := context.WithCancel(ctx)
	defer cancelSSE()
	go func() {
		client.SSEListener(sseCtx, s.ID, sseCh, p.cfg.Polling.SSEReconnectMaxRetries)
		close(sseCh)
	}()

	pol := p.cfg.Polling
	systemTicker := time.NewTicker(time.Duration(pol.SystemIntervalSeconds) * time.Second)
	thermalTicker := time.NewTicker(time.Duration(pol.ThermalIntervalSeconds) * time.Second)
	powerTicker := time.NewTicker(time.Duration(pol.PowerIntervalSeconds) * time.Second)
	firmwareTicker := time.NewTicker(time.Duration(pol.FirmwareIntervalHours) * time.Hour)
	storageTicker := time.NewTicker(time.Duration(pol.StorageIntervalMinutes) * time.Minute)
	jobTicker := time.NewTicker(time.Duration(pol.JobQueueIntervalSeconds) * time.Second)
	defer systemTicker.Stop()
	defer thermalTicker.Stop()
	defer powerTicker.Stop()
	defer firmwareTicker.Stop()
	defer storageTicker.Stop()
	defer jobTicker.Stop()

	// Initial poll — fetch everything once at startup so the UI has data
	// immediately, without waiting for the storage (10 min) or firmware (6 h)
	// ticker. All polls run in parallel so a single slow iDRAC doesn't delay
	// the others.
	var wg sync.WaitGroup
	pollers := []func(){
		func() { p.pollSystem(client, s.ID, s.Name) },
		func() { p.pollThermal(client, s.ID, s.Name) },
		func() { p.pollPower(client, s.ID, s.Name) },
		func() { p.pollStorage(client, s.ID, s.Name) },
		func() { p.pollFirmware(client, s.ID, s.Name) },
	}
	for _, fn := range pollers {
		wg.Add(1)
		go func(f func()) { defer wg.Done(); f() }(fn)
	}
	wg.Wait()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-sseCh:
			if !ok {
				// SSE listener gave up after max retries. Nil channel blocks
				// forever, so this case never fires again — we keep polling.
				sseCh = nil
			} else {
				p.hub.Emit("sse_event", event.ServerID, map[string]string{
					"type": event.EventType,
					"data": event.Data,
				})
			}
		case <-systemTicker.C:
			p.pollSystem(client, s.ID, s.Name)
		case <-thermalTicker.C:
			p.pollThermal(client, s.ID, s.Name)
		case <-powerTicker.C:
			p.pollPower(client, s.ID, s.Name)
		case <-firmwareTicker.C:
			p.pollFirmware(client, s.ID, s.Name)
		case <-storageTicker.C:
			p.pollStorage(client, s.ID, s.Name)
		case <-jobTicker.C:
			p.checkJobs(client, s.ID)
		}
	}
}

// ── Generic poll-and-cache helper ────────────────────────────────────────────

// pollAndCache fetches a value from iDRAC, marshals it to JSON, and writes it
// to the named server_cache column. Wraps everything in panic recovery so a
// single bad response doesn't kill the worker. Returns the fetched value and
// true on success; nil/false on any error.
func (p *Pool) pollAndCache(serverID, name, kind, column string, fetch func() (any, error)) (any, bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("poller[%s] %s PANIC: %v", name, kind, r)
		}
	}()
	value, err := fetch()
	if err != nil {
		log.Printf("poller[%s] %s error: %v", name, kind, err)
		return nil, false
	}
	data, _ := json.Marshal(value)
	q := fmt.Sprintf(`UPDATE server_cache SET %s = ? WHERE server_id = ?`, column)
	p.db.Exec(q, string(data), serverID)
	return value, true
}

// ── Individual pollers (post-processing only) ────────────────────────────────

// pollSystem also flips status=online and emits status events.
func (p *Pool) pollSystem(client *redfish.Client, serverID, name string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("poller[%s] system PANIC: %v", name, r)
		}
	}()
	sys, err := client.GetSystem()
	if err != nil {
		p.db.Exec(`UPDATE server_cache SET status='offline' WHERE server_id=?`, serverID)
		p.hub.Emit("server_status", serverID, map[string]string{"status": "offline"})
		log.Printf("poller[%s] system error: %v", name, err)
		return
	}
	data, _ := json.Marshal(sys)
	now := time.Now()
	res, _ := p.db.Exec(`UPDATE server_cache SET system_json=?, last_seen=?, status='online' WHERE server_id=?`,
		string(data), now, serverID)
	if rows, _ := res.RowsAffected(); rows == 0 {
		// Row missing (newly added server, FK enforcement edge case) — create it.
		p.db.Exec(`INSERT OR IGNORE INTO server_cache (server_id, status) VALUES (?, 'online')`, serverID)
		p.db.Exec(`UPDATE server_cache SET system_json=?, last_seen=?, status='online' WHERE server_id=?`,
			string(data), now, serverID)
	}
	p.hub.Emit("server_status", serverID, map[string]string{"status": "online", "power_state": sys.PowerState})
	p.hub.Emit("power_state", serverID, map[string]string{"state": sys.PowerState})
}

func (p *Pool) pollThermal(client *redfish.Client, serverID, name string) {
	v, ok := p.pollAndCache(serverID, name, "thermal", "thermal_json",
		func() (any, error) { return client.GetThermal() })
	if !ok {
		return
	}
	t := v.(*redfish.ThermalInfo)
	p.hub.Emit("thermal_update", serverID, map[string]float64{"inlet_temp": findInletReading(t.Temperatures)})
}

func (p *Pool) pollPower(client *redfish.Client, serverID, name string) {
	p.pollAndCache(serverID, name, "power", "power_json",
		func() (any, error) { return client.GetPower() })
}

func (p *Pool) pollFirmware(client *redfish.Client, serverID, name string) {
	p.pollAndCache(serverID, name, "firmware", "firmware_json",
		func() (any, error) { return client.GetFirmwareInventory() })
}

func (p *Pool) pollStorage(client *redfish.Client, serverID, name string) {
	p.pollAndCache(serverID, name, "storage", "storage_json",
		func() (any, error) { return client.GetStorage() })
}

// ── Job tracking ─────────────────────────────────────────────────────────────

// checkJobs polls iDRAC for the status of each locally-tracked job in
// queued/running state, updates the DB, and broadcasts via WebSocket.
func (p *Pool) checkJobs(client *redfish.Client, serverID string) {
	var jobs []models.Job
	p.db.Select(&jobs, `SELECT * FROM jobs WHERE server_id=? AND status IN ('queued','running') AND idrac_job_id IS NOT NULL`, serverID)

	for _, job := range jobs {
		if job.IDRACJobID == nil {
			continue
		}
		iJob, err := client.GetJob(path.Base(*job.IDRACJobID))
		if err != nil {
			continue
		}
		p.hub.Emit("job_update", serverID, map[string]any{
			"job_id":  job.ID,
			"status":  iJob.JobState,
			"percent": iJob.PercentComplete,
			"message": iJob.Message,
		})
		if !redfish.IsJobDone(iJob.JobState) {
			continue
		}

		status := models.JobStatusDone
		if iJob.JobState == "Failed" {
			status = models.JobStatusFailed
		}
		result, _ := json.Marshal(iJob)
		p.db.Exec(`UPDATE jobs SET status=?, result=?, finished_at=? WHERE id=?`,
			string(status), string(result), time.Now(), job.ID)
		if job.Type == models.JobTypeBiosConfig {
			p.hub.Emit("bios_job_done", serverID, map[string]any{
				"job_id":  job.ID,
				"success": iJob.JobState == "Completed",
			})
		}
	}
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// findInletReading picks an inlet/ambient temperature from a Redfish sensor
// list, tolerating the various names iDRAC uses across firmware/chassis
// generations: "Inlet Temp", "System Inlet Temp", "Ambient Temp", etc.
func findInletReading(temps []redfish.TempSensor) float64 {
	for _, t := range temps {
		if t.Name == "Inlet Temp" {
			return t.ReadingCelsius
		}
	}
	for _, t := range temps {
		lower := strings.ToLower(t.Name)
		if strings.Contains(lower, "inlet") || strings.Contains(lower, "ambient") {
			return t.ReadingCelsius
		}
	}
	return 0
}
