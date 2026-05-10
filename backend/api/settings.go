package api

import (
	"net/http"

	"github.com/dell-infra-manager/backend/models"
	"github.com/dell-infra-manager/backend/notifier"
	"github.com/gin-gonic/gin"
)

type SettingsHandler struct {
	notifier *notifier.Notifier
}

func NewSettingsHandler(n *notifier.Notifier) *SettingsHandler {
	return &SettingsHandler{notifier: n}
}

// GetNotifications returns the current SMTP/notification config.
// The password is never returned — only a flag indicating whether one is set.
func (h *SettingsHandler) GetNotifications(c *gin.Context) {
	s, err := h.notifier.LoadSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, s)
}

// updateNotificationsRequest is the input shape from the Settings UI. The
// optional plaintext password field lets the user (re)set credentials without
// us ever returning the existing one.
type updateNotificationsRequest struct {
	Enabled           bool   `json:"enabled"`
	SMTPHost          string `json:"smtp_host"`
	SMTPPort          int    `json:"smtp_port"`
	SMTPUsername      string `json:"smtp_username"`
	SMTPPassword      string `json:"smtp_password,omitempty"` // empty = keep existing
	SMTPFrom          string `json:"smtp_from"`
	SMTPTLS           string `json:"smtp_tls"`
	Recipients        string `json:"recipients"` // JSON array as string
	OnServerOffline   bool   `json:"on_server_offline"`
	OnHealthCritical  bool   `json:"on_health_critical"`
	OnJobFailed       bool   `json:"on_job_failed"`
	OnFirmwareUpdates bool   `json:"on_firmware_updates"`
}

func (h *SettingsHandler) UpdateNotifications(c *gin.Context) {
	var req updateNotificationsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.SMTPPort == 0 {
		req.SMTPPort = 587
	}
	if req.SMTPTLS == "" {
		req.SMTPTLS = "starttls"
	}
	settings := &models.NotificationSettings{
		Enabled:           req.Enabled,
		SMTPHost:          req.SMTPHost,
		SMTPPort:          req.SMTPPort,
		SMTPUsername:      req.SMTPUsername,
		SMTPFrom:          req.SMTPFrom,
		SMTPTLS:           req.SMTPTLS,
		Recipients:        req.Recipients,
		OnServerOffline:   req.OnServerOffline,
		OnHealthCritical:  req.OnHealthCritical,
		OnJobFailed:       req.OnJobFailed,
		OnFirmwareUpdates: req.OnFirmwareUpdates,
	}
	if err := h.notifier.SaveSettings(settings, req.SMTPPassword); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// firmwareDigestRunner is set by main.go to wire the daily firmware-digest
// runner into this handler so the "Send digest now" button can trigger it
// on demand, without us pulling worker into api as a dep cycle.
var firmwareDigestRunner func() = func() {}

// SetFirmwareDigestRunner registers the function the "Send digest now" button
// calls. Wired up in main.go after both the worker and the API are constructed.
func SetFirmwareDigestRunner(fn func()) { firmwareDigestRunner = fn }

// SendDigestNow fires the firmware-update digest immediately, ignoring the
// daily schedule. Useful for "did my SMTP setup work?" testing without
// waiting until tomorrow morning.
func (h *SettingsHandler) SendDigestNow(c *gin.Context) {
	go firmwareDigestRunner()
	c.JSON(http.StatusAccepted, gin.H{"ok": true})
}

// TestNotifications fires off a test email using the request body's settings —
// useful before saving. If smtp_password is empty we use the stored one.
func (h *SettingsHandler) TestNotifications(c *gin.Context) {
	var req updateNotificationsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.SMTPPort == 0 {
		req.SMTPPort = 587
	}
	if req.SMTPTLS == "" {
		req.SMTPTLS = "starttls"
	}
	// We need the encrypted password if user didn't provide a fresh one — load
	// it from the settings row.
	stored, err := h.notifier.LoadSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	settings := &models.NotificationSettings{
		SMTPHost:     req.SMTPHost,
		SMTPPort:     req.SMTPPort,
		SMTPUsername: req.SMTPUsername,
		SMTPPassword: stored.SMTPPassword,
		SMTPFrom:     req.SMTPFrom,
		SMTPTLS:      req.SMTPTLS,
		Recipients:   req.Recipients,
	}
	if err := h.notifier.TestSend(settings, req.SMTPPassword); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
