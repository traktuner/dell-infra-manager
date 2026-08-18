package api

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dell-infra-manager/backend/buildinfo"
	"github.com/dell-infra-manager/backend/models"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

const (
	defaultUpdateRepository = "traktuner/dell-infra-manager"
	applianceExecutable     = "/opt/dell-infra-manager/dell-infra-manager"
	maxReleaseBinarySize    = int64(256 * 1024 * 1024)
)

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type githubRelease struct {
	TagName string        `json:"tag_name"`
	HTMLURL string        `json:"html_url"`
	Assets  []githubAsset `json:"assets"`
}

type ApplianceHandler struct {
	db         *sqlx.DB
	repository string
	port       int
	client     *http.Client
	cacheMu    sync.Mutex
	cached     *githubRelease
	cachedAt   time.Time
	applyMu    sync.Mutex
	applying   bool
}

func NewApplianceHandler(db *sqlx.DB, port string) *ApplianceHandler {
	repository := strings.TrimSpace(os.Getenv("UPDATE_REPO"))
	if repository == "" {
		repository = defaultUpdateRepository
	}
	parsedPort, err := strconv.Atoi(port)
	if err != nil || parsedPort < 1 || parsedPort > 65535 {
		parsedPort = 8080
	}
	return &ApplianceHandler{
		db:         db,
		repository: repository,
		port:       parsedPort,
		client:     &http.Client{Timeout: 5 * time.Minute},
	}
}

func Health(c *gin.Context) {
	hash, err := buildinfo.BinarySHA256()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status": "ok", "version": buildinfo.Version, "commit": buildinfo.Commit,
		"binary_sha256": hash,
	})
}

func (h *ApplianceHandler) GetUpdateStatus(c *gin.Context) {
	release, err := h.latestRelease(c.Request.Context(), false)
	status := h.statusResponse(release)
	if err != nil {
		status["check_error"] = err.Error()
	}
	c.JSON(http.StatusOK, status)
}

func (h *ApplianceHandler) ApplyUpdate(c *gin.Context) {
	if !h.supported() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "self-update is available only in the OpenRC LXC appliance"})
		return
	}
	activeJobs, err := h.activeFirmwareJobs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "check active firmware jobs: " + err.Error()})
		return
	}
	if activeJobs > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "finish or remove all queued and running firmware jobs before updating the appliance"})
		return
	}
	h.applyMu.Lock()
	if h.applying {
		h.applyMu.Unlock()
		c.JSON(http.StatusConflict, gin.H{"error": "an appliance update is already running"})
		return
	}
	h.applying = true
	h.applyMu.Unlock()
	defer func() {
		h.applyMu.Lock()
		h.applying = false
		h.applyMu.Unlock()
	}()

	release, err := h.latestRelease(c.Request.Context(), true)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "check latest release: " + err.Error()})
		return
	}
	result, err := h.installRelease(c.Request.Context(), release)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if !result.updated {
		c.JSON(http.StatusOK, gin.H{"updated": false, "version": release.TagName})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{
		"updated": true, "version": release.TagName, "binary_sha256": result.hash,
		"message": "the appliance service will restart; managed servers are not restarted",
	})
}

func (h *ApplianceHandler) statusResponse(release *githubRelease) gin.H {
	currentHash, _ := buildinfo.BinarySHA256()
	resp := gin.H{
		"supported": h.supported(), "current_version": buildinfo.Version,
		"current_commit": buildinfo.Commit, "current_sha256": currentHash,
		"repository": h.repository,
	}
	if activeJobs, err := h.activeFirmwareJobs(); err == nil {
		resp["active_firmware_jobs"] = activeJobs
	}
	if release != nil {
		resp["latest_version"] = release.TagName
		resp["release_url"] = release.HTMLURL
		resp["update_available"] = buildinfo.Version == "dev" || release.TagName != buildinfo.Version
		resp["checked_at"] = time.Now().UTC().Format(time.RFC3339)
	}
	return resp
}

