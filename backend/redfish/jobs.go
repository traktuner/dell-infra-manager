package redfish

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

type IDRACJob struct {
	ID              string `json:"Id"`
	Name            string `json:"Name"`
	JobState        string `json:"JobState"`
	PercentComplete int    `json:"PercentComplete"`
	Message         string `json:"Message"`
	StartTime       string `json:"StartTime"`
	EndTime         string `json:"EndTime"`
}

// jobsCollection holds the inline-expanded Members from $expand=*.
type jobsCollection struct {
	Members []IDRACJob `json:"Members"`
}

// GetJobs fetches all iDRAC jobs in a single Redfish call using $expand=*,
// which inlines each Job's full payload into the collection response.
// This avoids the N+1 fan-out we'd otherwise need to dereference Members refs.
func (c *Client) GetJobs() ([]IDRACJob, error) {
	var col jobsCollection
	if err := c.get("/Managers/iDRAC.Embedded.1/Jobs?$expand=*($levels=1)", &col); err != nil {
		return nil, err
	}
	// Most-recently-started first. iDRAC sometimes uses "TIME_NA" as a placeholder
	// for not-yet-started jobs — those sort last naturally with string compare.
	sort.Slice(col.Members, func(i, j int) bool {
		return col.Members[i].StartTime > col.Members[j].StartTime
	})
	if col.Members == nil {
		col.Members = []IDRACJob{}
	}
	return col.Members, nil
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

// IsJobDone returns true for terminal states.
func IsJobDone(state string) bool {
	return state == "Completed" || state == "Failed"
}
