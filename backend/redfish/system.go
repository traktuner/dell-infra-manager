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
		Count                 int    `json:"Count"`
		LogicalProcessorCount int    `json:"LogicalProcessorCount"`
		Model                 string `json:"Model"`
	} `json:"ProcessorSummary"`
	MemorySummary struct {
		TotalSystemMemoryGiB float64 `json:"TotalSystemMemoryGiB"`
	} `json:"MemorySummary"`
	// Enriched fields — fetched from /Memory and /Processors collections
	// (not part of the Redfish System object). Marshaled into system_json
	// so the frontend can show DDR4 2933 MHz / Xeon Gold etc. without an
	// extra round-trip per server.
	MemoryType      string `json:"MemoryType,omitempty"`
	MemorySpeedMHz  int    `json:"MemorySpeedMHz,omitempty"`
	CoresPerCPU     int    `json:"CoresPerCPU,omitempty"`
	ThreadsPerCPU   int    `json:"ThreadsPerCPU,omitempty"`
}

func (c *Client) GetSystem() (*SystemInfo, error) {
	var s SystemInfo
	if err := c.get("/Systems/System.Embedded.1", &s); err != nil {
		return nil, err
	}

	// Best-effort enrichment: find the first populated DIMM for memory type/speed.
	// Probe up to 8 slots sequentially — first slot is usually populated, so
	// this is typically a single extra round-trip.
	var memCol struct {
		Members []ODataRef `json:"Members"`
	}
	if c.get("/Systems/System.Embedded.1/Memory", &memCol) == nil {
		max := 8
		if len(memCol.Members) < max {
			max = len(memCol.Members)
		}
		for i := 0; i < max; i++ {
			var d struct {
				MemoryDeviceType  string `json:"MemoryDeviceType"`
				OperatingSpeedMhz int    `json:"OperatingSpeedMhz"`
				Status            struct {
					State string `json:"State"`
				} `json:"Status"`
			}
			if c.get(stripBaseURL(memCol.Members[i].ID), &d) != nil {
				continue
			}
			if d.Status.State == "Enabled" && d.MemoryDeviceType != "" {
				s.MemoryType = d.MemoryDeviceType
				s.MemorySpeedMHz = d.OperatingSpeedMhz
				break
			}
		}
	}

	// Per-CPU core/thread counts: derive from first processor.
	var procCol struct {
		Members []ODataRef `json:"Members"`
	}
	if c.get("/Systems/System.Embedded.1/Processors", &procCol) == nil && len(procCol.Members) > 0 {
		var p struct {
			TotalCores   int `json:"TotalCores"`
			TotalThreads int `json:"TotalThreads"`
		}
		if c.get(stripBaseURL(procCol.Members[0].ID), &p) == nil {
			s.CoresPerCPU = p.TotalCores
			s.ThreadsPerCPU = p.TotalThreads
		}
	}

	return &s, nil
}
