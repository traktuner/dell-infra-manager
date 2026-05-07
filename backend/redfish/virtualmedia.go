package redfish

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

type VirtualMediaCollection struct {
	Members []ODataRef `json:"Members"`
}

type VirtualMedia struct {
	ID           string `json:"Id"`
	MediaTypes   []string `json:"MediaTypes"`
	Inserted     bool   `json:"Inserted"`
	Image        string `json:"Image"`
	ConnectedVia string `json:"ConnectedVia"`
	WriteProtected bool `json:"WriteProtected"`
	Status       Status `json:"Status"`
}

func (c *Client) GetVirtualMedia() ([]VirtualMedia, error) {
	var col VirtualMediaCollection
	if err := c.get("/Managers/iDRAC.Embedded.1/VirtualMedia", &col); err != nil {
		return nil, err
	}
	var result []VirtualMedia
	for _, ref := range col.Members {
		var vm VirtualMedia
		if err := c.get(stripBaseURL(ref.ID), &vm); err == nil {
			result = append(result, vm)
		}
	}
	return result, nil
}

func (c *Client) InsertVirtualMedia(slot, imageURL string) error {
	if slot == "" {
		slot = "CD"
	}
	body, _ := json.Marshal(map[string]string{"Image": imageURL})
	path := fmt.Sprintf("/Managers/iDRAC.Embedded.1/VirtualMedia/%s/Actions/VirtualMedia.InsertMedia", slot)
	resp, err := c.post(path, bytes.NewReader(body), "application/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("insert media failed (%d): %s", resp.StatusCode, string(b))
	}
	return nil
}

func (c *Client) EjectVirtualMedia(slot string) error {
	if slot == "" {
		slot = "CD"
	}
	path := fmt.Sprintf("/Managers/iDRAC.Embedded.1/VirtualMedia/%s/Actions/VirtualMedia.EjectMedia", slot)
	resp, err := c.post(path, bytes.NewReader([]byte("{}")), "application/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("eject media failed (%d): %s", resp.StatusCode, string(b))
	}
	return nil
}
