package redfish

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
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

// ConfigureVNC enables VNC on iDRAC with the given port and password.
//
// Done in two PATCHes so the basic enable always succeeds even on iDRAC
// firmware versions that don't accept our SSL setting:
//  1. Enable + Port + Password — REQUIRED, fails the call if rejected.
//  2. SSLEncryptionBitLength=Disabled — best-effort, logged but non-fatal.
//     SSL must be off because our backend speaks plain TCP to iDRAC and
//     pipes bytes verbatim to noVNC. With SSL on, iDRAC sends a TLS
//     ClientHello which noVNC rejects as "unexpected data message".
//
// iDRAC applies both changes immediately; no reboot needed.
func (c *Client) ConfigureVNC(port int, password string) error {
	if err := c.patchAttributes(map[string]any{
		"VNCServer.1.Enable":   "Enabled",
		"VNCServer.1.Port":     port,
		"VNCServer.1.Password": password,
	}); err != nil {
		return err
	}

	// Best-effort: turn off SSL so plain RFB works through our proxy.
	// Different firmware revisions accept different value formats for
	// VNCServer.1.SSLEncryptionBitLength — try the modern string enum first,
	// then the legacy integer. If neither lands, log and continue; noVNC will
	// surface "unexpected data message" and the user can disable SSL manually.
	for _, val := range []any{"Disabled", 0} {
		if err := c.patchAttributes(map[string]any{
			"VNCServer.1.SSLEncryptionBitLength": val,
		}); err == nil {
			return nil
		}
	}
	log.Printf("redfish: VNC SSL disable rejected by iDRAC (non-fatal); KVM may show 'unexpected data message'")
	return nil
}

func (c *Client) patchAttributes(attrs map[string]any) error {
	body, _ := json.Marshal(map[string]any{"Attributes": attrs})
	resp, err := c.patch(idracAttributesPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("patch iDRAC attrs: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("iDRAC PATCH (%d): %s", resp.StatusCode, b)
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
