package redfish

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type PowerInfo struct {
	PowerControl  []PowerControl  `json:"PowerControl"`
	PowerSupplies []PowerSupply   `json:"PowerSupplies"`
}

type PowerControl struct {
	Name                string  `json:"Name"`
	PowerConsumedWatts  float64 `json:"PowerConsumedWatts"`
	PowerCapacityWatts  float64 `json:"PowerCapacityWatts"`
}

type PowerSupply struct {
	Name                  string  `json:"Name"`
	LastPowerOutputWatts  float64 `json:"LastPowerOutputWatts"`
	LineInputVoltage      float64 `json:"LineInputVoltage"`
	Status                struct {
		Health string `json:"Health"`
		State  string `json:"State"`
	} `json:"Status"`
}

func (c *Client) GetPower() (*PowerInfo, error) {
	var p PowerInfo
	if err := c.get("/Chassis/System.Embedded.1/Power", &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ResetType values supported by iDRAC9 Redfish.
type ResetType string

const (
	ResetOn               ResetType = "On"
	ResetForceOff         ResetType = "ForceOff"
	ResetGracefulShutdown ResetType = "GracefulShutdown"
	ResetGracefulRestart  ResetType = "GracefulRestart"
	ResetForceRestart     ResetType = "ForceRestart"
	ResetPushPowerButton  ResetType = "PushPowerButton"
	ResetNMI              ResetType = "Nmi"
)

func (c *Client) ResetSystem(resetType ResetType) error {
	body, _ := json.Marshal(map[string]string{"ResetType": string(resetType)})
	resp, err := c.post("/Systems/System.Embedded.1/Actions/ComputerSystem.Reset",
		bytes.NewReader(body), "application/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("reset failed (%d): %s", resp.StatusCode, string(b))
	}
	return nil
}

func ValidResetType(s string) (ResetType, error) {
	valid := map[ResetType]bool{
		ResetOn: true, ResetForceOff: true, ResetGracefulShutdown: true,
		ResetGracefulRestart: true, ResetForceRestart: true,
		ResetPushPowerButton: true, ResetNMI: true,
	}
	rt := ResetType(s)
	if !valid[rt] {
		return "", fmt.Errorf("invalid reset type: %s", s)
	}
	return rt, nil
}

// GetPowerState returns the current PowerState string ("On", "Off", etc.)
func (c *Client) GetPowerState() (string, error) {
	sys, err := c.GetSystem()
	if err != nil {
		return "", err
	}
	return sys.PowerState, nil
}

func checkResponse(resp *http.Response) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}
