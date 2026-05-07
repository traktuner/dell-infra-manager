package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/dell-infra-manager/backend/crypto"
	"github.com/dell-infra-manager/backend/models"
	"github.com/dell-infra-manager/backend/redfish"
)

const dellDownloadBase = "https://downloads.dell.com/"

// firmwareJobWorker continuously processes queued firmware update jobs.
func (p *Pool) firmwareJobWorker(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.processPendingFirmwareJobs(ctx)
		}
	}
}

func (p *Pool) processPendingFirmwareJobs(ctx context.Context) {
	var jobs []models.Job
	p.db.Select(&jobs, `SELECT * FROM jobs WHERE type=? AND status=? LIMIT 5`,
		string(models.JobTypeFirmwareUpdate), string(models.JobStatusQueued))

	for _, job := range jobs {
		select {
		case <-ctx.Done():
			return
		default:
		}
		go p.executeFirmwareJob(ctx, job)
	}
}

func (p *Pool) executeFirmwareJob(ctx context.Context, job models.Job) {
	now := time.Now()
	p.db.Exec(`UPDATE jobs SET status=?, started_at=? WHERE id=?`,
		string(models.JobStatusRunning), now, job.ID)
	p.hub.Emit("job_update", job.ServerID, map[string]interface{}{
		"job_id": job.ID, "status": "running", "percent": 0,
	})

	var payload models.FirmwareUpdatePayload
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil {
		p.failJob(job.ID, "invalid payload: "+err.Error())
		return
	}

	client, err := p.buildClientForServer(job.ServerID)
	if err != nil {
		p.failJob(job.ID, "build client: "+err.Error())
		return
	}

	// Download DUP file from Dell
	p.hub.Emit("job_update", job.ServerID, map[string]interface{}{
		"job_id": job.ID, "status": "running", "percent": 10, "message": "Downloading DUP...",
	})
	dupData, filename, err := downloadDUP(ctx, payload.CatalogPath)
	if err != nil {
		p.failJob(job.ID, "download DUP: "+err.Error())
		return
	}

	// Upload to iDRAC
	p.hub.Emit("job_update", job.ServerID, map[string]interface{}{
		"job_id": job.ID, "status": "running", "percent": 40, "message": "Uploading to iDRAC...",
	})
	applyTime := "OnReset"
	location, err := client.UploadFirmware(filename, dupData, applyTime)
	if err != nil {
		p.failJob(job.ID, "upload firmware: "+err.Error())
		return
	}

	// Save iDRAC job ID
	p.db.Exec(`UPDATE jobs SET idrac_job_id=? WHERE id=?`, location, job.ID)
	log.Printf("firmware job[%s] iDRAC location: %s", job.ID, location)
	// The checkJobs polling loop will track the iDRAC job to completion.
}

func (p *Pool) failJob(jobID, reason string) {
	now := time.Now()
	p.db.Exec(`UPDATE jobs SET status=?, result=?, finished_at=? WHERE id=?`,
		string(models.JobStatusFailed), reason, now, jobID)
}

func downloadDUP(ctx context.Context, catalogPath string) ([]byte, string, error) {
	url := dellDownloadBase + catalogPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("download failed: %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	// Extract filename from path
	filename := lastSegment(catalogPath)
	return data, filename, nil
}

func (p *Pool) buildClientForServer(serverID string) (*redfish.Client, error) {
	var s models.Server
	if err := p.db.Get(&s, `SELECT * FROM servers WHERE id = ?`, serverID); err != nil {
		return nil, err
	}
	password, err := crypto.Decrypt(s.Password)
	if err != nil {
		return nil, err
	}
	return redfish.NewClient(s.Hostname, s.Port, s.Username, password, s.TLSVerify), nil
}

// catalogUpdater downloads the Dell catalog daily at the configured hour.
func (p *Pool) catalogUpdater(ctx context.Context) {
	// Run once on startup if catalog doesn't exist
	p.maybeDownloadCatalog()

	for {
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(),
			p.cfg.Polling.CatalogUpdateHour, 0, 0, 0, now.Location())
		if next.Before(now) {
			next = next.Add(24 * time.Hour)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(next)):
			p.maybeDownloadCatalog()
		}
	}
}

func (p *Pool) maybeDownloadCatalog() {
	// Skip download if we have a cached copy younger than 24 hours.
	if info, err := os.Stat(p.cfg.Dell.CachePath); err == nil {
		age := time.Since(info.ModTime())
		if age < 24*time.Hour {
			log.Printf("catalog: using cached copy (age %v)", age.Round(time.Minute))
			return
		}
	}
	log.Println("catalog: downloading...")
	if err := redfish.DownloadCatalog(p.cfg.Dell.CatalogURL, p.cfg.Dell.CachePath); err != nil {
		log.Printf("catalog: download failed: %v", err)
		return
	}
	log.Println("catalog: download complete")
}

