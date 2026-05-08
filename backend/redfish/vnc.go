package redfish

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// VNCStatus holds the current VNC configuration read from iDRAC.
type VNCStatus struct {
	Enabled  bool   `json:"enabled"`
	Port     int    `json:"port"`
	Password string `json:"-"` // never serialised to JSON
}

// iDRAC9 Redfish attribute path for iDRAC manager attributes.
const idracAttributesPath = "/Managers/iDRAC.Embedded.1/Attributes"

// GetVNCStatus reads the current VNC server config from iDRAC Attributes.
func (c *Client) GetVNCStatus() (*VNCStatus, error) {
	var result struct {
		Attributes map[string]interface{} `json:"Attributes"`
	}
	if err := c.get(idracAttributesPath, &result); err != nil {
		return nil, fmt.Errorf("get iDRAC attributes: %w", err)
	}
	attrs := result.Attributes

	enabled := false
	if v, ok := attrs["VNCServer.1.Enable"]; ok {
		switch s := v.(type) {
		case string:
			enabled = s == "Enabled"
		case bool:
			enabled = s
		}
	}
	port := 5901
	if v, ok := attrs["VNCServer.1.Port"]; ok {
		switch n := v.(type) {
		case float64:
			port = int(n)
		case int:
			port = n
		}
	}
	return &VNCStatus{Enabled: enabled, Port: port}, nil
}

// EnableVNC enables the iDRAC VNC server on the given port with the given
// password. iDRAC applies the setting immediately (no reboot required).
func (c *Client) EnableVNC(port int, password string) error {
	payload := map[string]interface{}{
		"Attributes": map[string]interface{}{
			"VNCServer.1.Enable":                "Enabled",
			"VNCServer.1.Port":                  port,
			"VNCServer.1.Password":              password,
			"VNCServer.1.SSLEncryptionBitLength": 0, // no TLS on VNC — our Go proxy handles transport
		},
	}
	body, _ := json.Marshal(payload)
	resp, err := c.patch(idracAttributesPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("patch iDRAC VNC attributes: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("iDRAC VNC enable returned %d: %s", resp.StatusCode, string(b))
	}
	return nil
}
