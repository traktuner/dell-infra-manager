package redfish

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
)

type IDRACJob struct {
	ID              string  `json:"Id"`
	Name            string  `json:"Name"`
	JobState        string  `json:"JobState"`
	PercentComplete int     `json:"PercentComplete"`
	Message         string  `json:"Message"`
	StartTime       string  `json:"StartTime"`
	EndTime         string  `json:"EndTime"`
}

type IDRACJobCollection struct {
	Members []ODataRef `json:"Members"`
}

func (c *Client) GetJobs() ([]IDRACJob, error) {
	var col IDRACJobCollection
	if err := c.get("/Managers/iDRAC.Embedded.1/Jobs", &col); err != nil {
		return nil, err
	}
	// Fetch each job in parallel — iDRAC fleets can have dozens of historical jobs,
	// and serial fetches blow past the 30s HTTP timeout.
	jobs := make([]IDRACJob, len(col.Members))
	var wg sync.WaitGroup
	for i, ref := range col.Members {
		wg.Add(1)
		go func(i int, path string) {
			defer wg.Done()
			var j IDRACJob
			if err := c.get(path, &j); err == nil {
				jobs[i] = j
			}
		}(i, stripBaseURL(ref.ID))
	}
	wg.Wait()
	// Strip out failed fetches (zero-value structs).
	out := jobs[:0]
	for _, j := range jobs {
		if j.ID != "" {
			out = append(out, j)
		}
	}
	// Most-recently-started first.
	sort.Slice(out, func(i, j int) bool { return out[i].StartTime > out[j].StartTime })
	return out, nil
}

func (c *Client) GetJob(jid string) (*IDRACJob, error) {
	var j IDRACJob
	if err := c.get("/Managers/iDRAC.Embedded.1/Jobs/"+jid, &j); err != nil {
		return nil, err
	}
	return &j, nil
}

func (c *Client) DeleteJob(jid string) error {
	resp, err := c.delete("/Managers/iDRAC.Embedded.1/Jobs/" + jid)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete job failed (%d): %s", resp.StatusCode, string(b))
	}
	return nil
}

func (c *Client) ClearJobQueue(jobIDs []string) error {
	if len(jobIDs) == 0 {
		jobIDs = []string{"JID_CLEARALL"}
	}
	body, _ := json.Marshal(map[string][]string{"JobIDs": jobIDs})
	resp, err := c.post(
		"/Managers/iDRAC.Embedded.1/Actions/Oem/DellManager.ClearJobQueue",
		bytes.NewReader(body), "application/json",
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("clear job queue failed (%d): %s", resp.StatusCode, string(b))
	}
	return nil
}

// WaitForJobState checks job state — caller polls this at interval.
func IsJobDone(state string) bool {
	return state == "Completed" || state == "Failed"
}

func jobCheckResponse(resp *http.Response) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}
