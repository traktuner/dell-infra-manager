package redfish

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

type BiosResponse struct {
	Attributes map[string]interface{} `json:"Attributes"`
}

type BiosRegistry struct {
	RegistryEntries []BiosRegistryAttribute `json:"RegistryEntries"`
}

type BiosRegistryAttribute struct {
	AttributeName string        `json:"AttributeName"`
	DisplayName   string        `json:"DisplayName"`
	Type          string        `json:"Type"`
	ReadOnly      bool          `json:"ReadOnly"`
	Value         []EnumValue   `json:"Value,omitempty"` // For Enumeration type
	LowerBound    *int          `json:"LowerBound,omitempty"`
	UpperBound    *int          `json:"UpperBound,omitempty"`
}

type EnumValue struct {
	ValueName string `json:"ValueName"`
}

func (c *Client) GetBios() (*BiosResponse, error) {
	var bios BiosResponse
	if err := c.get("/Systems/System.Embedded.1/Bios", &bios); err != nil {
		return nil, err
	}
	return &bios, nil
}

func (c *Client) GetBiosRegistry() (*BiosRegistry, error) {
	var reg BiosRegistry
	if err := c.get("/Systems/System.Embedded.1/Bios/BiosRegistry", &reg); err != nil {
		return nil, err
	}
	return &reg, nil
}

type BiosPatchRequest struct {
	Attributes               map[string]interface{} `json:"Attributes"`
	RedfishSettingsApplyTime *ApplyTime             `json:"@Redfish.SettingsApplyTime,omitempty"`
}

type ApplyTime struct {
	ApplyTime string `json:"ApplyTime"` // OnReset | AtMaintenanceWindowStart
}

// SetBiosAttributes sends a PATCH to /Bios/Settings and returns the iDRAC job ID.
func (c *Client) SetBiosAttributes(attributes map[string]interface{}, applyTime string) (string, error) {
	if applyTime == "" {
		applyTime = "OnReset"
	}
	payload := BiosPatchRequest{
		Attributes: attributes,
		RedfishSettingsApplyTime: &ApplyTime{ApplyTime: applyTime},
	}
	body, _ := json.Marshal(payload)
	resp, err := c.patch("/Systems/System.Embedded.1/Bios/Settings", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("bios patch failed (%d): %s", resp.StatusCode, string(b))
	}
	return resp.Header.Get("Location"), nil
}
