package worker

import (
	"context"
	"encoding/json"
	"log"
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

	// Server polling: watch for new servers and start per-server goroutines
	p.serverWatcher(ctx)
}

type serverWorkerState struct {
	cancel    context.CancelFunc
	updatedAt time.Time
	alive     chan struct{} // closed when the worker goroutine exits
}

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
		log.Printf("serverWatcher: scanned %d server(s)", len(servers))
		active := make(map[string]bool)
		for _, s := range servers {
			active[s.ID] = true
			existing, ok := running[s.ID]
			switch {
			case !ok:
				log.Printf("poller[%s] starting worker (id=%s)", s.Name, s.ID)
				start(s)
			case isDead(existing):
				log.Printf("poller[%s] previous worker exited, restarting", s.Name)
				delete(running, s.ID)
				start(s)
			case !existing.updatedAt.Equal(s.UpdatedAt):
				log.Printf("poller[%s] config changed, restarting worker", s.Name)
				existing.cancel()
				<-existing.alive // wait for old worker to fully exit before starting new one
				start(s)
			}
		}
		for id, state := range running {
			if !active[id] {
				log.Printf("poller: server %s removed, stopping worker", id)
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

func (p *Pool) serverWorker(ctx context.Context, s models.Server) {
	log.Printf("poller[%s] starting", s.Name)
	password, err := crypto.Decrypt(s.Password)
	if err != nil {
		log.Printf("poller[%s] decrypt error: %v", s.Name, err)
		return
	}
	client := redfish.NewClient(s.Hostname, s.Port, s.Username, password, s.TLSVerify)

	// Try SSE first
	sseCh := make(chan redfish.SSEEvent, 64)
	sseCtx, cancelSSE := context.WithCancel(ctx)
	defer cancelSSE()
	go func() {
		client.SSEListener(sseCtx, s.ID, sseCh, p.cfg.Polling.SSEReconnectMaxRetries)
		close(sseCh)
	}()

	systemTicker := time.NewTicker(time.Duration(p.cfg.Polling.SystemIntervalSeconds) * time.Second)
	thermalTicker := time.NewTicker(time.Duration(p.cfg.Polling.ThermalIntervalSeconds) * time.Second)
	powerTicker := time.NewTicker(time.Duration(p.cfg.Polling.PowerIntervalSeconds) * time.Second)
	firmwareTicker := time.NewTicker(time.Duration(p.cfg.Polling.FirmwareIntervalHours) * time.Hour)
	storageTicker := time.NewTicker(time.Duration(p.cfg.Polling.StorageIntervalMinutes) * time.Minute)
	jobTicker := time.NewTicker(time.Duration(p.cfg.Polling.JobQueueIntervalSeconds) * time.Second)
	defer func() {
		systemTicker.Stop(); thermalTicker.Stop(); powerTicker.Stop()
		firmwareTicker.Stop(); storageTicker.Stop(); jobTicker.Stop()
	}()

	// Initial poll — fetch everything once at startup so the UI has data immediately,
	// without waiting for the storage (10 min) or firmware (6h) tickers. All polls run
	// in parallel so a single slow iDRAC doesn't delay the others.
	var wg sync.WaitGroup
	pollers := []func(*redfish.Client, string, string){
		p.pollSystem, p.pollThermal, p.pollPower, p.pollStorage, p.pollFirmware,
	}
	for _, fn := range pollers {
		wg.Add(1)
		go func(f func(*redfish.Client, string, string)) {
			defer wg.Done()
			f(client, s.ID, s.Name)
		}(fn)
	}
	wg.Wait()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-sseCh:
			if !ok {
				// Channel closed — SSEListener exhausted retries.
				// Nil it out so this case never fires again (nil channel blocks forever).
				sseCh = nil
			} else {
				p.handleSSEEvent(event)
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

// safePoll wraps a poll func with panic recovery so a bad payload from one server
// doesn't kill the worker goroutine.
func (p *Pool) safePoll(name, kind string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("poller[%s] %s PANIC: %v", name, kind, r)
		}
	}()
	fn()
}

func (p *Pool) pollSystem(client *redfish.Client, serverID, name string) {
	p.safePoll(name, "system", func() {
		sys, err := client.GetSystem()
		if err != nil {
			p.setStatus(serverID, "offline")
			p.hub.Emit("server_status", serverID, map[string]string{"status": "offline"})
			log.Printf("poller[%s] system error: %v", name, err)
			return
		}
		data, _ := json.Marshal(sys)
		res, dbErr := p.db.Exec(`UPDATE server_cache SET system_json=?, last_seen=?, status='online' WHERE server_id=?`,
			string(data), time.Now(), serverID)
		if dbErr != nil {
			log.Printf("poller[%s] system db error: %v", name, dbErr)
			return
		}
		if rows, _ := res.RowsAffected(); rows == 0 {
			// cache row missing — recreate it so next poll succeeds
			log.Printf("poller[%s] system: no cache row, inserting", name)
			p.db.Exec(`INSERT OR IGNORE INTO server_cache (server_id, status) VALUES (?, 'online')`, serverID)
			p.db.Exec(`UPDATE server_cache SET system_json=?, last_seen=?, status='online' WHERE server_id=?`,
				string(data), time.Now(), serverID)
		}
		log.Printf("poller[%s] system OK: %s / %s / %s", name, sys.Model, sys.SerialNumber, sys.PowerState)
		p.hub.Emit("server_status", serverID, map[string]string{"status": "online", "power_state": sys.PowerState})
		p.hub.Emit("power_state", serverID, map[string]string{"state": sys.PowerState})
	})
}

func (p *Pool) pollThermal(client *redfish.Client, serverID, name string) {
	p.safePoll(name, "thermal", func() {
		t, err := client.GetThermal()
		if err != nil {
			log.Printf("poller[%s] thermal error: %v", name, err)
			return
		}
		data, _ := json.Marshal(t)
		p.db.Exec(`UPDATE server_cache SET thermal_json=? WHERE server_id=?`, string(data), serverID)

		inletTemp := 0.0
		for _, ts := range t.Temperatures {
			if ts.Name == "Inlet Temp" {
				inletTemp = ts.ReadingCelsius
				break
			}
		}
		p.hub.Emit("thermal_update", serverID, map[string]float64{"inlet_temp": inletTemp})
	})
}

func (p *Pool) pollPower(client *redfish.Client, serverID, name string) {
	p.safePoll(name, "power", func() {
		pw, err := client.GetPower()
		if err != nil {
			log.Printf("poller[%s] power error: %v", name, err)
			return
		}
		data, _ := json.Marshal(pw)
		p.db.Exec(`UPDATE server_cache SET power_json=? WHERE server_id=?`, string(data), serverID)
	})
}

func (p *Pool) pollFirmware(client *redfish.Client, serverID, name string) {
	p.safePoll(name, "firmware", func() {
		fw, err := client.GetFirmwareInventory()
		if err != nil {
			log.Printf("poller[%s] firmware error: %v", name, err)
			return
		}
		data, _ := json.Marshal(fw)
		p.db.Exec(`UPDATE server_cache SET firmware_json=? WHERE server_id=?`, string(data), serverID)
	})
}

func (p *Pool) pollStorage(client *redfish.Client, serverID, name string) {
	p.safePoll(name, "storage", func() {
		st, err := client.GetStorage()
		if err != nil {
			log.Printf("poller[%s] storage error: %v", name, err)
			return
		}
		data, _ := json.Marshal(st)
		p.db.Exec(`UPDATE server_cache SET storage_json=? WHERE server_id=?`, string(data), serverID)
	})
}

func (p *Pool) checkJobs(client *redfish.Client, serverID string) {
	var jobs []models.Job
	p.db.Select(&jobs, `SELECT * FROM jobs WHERE server_id=? AND status IN ('queued','running') AND idrac_job_id IS NOT NULL`, serverID)

	for _, job := range jobs {
		if job.IDRACJobID == nil {
			continue
		}
		jid := lastSegment(*job.IDRACJobID)
		iJob, err := client.GetJob(jid)
		if err != nil {
			continue
		}
		p.hub.Emit("job_update", serverID, map[string]interface{}{
			"job_id":   job.ID,
			"status":   iJob.JobState,
			"percent":  iJob.PercentComplete,
			"message":  iJob.Message,
		})
		if redfish.IsJobDone(iJob.JobState) {
			status := string(models.JobStatusDone)
			if iJob.JobState == "Failed" {
				status = string(models.JobStatusFailed)
			}
			now := time.Now()
			result, _ := json.Marshal(iJob)
			rs := string(result)
			p.db.Exec(`UPDATE jobs SET status=?, result=?, finished_at=? WHERE id=?`,
				status, rs, now, job.ID)
			if job.Type == models.JobTypeBiosConfig {
				p.hub.Emit("bios_job_done", serverID, map[string]interface{}{
					"job_id":  job.ID,
					"success": iJob.JobState == "Completed",
				})
			}
		}
	}
}

func (p *Pool) setStatus(serverID, status string) {
	p.db.Exec(`UPDATE server_cache SET status=? WHERE server_id=?`, status, serverID)
}

func (p *Pool) handleSSEEvent(e redfish.SSEEvent) {
	p.hub.Emit("sse_event", e.ServerID, map[string]string{
		"type": e.EventType,
		"data": e.Data,
	})
}

// lastSegment returns the last path segment of a URL/path string.
func lastSegment(s string) string {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return s[i+1:]
		}
	}
	return s
}
