package redfish

import "sync"

type StorageCollection struct {
	Members []ODataRef `json:"Members"`
}

type ODataRef struct {
	ID string `json:"@odata.id"`
}

// StorageController matches the Redfish Storage v1_x schema.
//
// IMPORTANT: per spec, "Drives" is an array of references but "Volumes" is a
// SINGLE reference to a Volume collection — NOT an array of volume refs.
// We previously had `Volumes []ODataRef` which caused unmarshal to fail with
// "cannot unmarshal object into Go struct field of type []redfish.ODataRef",
// so every controller fetch errored and the storage list came up empty.
type StorageController struct {
	ID                 string                  `json:"Id"`
	Name               string                  `json:"Name"`
	Status             Status                  `json:"Status"`
	Drives             []ODataRef              `json:"Drives"`
	Volumes            ODataRef                `json:"Volumes"`
	StorageControllers []StorageControllerCard `json:"StorageControllers"`
}

// StorageControllerCard is the per-controller detail (model, FW version)
// embedded inside the Storage object's `StorageControllers` array.
type StorageControllerCard struct {
	Name            string `json:"Name"`
	Manufacturer    string `json:"Manufacturer"`
	Model           string `json:"Model"`
	FirmwareVersion string `json:"FirmwareVersion"`
	Status          Status `json:"Status"`
}

type Drive struct {
	ID               string  `json:"Id"`
	Name             string  `json:"Name"`
	MediaType        string  `json:"MediaType"` // "HDD" | "SSD"
	CapacityBytes    int64   `json:"CapacityBytes"`
	Protocol         string  `json:"Protocol"` // "SAS" | "SATA" | "NVMe"
	RotationSpeedRPM float64 `json:"RotationSpeedRPM"`
	FailurePredicted bool    `json:"FailurePredicted"`
	Manufacturer     string  `json:"Manufacturer"`
	Model            string  `json:"Model"`
	SerialNumber     string  `json:"SerialNumber"`
	Status           Status  `json:"Status"`
}

type Volume struct {
	ID            string `json:"Id"`
	Name          string `json:"Name"`
	RAIDType      string `json:"RAIDType"`
	CapacityBytes int64  `json:"CapacityBytes"`
	VolumeType    string `json:"VolumeType"`
	Status        Status `json:"Status"`
}

type Status struct {
	Health string `json:"Health"`
	State  string `json:"State"`
}

type StorageDetail struct {
	Controller StorageController
	Drives     []Drive
	Volumes    []Volume
}

// GetStorage walks every storage controller, then fetches its drives and
// volumes in parallel (max 10 concurrent — a single PERC controller can
// expose 24+ drives and serial fetches blow past the per-request timeout).
func (c *Client) GetStorage() ([]StorageDetail, error) {
	var col StorageCollection
	if err := c.get("/Systems/System.Embedded.1/Storage", &col); err != nil {
		return nil, err
	}

	const maxConcurrent = 10
	result := make([]StorageDetail, 0, len(col.Members))
	for _, ref := range col.Members {
		ctrlPath := stripBaseURL(ref.ID)
		var ctrl StorageController
		if err := c.get(ctrlPath, &ctrl); err != nil {
			continue
		}
		detail := StorageDetail{Controller: ctrl}

		// Drives — direct refs in ctrl.Drives, fetch in parallel.
		if len(ctrl.Drives) > 0 {
			drives := make([]Drive, len(ctrl.Drives))
			sem := make(chan struct{}, maxConcurrent)
			var wg sync.WaitGroup
			for i, dRef := range ctrl.Drives {
				wg.Add(1)
				go func(i int, path string) {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()
					var d Drive
					if err := c.get(path, &d); err == nil {
						drives[i] = d
					}
				}(i, stripBaseURL(dRef.ID))
			}
			wg.Wait()
			for _, d := range drives {
				if d.ID != "" {
					detail.Drives = append(detail.Drives, d)
				}
			}
		}

		// Volumes — ctrl.Volumes is a single ref to a Collection, walk it.
		if ctrl.Volumes.ID != "" {
			var volCol StorageCollection
			if c.get(stripBaseURL(ctrl.Volumes.ID), &volCol) == nil && len(volCol.Members) > 0 {
				volumes := make([]Volume, len(volCol.Members))
				sem := make(chan struct{}, maxConcurrent)
				var wg sync.WaitGroup
				for i, vRef := range volCol.Members {
					wg.Add(1)
					go func(i int, path string) {
						defer wg.Done()
						sem <- struct{}{}
						defer func() { <-sem }()
						var v Volume
						if err := c.get(path, &v); err == nil {
							volumes[i] = v
						}
					}(i, stripBaseURL(vRef.ID))
				}
				wg.Wait()
				for _, v := range volumes {
					if v.ID != "" {
						detail.Volumes = append(detail.Volumes, v)
					}
				}
			}
		}

		result = append(result, detail)
	}
	return result, nil
}

// stripBaseURL strips the /redfish/v1 prefix from @odata.id references
// so they can be used directly with c.get().
func stripBaseURL(odataID string) string {
	const prefix = "/redfish/v1"
	if len(odataID) > len(prefix) && odataID[:len(prefix)] == prefix {
		return odataID[len(prefix):]
	}
	return odataID
}
