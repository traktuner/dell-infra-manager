// Package notifier sends SMTP email alerts for important events.
// Settings live in the notification_settings DB row (single-row table).
//
// Event types currently supported:
//   - "server_offline"     → server became unreachable
//   - "health_critical"    → hardware health flipped to Critical
//   - "job_failed"         → an iDRAC job ended with state Failed
//   - "firmware_updates"   → new firmware versions available (digest, daily)
//
// Each event is deduped per (server_id, event_type) for the cooldown window
// so a flapping server doesn't email on every poll.
package notifier

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/smtp"
	"strings"
	"time"

	"github.com/dell-infra-manager/backend/crypto"
	"github.com/dell-infra-manager/backend/models"
	"github.com/jmoiron/sqlx"
)

// DedupCooldown caps how often we send the same event-for-the-same-server.
// 1 hour is a reasonable balance between "actionable" and "not spammy" for
// homelab/small fleet use. Override per deployment if needed.
const DedupCooldown = 1 * time.Hour

type Notifier struct {
	db *sqlx.DB
}

func New(db *sqlx.DB) *Notifier {
	return &Notifier{db: db}
}

// LoadSettings returns the current SMTP configuration. Always non-nil thanks
// to the seed row inserted by the migration.
func (n *Notifier) LoadSettings() (*models.NotificationSettings, error) {
	var s models.NotificationSettings
	if err := n.db.Get(&s, `SELECT * FROM notification_settings WHERE id = 1`); err != nil {
		return nil, err
	}
	s.HasPassword = s.SMTPPassword != ""
	return &s, nil
}

// SaveSettings persists the user-edited config. The plaintext password (if
// any) is encrypted with the same key as iDRAC credentials. Pass empty
// password to keep the existing one.
func (n *Notifier) SaveSettings(in *models.NotificationSettings, plainPassword string) error {
	encPassword := ""
	switch {
	case plainPassword != "":
		var err error
		encPassword, err = crypto.Encrypt(plainPassword)
		if err != nil {
			return fmt.Errorf("encrypt smtp password: %w", err)
		}
	default:
		// Keep existing — read it back from DB.
		var existing string
		n.db.Get(&existing, `SELECT smtp_password FROM notification_settings WHERE id = 1`)
		encPassword = existing
	}

	_, err := n.db.Exec(`
		UPDATE notification_settings SET
			enabled = ?,
			smtp_host = ?, smtp_port = ?, smtp_username = ?, smtp_password = ?,
			smtp_from = ?, smtp_tls = ?, recipients = ?,
			on_server_offline = ?, on_health_critical = ?, on_job_failed = ?, on_firmware_updates = ?,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = 1`,
		in.Enabled, in.SMTPHost, in.SMTPPort, in.SMTPUsername, encPassword,
		in.SMTPFrom, in.SMTPTLS, in.Recipients,
		in.OnServerOffline, in.OnHealthCritical, in.OnJobFailed, in.OnFirmwareUpdates,
	)
	return err
}

// Send fires off a single email if the event isn't deduped and the relevant
// per-event toggle is on. serverID can be empty for non-server-scoped events
// ("firmware_updates" digest); dedup key falls back to "_global".
func (n *Notifier) Send(event, serverID, subject, body string) {
	settings, err := n.LoadSettings()
	if err != nil || !settings.Enabled || settings.SMTPHost == "" {
		return
	}
	if !n.eventEnabled(settings, event) {
		return
	}
	dedupKey := serverID
	if dedupKey == "" {
		dedupKey = "_global"
	}
	if n.recentlySent(dedupKey, event) {
		return
	}

	recipients, err := parseRecipients(settings.Recipients)
	if err != nil || len(recipients) == 0 {
		log.Printf("notifier: no recipients configured (recipients=%q)", settings.Recipients)
		return
	}

	if err := n.smtpSend(settings, recipients, subject, body); err != nil {
		log.Printf("notifier: SMTP send failed: %v", err)
		return
	}
	n.markSent(dedupKey, event)
	log.Printf("notifier: %s [%s] sent to %d recipient(s)", event, dedupKey, len(recipients))
}

