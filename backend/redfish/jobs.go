package redfish

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	var jobs []IDRACJob
	for _, ref := range col.Members {
		var j IDRACJob
		if err := c.get(stripBaseURL(ref.ID), &j); err == nil {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
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
