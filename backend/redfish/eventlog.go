package redfish

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"
)

type LogEntryCollection struct {
	Members    []LogEntry `json:"Members"`
	TotalCount int        `json:"Members@odata.count"`
}

type LogEntry struct {
	ID        string    `json:"Id"`
	Created   time.Time `json:"Created"`
	Severity  string    `json:"Severity"`
	Message   string    `json:"Message"`
	MessageID string    `json:"MessageId"`
	Category  string    `json:"OemRecordFormat"`
}

func (c *Client) GetEventLog(top, skip int) (*LogEntryCollection, error) {
	path := fmt.Sprintf("/Managers/iDRAC.Embedded.1/LogServices/Lclog/Entries?$top=%d&$skip=%d", top, skip)
	var col LogEntryCollection
	if err := c.get(path, &col); err != nil {
		return nil, err
	}
	return &col, nil
}

func (c *Client) ClearEventLog() error {
	resp, err := c.post(
		"/Managers/iDRAC.Embedded.1/LogServices/Lclog/Actions/LogService.ClearLog",
		bytes.NewReader([]byte("{}")), "application/json",
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("clear log failed (%d): %s", resp.StatusCode, string(b))
	}
	return nil
}

func logCheckResponse(resp *http.Response) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}