// TestSend ignores the enable flag and dedup — used by the "Send Test Email"
// button in Settings. Returns the SMTP error to the caller for display.
func (n *Notifier) TestSend(settings *models.NotificationSettings, plainPassword string) error {
	// If user provided a fresh password, use it; else decrypt the stored one.
	pw := plainPassword
	if pw == "" && settings.SMTPPassword != "" {
		var err error
		pw, err = crypto.Decrypt(settings.SMTPPassword)
		if err != nil {
			return fmt.Errorf("decrypt stored password: %w", err)
		}
	}
	recipients, err := parseRecipients(settings.Recipients)
	if err != nil || len(recipients) == 0 {
		return fmt.Errorf("no recipients configured")
	}
	cfg := *settings
	cfg.SMTPPassword = pw // pass plaintext to smtpSendInner via local copy
	return n.smtpSendInner(&cfg, recipients,
		"[Dell iDRAC Manager] Test email",
		"This is a test email from the Dell iDRAC Manager.\n\nIf you can read this, SMTP is configured correctly.")
}

// ── helpers ─────────────────────────────────────────────────────────────────

func (n *Notifier) eventEnabled(s *models.NotificationSettings, event string) bool {
	switch event {
	case "server_offline":
		return s.OnServerOffline
	case "health_critical":
		return s.OnHealthCritical
	case "job_failed":
		return s.OnJobFailed
	case "firmware_updates":
		return s.OnFirmwareUpdates
	}
	return false
}

func (n *Notifier) recentlySent(serverID, event string) bool {
	var sentAt time.Time
	err := n.db.Get(&sentAt,
		`SELECT sent_at FROM notification_dedup WHERE server_id=? AND event=?`,
		serverID, event)
	if err != nil {
		return false
	}
	return time.Since(sentAt) < DedupCooldown
}

func (n *Notifier) markSent(serverID, event string) {
	n.db.Exec(`
		INSERT INTO notification_dedup (server_id, event, sent_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(server_id, event) DO UPDATE SET sent_at = CURRENT_TIMESTAMP`,
		serverID, event)
}

// ClearDedup forgets a previous send so the next event triggers an email
// immediately. Use when the user explicitly resets the alert state, or when
// the opposite event happens (e.g. server back online → forget "offline" mark
// so a future offline alert isn't suppressed).
func (n *Notifier) ClearDedup(serverID, event string) {
	n.db.Exec(`DELETE FROM notification_dedup WHERE server_id=? AND event=?`, serverID, event)
}

func parseRecipients(raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(list))
	for _, r := range list {
		r = strings.TrimSpace(r)
		if r != "" {
			out = append(out, r)
		}
	}
	return out, nil
}

func (n *Notifier) smtpSend(s *models.NotificationSettings, to []string, subject, body string) error {
	pw, err := crypto.Decrypt(s.SMTPPassword)
	if err != nil {
		return fmt.Errorf("decrypt smtp password: %w", err)
	}
	cfg := *s
	cfg.SMTPPassword = pw
	return n.smtpSendInner(&cfg, to, subject, body)
}

// smtpSendInner expects SMTPPassword to be PLAINTEXT (not the encrypted DB form).
func (n *Notifier) smtpSendInner(s *models.NotificationSettings, to []string, subject, body string) error {
	addr := fmt.Sprintf("%s:%d", s.SMTPHost, s.SMTPPort)
	from := s.SMTPFrom
	if from == "" {
		from = s.SMTPUsername
	}
	var auth smtp.Auth
	if s.SMTPUsername != "" {
		auth = smtp.PlainAuth("", s.SMTPUsername, s.SMTPPassword, s.SMTPHost)
	}

	msg := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s",
		from, strings.Join(to, ", "), subject, body))

	switch s.SMTPTLS {
	case "tls":
		return sendOverImplicitTLS(addr, s.SMTPHost, auth, from, to, msg)
	default: // "starttls" or "none"
		return smtp.SendMail(addr, auth, from, to, msg)
	}
}

// sendOverImplicitTLS handles the SMTPS case (port 465). Stdlib smtp.SendMail
// only knows STARTTLS, so we open the TLS connection ourselves.
func sendOverImplicitTLS(addr, host string, auth smtp.Auth, from string, to []string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host}) //nolint:gosec
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer c.Close()
	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return err
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, addr := range to {
		if err := c.Rcpt(addr); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
