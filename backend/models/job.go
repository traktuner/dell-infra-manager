package models

import "time"

type JobType string
type JobStatus string

const (
	JobTypeFirmwareUpdate JobType = "firmware_update"
	JobTypeBiosConfig     JobType = "bios_config"

	JobStatusQueued  JobStatus = "queued"
	JobStatusRunning JobStatus = "running"
	JobStatusDone    JobStatus = "done"
	JobStatusFailed  JobStatus = "failed"
)

type Job struct {
	ID          string     `db:"id"            json:"id"`
	ServerID    string     `db:"server_id"     json:"server_id"`
	Type        JobType    `db:"type"          json:"type"`
	Status      JobStatus  `db:"status"        json:"status"`
	Payload     string     `db:"payload"       json:"payload"`  // JSON
	Result      *string    `db:"result"        json:"result"`   // JSON or error message
	IDRACJobID  *string    `db:"idrac_job_id"  json:"idrac_job_id"`
	CreatedAt   time.Time  `db:"created_at"    json:"created_at"`
	StartedAt   *time.Time `db:"started_at"    json:"started_at"`
	FinishedAt  *time.Time `db:"finished_at"   json:"finished_at"`
}

type FirmwareUpdatePayload struct {
	Component   string `json:"component"`
	CatalogPath string `json:"catalog_path"`
	Version     string `json:"version"`
}

type BiosConfigPayload struct {
	Attributes map[string]interface{} `json:"attributes"`
	ApplyTime  string                 `json:"apply_time"`
}
