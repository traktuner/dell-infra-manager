package models

import "time"

type Server struct {
	ID        string    `db:"id"         json:"id"`
	Name      string    `db:"name"       json:"name"`
	Hostname  string    `db:"hostname"   json:"hostname"`
	Port      int       `db:"port"       json:"port"`
	Username  string    `db:"username"   json:"username"`
	Password  string    `db:"password"   json:"-"`
	TLSVerify bool      `db:"tls_verify" json:"tls_verify"`
	Tags      string    `db:"tags"       json:"tags"` // JSON array string
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

type ServerCache struct {
	ServerID           string     `db:"server_id"`
	SystemJSON         *string    `db:"system_json"`
	ThermalJSON        *string    `db:"thermal_json"`
	PowerJSON          *string    `db:"power_json"`
	FirmwareJSON       *string    `db:"firmware_json"`
	StorageJSON        *string    `db:"storage_json"`
	BiosJSON           *string    `db:"bios_json"`
	BiosRegistryJSON   *string    `db:"bios_registry_json"`
	VirtualMediaJSON   *string    `db:"virtualmedia_json"`
	LastSeen           *time.Time `db:"last_seen"`
	Status             string     `db:"status"`
}

// AddServerRequest is the incoming JSON for creating a server.
type AddServerRequest struct {
	Name      string `json:"name"       binding:"required"`
	Hostname  string `json:"hostname"   binding:"required"`
	Port      int    `json:"port"`
	Username  string `json:"username"   binding:"required"`
	Password  string `json:"password"   binding:"required"`
	TLSVerify bool   `json:"tls_verify"`
	Tags      string `json:"tags"`
}

// UpdateServerRequest allows partial updates.
type UpdateServerRequest struct {
	Name      *string `json:"name"`
	Hostname  *string `json:"hostname"`
	Port      *int    `json:"port"`
	Username  *string `json:"username"`
	Password  *string `json:"password"`
	TLSVerify *bool   `json:"tls_verify"`
	Tags      *string `json:"tags"`
}