func (h *ApplianceHandler) activeFirmwareJobs() (int, error) {
	if h.db == nil {
		return 0, nil
	}
	var count int
	err := h.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE type=? AND status IN ('queued','running')`,
		string(models.JobTypeFirmwareUpdate)).Scan(&count)
	return count, err
}

func (h *ApplianceHandler) supported() bool {
	exe, err := os.Executable()
	if err != nil || filepath.Clean(exe) != applianceExecutable {
		return false
	}
	_, err = exec.LookPath("rc-service")
	return err == nil
}

func (h *ApplianceHandler) latestRelease(ctx context.Context, force bool) (*githubRelease, error) {
	h.cacheMu.Lock()
	defer h.cacheMu.Unlock()
	if !force && h.cached != nil && time.Since(h.cachedAt) < 5*time.Minute {
		copy := *h.cached
		return &copy, nil
	}
	url := "https://api.github.com/repos/" + h.repository + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "dell-infra-manager/"+buildinfo.Version)
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub returned HTTP %d", resp.StatusCode)
	}
	var release githubRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2*1024*1024)).Decode(&release); err != nil {
		return nil, err
	}
	if release.TagName == "" {
		return nil, fmt.Errorf("latest release has no tag")
	}
	h.cached = &release
	h.cachedAt = time.Now()
	copy := release
	return &copy, nil
}

type installResult struct {
	updated bool
	hash    string
}

func (h *ApplianceHandler) installRelease(ctx context.Context, release *githubRelease) (installResult, error) {
	binaryName := "dell-infra-manager-linux-" + runtime.GOARCH
	binaryAsset, ok := findAsset(release.Assets, binaryName)
	if !ok {
		return installResult{}, fmt.Errorf("release %s has no %s asset", release.TagName, binaryName)
	}
	checksumAsset, ok := findAsset(release.Assets, "SHA256SUMS")
	if !ok {
		return installResult{}, fmt.Errorf("release %s has no SHA256SUMS asset", release.TagName)
	}
	checksums, err := h.downloadBytes(ctx, checksumAsset.BrowserDownloadURL, 1024*1024)
	if err != nil {
		return installResult{}, fmt.Errorf("download release checksums: %w", err)
	}
	expectedHash, err := checksumForAsset(checksums, binaryName)
	if err != nil {
		return installResult{}, err
	}
	newPath, actualHash, err := h.downloadBinary(ctx, binaryAsset)
	if err != nil {
		return installResult{}, err
	}
	defer os.Remove(newPath)
	if actualHash != expectedHash {
		return installResult{}, fmt.Errorf("release checksum mismatch for %s", binaryName)
	}
	currentHash, err := buildinfo.BinarySHA256()
	if err == nil && currentHash == actualHash {
		return installResult{hash: actualHash}, nil
	}

	stat, err := os.Stat(applianceExecutable)
	if err != nil {
		return installResult{}, fmt.Errorf("inspect current appliance binary: %w", err)
	}
	if err := os.Chmod(newPath, stat.Mode().Perm()); err != nil {
		return installResult{}, fmt.Errorf("set update permissions: %w", err)
	}
	previous := applianceExecutable + ".previous"
	if err := copyFileAtomic(applianceExecutable, previous, stat.Mode().Perm()); err != nil {
		return installResult{}, fmt.Errorf("back up current appliance binary: %w", err)
	}
	if err := os.Rename(newPath, applianceExecutable); err != nil {
		return installResult{}, fmt.Errorf("install appliance binary: %w", err)
	}
	if err := h.startRestartHelper(actualHash, previous); err != nil {
		_ = copyFileAtomic(previous, applianceExecutable, stat.Mode().Perm())
		return installResult{}, fmt.Errorf("start verified appliance restart: %w", err)
	}
	return installResult{updated: true, hash: actualHash}, nil
}

func findAsset(assets []githubAsset, name string) (githubAsset, bool) {
	for _, asset := range assets {
		if asset.Name == name {
			return asset, true
		}
	}
	return githubAsset{}, false
}

func checksumForAsset(data []byte, name string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			hash := strings.ToLower(fields[0])
			if len(hash) != 64 {
				break
			}
			if _, err := hex.DecodeString(hash); err != nil {
				break
			}
			return hash, nil
		}
	}
	return "", fmt.Errorf("SHA256SUMS has no valid checksum for %s", name)
}

func (h *ApplianceHandler) downloadBytes(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("download exceeds %d bytes", limit)
	}
	return data, nil
}

func (h *ApplianceHandler) downloadBinary(ctx context.Context, asset githubAsset) (string, string, error) {
	if asset.Size <= 0 || asset.Size > maxReleaseBinarySize {
		return "", "", fmt.Errorf("release binary size %d is outside the allowed range", asset.Size)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("download release binary: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("download release binary: HTTP %d", resp.StatusCode)
	}
	f, err := os.CreateTemp(filepath.Dir(applianceExecutable), ".dell-infra-manager-*.new")
	if err != nil {
		return "", "", err
	}
	name := f.Name()
	cleanup := func(e error) (string, string, error) {
		_ = f.Close()
		_ = os.Remove(name)
		return "", "", e
	}
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(f, hash), io.LimitReader(resp.Body, maxReleaseBinarySize+1))
	if err != nil {
		return cleanup(err)
	}
	if written > maxReleaseBinarySize || written != asset.Size {
		return cleanup(fmt.Errorf("release binary size mismatch: got %d, expected %d", written, asset.Size))
	}
	if err := f.Sync(); err != nil {
		return cleanup(err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return cleanup(err)
	}
	magic := make([]byte, 4)
	if _, err := io.ReadFull(f, magic); err != nil || string(magic) != "\x7fELF" {
		return cleanup(fmt.Errorf("release asset is not an ELF binary"))
	}
	if err := f.Close(); err != nil {
		return cleanup(err)
	}
	return name, hex.EncodeToString(hash.Sum(nil)), nil
}

func copyFileAtomic(source, target string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(target), ".backup-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, target)
}

func (h *ApplianceHandler) startRestartHelper(expectedHash, previous string) error {
	script, err := os.CreateTemp("/tmp", "dell-infra-manager-update-*.sh")
	if err != nil {
		return err
	}
	scriptPath := script.Name()
	body := fmt.Sprintf(`#!/bin/sh
set -eu
EXPECTED='%s'
BIN='%s'
PREVIOUS='%s'
PORT='%d'
LOG='/tmp/dell-infra-manager-update.log'
{
  sleep 1
  rc-service dell-infra-manager restart
  for i in $(seq 1 30); do
    BODY=$(wget -qO- "http://127.0.0.1:$PORT/healthz?u=$(date +%%s)" 2>/dev/null || true)
    echo "$BODY" | grep -q "$EXPECTED" && { echo "Update verified: $EXPECTED"; rm -f "$0"; exit 0; }
    sleep 1
  done
  echo "Updated service did not verify; rolling back"
  cp "$PREVIOUS" "$BIN.rollback"
  chmod +x "$BIN.rollback"
  mv "$BIN.rollback" "$BIN"
  rc-service dell-infra-manager restart || true
  rm -f "$0"
  exit 1
} >> "$LOG" 2>&1
`, expectedHash, applianceExecutable, previous, h.port)
	if _, err := script.WriteString(body); err != nil {
		_ = script.Close()
		_ = os.Remove(scriptPath)
		return err
	}
	if err := script.Chmod(0o700); err != nil {
		_ = script.Close()
		_ = os.Remove(scriptPath)
		return err
	}
	if err := script.Close(); err != nil {
		_ = os.Remove(scriptPath)
		return err
	}
	cmd := exec.Command(scriptPath)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		_ = os.Remove(scriptPath)
		return err
	}
	return nil
}
