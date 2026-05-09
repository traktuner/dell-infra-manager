package redfish

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// VNCStatus reflects what iDRAC currently has configured for VNCServer.1.
type VNCStatus struct {
	Enabled bool `json:"enabled"`
	Port    int  `json:"port"`
}

const idracAttributesPath = "/Managers/iDRAC.Embedded.1/Attributes"

// GetVNCStatus reads the current VNC server config from iDRAC.
// Per Dell iDRAC9 Attribute Registry: VNCServer.1.Enable is a string
// ("Enabled"/"Disabled") and VNCServer.1.Port is a number.
func (c *Client) GetVNCStatus() (*VNCStatus, error) {
	var result struct {
		Attributes map[string]any `json:"Attributes"`
	}
	if err := c.get(idracAttributesPath, &result); err != nil {
		return nil, fmt.Errorf("read iDRAC attributes: %w", err)
	}
	return &VNCStatus{
		Enabled: result.Attributes["VNCServer.1.Enable"] == "Enabled",
		Port:    intAttr(result.Attributes["VNCServer.1.Port"], 5901),
	}, nil
}

// ConfigureVNC enables VNC on iDRAC with the given port and password,
// and explicitly disables SSL/TLS on the VNC channel.
//
// SSL must be off because our backend opens a plain TCP socket to iDRAC
// and proxies bytes verbatim to noVNC. With SSL enabled iDRAC sends a TLS
// ClientHello as the first bytes — noVNC then aborts with
// "unexpected data message" since those bytes aren't valid RFB.
//
// iDRAC applies the change immediately; no reboot needed.
func (c *Client) ConfigureVNC(port int, password string) error {
	body, _ := json.Marshal(map[string]any{
		"Attributes": map[string]any{
			"VNCServer.1.Enable":                 "Enabled",
			"VNCServer.1.Port":                   port,
			"VNCServer.1.Password":               password,
			"VNCServer.1.SSLEncryptionBitLength": "Disabled",
		},
	})
	resp, err := c.patch(idracAttributesPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("patch iDRAC VNC: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("iDRAC VNC configure (%d): %s", resp.StatusCode, b)
	}
	return nil
}

// intAttr extracts an int from a JSON-decoded any value, returning fallback on
// any type mismatch. iDRAC sometimes returns numeric attributes as strings,
// sometimes as numbers — this normalises both.
func intAttr(v any, fallback int) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		var i int
		if _, err := fmt.Sscanf(n, "%d", &i); err == nil {
			return i
		}
	}
	return fallback
}
