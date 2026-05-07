package redfish

type SystemInfo struct {
	ID           string `json:"Id"`
	Name         string `json:"Name"`
	Model        string `json:"Model"`
	SerialNumber string `json:"SerialNumber"` // ServiceTag
	HostName     string `json:"HostName"`
	PowerState   string `json:"PowerState"`
	BiosVersion  string `json:"BiosVersion"`
	Status       struct {
		Health string `json:"Health"`
		State  string `json:"State"`
	} `json:"Status"`
	ProcessorSummary struct {
		Count int `json:"Count"`
	} `json:"ProcessorSummary"`
	MemorySummary struct {
		TotalSystemMemoryGiB float64 `json:"TotalSystemMemoryGiB"`
	} `json:"MemorySummary"`
}

func (c *Client) GetSystem() (*SystemInfo, error) {
	var s SystemInfo
	if err := c.get("/Systems/System.Embedded.1", &s); err != nil {
		return nil, err
	}
	return &s, nil
}
