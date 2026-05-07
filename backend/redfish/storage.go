package redfish

type StorageCollection struct {
	Members []ODataRef `json:"Members"`
}

type ODataRef struct {
	ID string `json:"@odata.id"`
}

type StorageController struct {
	ID     string     `json:"Id"`
	Name   string     `json:"Name"`
	Status Status     `json:"Status"`
	Drives []ODataRef `json:"Drives"`
	Volumes []ODataRef `json:"Volumes"`
}

type Drive struct {
	ID                string  `json:"Id"`
	Name              string  `json:"Name"`
	MediaType         string  `json:"MediaType"`
	CapacityBytes     int64   `json:"CapacityBytes"`
	Protocol          string  `json:"Protocol"`
	RotationSpeedRPM  float64 `json:"RotationSpeedRPM"`
	FailurePredicted  bool    `json:"FailurePredicted"`
	Status            Status  `json:"Status"`
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

func (c *Client) GetStorage() ([]StorageDetail, error) {
	var col StorageCollection
	if err := c.get("/Systems/System.Embedded.1/Storage", &col); err != nil {
		return nil, err
	}

	var result []StorageDetail
	for _, ref := range col.Members {
		path := stripBaseURL(ref.ID)
		var ctrl StorageController
		if err := c.get(path, &ctrl); err != nil {
			continue
		}
		detail := StorageDetail{Controller: ctrl}

		for _, dRef := range ctrl.Drives {
			var drive Drive
			if err := c.get(stripBaseURL(dRef.ID), &drive); err == nil {
				detail.Drives = append(detail.Drives, drive)
			}
		}
		for _, vRef := range ctrl.Volumes {
			var vol Volume
			if err := c.get(stripBaseURL(vRef.ID), &vol); err == nil {
				detail.Volumes = append(detail.Volumes, vol)
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
