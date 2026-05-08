package redfish

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"sync"
)

type FirmwareCollection struct {
	Members []ODataRef `json:"Members"`
}

type FirmwareComponent struct {
	ID         string `json:"Id"`
	Name       string `json:"Name"`
	Version    string `json:"Version"`
	Updateable bool   `json:"Updateable"`
	Status     Status `json:"Status"`
}

func (c *Client) GetFirmwareInventory() ([]FirmwareComponent, error) {
	var col FirmwareCollection
	if err := c.get("/UpdateService/FirmwareInventory", &col); err != nil {
		return nil, err
	}

	// Fetch all components in parallel — serial fetches with 50+ components
	// can block the initial poll for several minutes.
	const maxConcurrent = 10
	sem := make(chan struct{}, maxConcurrent)
	components := make([]FirmwareComponent, len(col.Members))
	var wg sync.WaitGroup
	for i, ref := range col.Members {
		wg.Add(1)
		go func(i int, path string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			var comp FirmwareComponent
			if err := c.get(path, &comp); err == nil {
				components[i] = comp
			}
		}(i, stripBaseURL(ref.ID))
	}
	wg.Wait()

	var result []FirmwareComponent
	for _, comp := range components {
		if comp.ID != "" {
			result = append(result, comp)
		}
	}
	return result, nil
}

type UpdateParams struct {
	Targets                  []string `json:"Targets"`
	RedfishOperationApplyTime string  `json:"@Redfish.OperationApplyTime"`
}

// UploadFirmware uploads a DUP file and returns the iDRAC job ID from the Location header.
func (c *Client) UploadFirmware(filename string, fileData []byte, applyTime string) (string, error) {
	params := UpdateParams{
		Targets:                  []string{},
		RedfishOperationApplyTime: applyTime,
	}
	paramsJSON, _ := json.Marshal(params)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// UpdateParameters part
	ph := make(textproto.MIMEHeader)
	ph.Set("Content-Disposition", `form-data; name="UpdateParameters"`)
	ph.Set("Content-Type", "application/json")
	part, err := writer.CreatePart(ph)
	if err != nil {
		return "", err
	}
	part.Write(paramsJSON)

	// UpdateFile part
	fh := make(textproto.MIMEHeader)
	fh.Set("Content-Disposition", fmt.Sprintf(`form-data; name="UpdateFile"; filename="%s"`, filename))
	fh.Set("Content-Type", "application/octet-stream")
	filePart, err := writer.CreatePart(fh)
	if err != nil {
		return "", err
	}
	filePart.Write(fileData)
	writer.Close()

	resp, err := c.post("/UpdateService/MultipartUpload", &body, writer.FormDataContentType())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload failed (%d): %s", resp.StatusCode, string(b))
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("no Location header in firmware upload response")
	}
	// Extract job ID from e.g. /redfish/v1/TaskService/Tasks/JID_xxx
	return location, nil
}
