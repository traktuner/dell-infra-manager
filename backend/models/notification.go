package models

import "time"

// NotificationSettings is a singleton row (id always = 1) holding the SMTP
// config and which events trigger emails.
type NotificationSettings struct {
	ID                int       `db:"id"                  json:"-"`
	Enabled           bool      `db:"enabled"             json:"enabled"`
	SMTPHost          string    `db:"smtp_host"           json:"smtp_host"`
	SMTPPort          int       `db:"smtp_port"           json:"smtp_port"`
	SMTPUsername      string    `db:"smtp_username"       json:"smtp_username"`
	SMTPPassword      string    `db:"smtp_password"       json:"-"` // never serialised — write-only
	SMTPFrom          string    `db:"smtp_from"           json:"smtp_from"`
	SMTPTLS           string    `db:"smtp_tls"            json:"smtp_tls"`     // none | starttls | tls
	Recipients        string    `db:"recipients"          json:"recipients"`   // JSON array, kept as string in DB
	OnServerOffline   bool      `db:"on_server_offline"   json:"on_server_offline"`
	OnHealthCritical  bool      `db:"on_health_critical"  json:"on_health_critical"`
	OnJobFailed       bool      `db:"on_job_failed"       json:"on_job_failed"`
	OnFirmwareUpdates bool      `db:"on_firmware_updates" json:"on_firmware_updates"`
	UpdatedAt         time.Time `db:"updated_at"          json:"updated_at"`
	// HasPassword is a synthesised flag — true when smtp_password is non-empty.
	// We never expose the password itself; the UI shows "•••••" if true.
	HasPassword bool `db:"-" json:"has_password"`
}
