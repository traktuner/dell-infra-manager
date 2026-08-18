package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/dell-infra-manager/backend/crypto"
	"github.com/dell-infra-manager/backend/models"
	"github.com/dell-infra-manager/backend/redfish"
)

const dellDownloadBase = "https://downloads.dell.com/"
const maxFirmwarePackageSize = int64(1024 * 1024 * 1024)

// firmwareJobWorker continuously processes queued firmware update jobs.
func (p *Pool) firmwareJobWorker(ctx context.Context) {
	p.recoverInterruptedFirmwareJobs()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.processNextFirmwareJob(ctx)
		}
	}
}

func (p *Pool) recoverInterruptedFirmwareJobs() {
	now := time.Now()
	result, err := p.db.Exec(`UPDATE jobs SET status=?, result=?, finished_at=? WHERE type=? AND status=?`,
		string(models.JobStatusFailed),
		"appliance stopped while this firmware job was running; inspect the iDRAC job queue before retrying",
		now, string(models.JobTypeFirmwareUpdate), string(models.JobStatusRunning))
	if err != nil {
		log.Printf("recover interrupted firmware jobs: %v", err)
		return
	}
	if rows, _ := result.RowsAffected(); rows > 0 {
		log.Printf("marked %d interrupted firmware job(s) as failed", rows)
	}
}

func (p *Pool) processNextFirmwareJob(ctx context.Context) {
	var job models.Job
	if err := p.db.Get(&job, `SELECT * FROM jobs WHERE type=? AND status=? ORDER BY created_at LIMIT 1`,
		string(models.JobTypeFirmwareUpdate), string(models.JobStatusQueued)); err != nil {
		return
	}
	result, err := p.db.Exec(`UPDATE jobs SET status=?, started_at=? WHERE id=? AND status=?`,
		string(models.JobStatusRunning), time.Now(), job.ID, string(models.JobStatusQueued))
	if err != nil {
		log.Printf("firmware job[%s] claim failed: %v", job.ID, err)
		return
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return
	}
	p.executeFirmwareJob(ctx, job)
}

func (p *Pool) executeFirmwareJob(ctx context.Context, job models.Job) {
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
	firmwareDir := filepath.Dir(p.cfg.Database.Path)
	dupFile, size, filename, err := downloadDUP(ctx, payload.CatalogPath, firmwareDir)
	if err != nil {
		p.failJob(job.ID, "download DUP: "+err.Error())
		return
	}
	defer func() {
		name := dupFile.Name()
		_ = dupFile.Close()
		_ = os.Remove(name)
	}()

	// Upload to iDRAC
	p.hub.Emit("job_update", job.ServerID, map[string]interface{}{
		"job_id": job.ID, "status": "running", "percent": 40, "message": "Uploading to iDRAC...",
	})
	applyTime := "OnReset"
	location, err := client.UploadFirmware(filename, dupFile, size, applyTime)
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

func downloadDUP(ctx context.Context, catalogPath, tempDir string) (*os.File, int64, string, error) {
	downloadURL, filename, err := validatedDellDownloadURL(catalogPath)
	if err != nil {
		return nil, 0, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, 0, "", err
	}
	client := &http.Client{Timeout: 20 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, 0, "", fmt.Errorf("download failed: %d", resp.StatusCode)
	}
	if resp.ContentLength > maxFirmwarePackageSize {
		return nil, 0, "", fmt.Errorf("firmware package is too large: %d bytes", resp.ContentLength)
	}
	if err := os.MkdirAll(tempDir, 0o750); err != nil {
		return nil, 0, "", err
	}
	f, err := os.CreateTemp(tempDir, ".firmware-*.dup")
	if err != nil {
		return nil, 0, "", err
	}
	cleanup := func(e error) (*os.File, int64, string, error) {
		name := f.Name()
		_ = f.Close()
		_ = os.Remove(name)
		return nil, 0, "", e
	}
	written, err := io.Copy(f, io.LimitReader(resp.Body, maxFirmwarePackageSize+1))
	if err != nil {
		return cleanup(err)
	}
	if written > maxFirmwarePackageSize {
		return cleanup(fmt.Errorf("firmware package exceeds %d bytes", maxFirmwarePackageSize))
	}
	if written == 0 {
		return cleanup(fmt.Errorf("firmware package is empty"))
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return cleanup(err)
	}
	return f, written, filename, nil
}

func validatedDellDownloadURL(catalogPath string) (string, string, error) {
	if strings.Contains(catalogPath, "\\") {
		return "", "", fmt.Errorf("invalid catalog path")
	}
	rel, err := url.Parse(strings.TrimSpace(catalogPath))
	if err != nil || rel.IsAbs() || rel.Host != "" || rel.RawQuery != "" || rel.Fragment != "" {
		return "", "", fmt.Errorf("invalid catalog path")
	}
	for _, segment := range strings.Split(rel.Path, "/") {
		if segment == "." || segment == ".." {
			return "", "", fmt.Errorf("invalid catalog path")
		}
	}
	cleanPath := path.Clean("/" + rel.Path)
	if cleanPath == "/" {
		return "", "", fmt.Errorf("invalid catalog path")
	}
	base, _ := url.Parse(dellDownloadBase)
	rel.Path = strings.TrimPrefix(cleanPath, "/")
	return base.ResolveReference(rel).String(), path.Base(cleanPath), nil
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
