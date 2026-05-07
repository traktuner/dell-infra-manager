package models

import "time"

type LogEntry struct {
	ID        string    `json:"id"`
	Created   time.Time `json:"created"`
	Severity  string    `json:"severity"`
	Message   string    `json:"message"`
	MessageID string    `json:"message_id"`
	Category  string    `json:"category"`
}
