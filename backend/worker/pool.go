package worker

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/dell-manager/backend/api"
	"github.com/dell-manager/backend/config"
	"github.com/dell-manager/backend/crypto"
	"github.com/dell-manager/backend/models"
	"github.com/dell-manager/backend/redfish"
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
	cancel context.CancelFunc
}

func (p *Pool) serverWatcher(ctx context.Context) {
	running := make(map[string]*serverWorkerState)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			for _, s := range running {
				s.cancel()
			}
			return
		case <-ticker.C:
			var servers []models.Server
			if err := p.db.Select(&servers, `SELECT * FROM servers`); err != nil {
				continue
			}
			// Start workers for new servers
			active := make(map[string]bool)
			for _, s := range servers {
				active[s.ID] = true
				if _, ok := running[s.ID]; !ok {
					sCtx, cancel := context.WithCancel(ctx)
					running[s.ID] = &serverWorkerState{cancel: cancel}
					go p.serverWorker(sCtx, s)
				}
			}
			// Cancel workers for deleted servers
			for id, state := range running {
				if !active[id] {
					state.cancel()
					delete(running, id)
				}
			}
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

	// Initial poll
	p.pollSystem(client, s.ID, s.Name)
	p.pollThermal(client, s.ID, s.Name)
	p.pollPower(client, s.ID, s.Name)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-sseCh:
			if ok {
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

func (p *Pool) pollSystem(client *redfish.Client, serverID, name string) {
	sys, err := client.GetSystem()
	if err != nil {
		p.setStatus(serverID, "offline")
		p.hub.Emit("server_status", serverID, map[string]string{"status": "offline"})
		log.Printf("poller[%s] system error: %v", name, err)
		return
	}
	data, _ := json.Marshal(sys)
	p.db.Exec(`UPDATE server_cache SET system_json=?, last_seen=?, status='online' WHERE server_id=?`,
		string(data), time.Now(), serverID)
	p.hub.Emit("server_status", serverID, map[string]string{"status": "online", "power_state": sys.PowerState})
	p.hub.Emit("power_state", serverID, map[string]string{"state": sys.PowerState})
}

func (p *Pool) pollThermal(client *redfish.Client, serverID, name string) {
	t, err := client.GetThermal()
	if err != nil {
		log.Printf("poller[%s] thermal error: %v", name, err)
		return
	}
	data, _ := json.Marshal(t)
	p.db.Exec(`UPDATE server_cache SET thermal_json=? WHERE server_id=?`, string(data), serverID)

	// Find inlet temp for WS event
	inletTemp := 0.0
	for _, ts := range t.Temperatures {
		if ts.Name == "Inlet Temp" {
			inletTemp = ts.ReadingCelsius
			break
		}
	}
	p.hub.Emit("thermal_update", serverID, map[string]float64{"inlet_temp": inletTemp})
}

func (p *Pool) pollPower(client *redfish.Client, serverID, name string) {
	pw, err := client.GetPower()
	if err != nil {
		log.Printf("poller[%s] power error: %v", name, err)
		return
	}
	data, _ := json.Marshal(pw)
	p.db.Exec(`UPDATE server_cache SET power_json=? WHERE server_id=?`, string(data), serverID)
}

func (p *Pool) pollFirmware(client *redfish.Client, serverID, name string) {
	fw, err := client.GetFirmwareInventory()
	if err != nil {
		log.Printf("poller[%s] firmware error: %v", name, err)
		return
	}
	data, _ := json.Marshal(fw)
	p.db.Exec(`UPDATE server_cache SET firmware_json=? WHERE server_id=?`, string(data), serverID)
}

func (p *Pool) pollStorage(client *redfish.Client, serverID, name string) {
	st, err := client.GetStorage()
	if err != nil {
		log.Printf("poller[%s] storage error: %v", name, err)
		return
	}
	data, _ := json.Marshal(st)
	p.db.Exec(`UPDATE server_cache SET storage_json=? WHERE server_id=?`, string(data), serverID)
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
