package redfish

import (
	"bufio"
	"context"
	"log"
	"strings"
	"time"
)

type SSEEvent struct {
	ServerID string
	Data     string
	EventType string
}

// SSEListener connects to iDRAC SSE stream and sends events to the out channel.
// It reconnects automatically with exponential backoff.
// When maxRetries consecutive failures occur, it gives up and closes out.
func (c *Client) SSEListener(ctx context.Context, serverID string, out chan<- SSEEvent, maxRetries int) {
	backoff := []time.Duration{5 * time.Second, 10 * time.Second, 30 * time.Second, 60 * time.Second}
	failures := 0

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := c.streamSSE(ctx, serverID, out)
		if err == nil || ctx.Err() != nil {
			return
		}

		failures++
		log.Printf("SSE[%s] disconnected (%d/%d): %v", serverID, failures, maxRetries, err)
		if failures >= maxRetries {
			log.Printf("SSE[%s] max retries reached, giving up", serverID)
			return
		}

		delay := backoff[min(failures-1, len(backoff)-1)]
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

func (c *Client) streamSSE(ctx context.Context, serverID string, out chan<- SSEEvent) error {
	req, sseClient, err := c.newSSERequest("/SSE?$filter=EventFormatType eq MetricReport")
	if err != nil {
		return err
	}
	req = req.WithContext(ctx)

	resp, err := sseClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	var eventType, data string

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event:"):
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		case line == "":
			if data != "" {
				select {
				case out <- SSEEvent{ServerID: serverID, EventType: eventType, Data: data}:
				case <-ctx.Done():
					return nil
				}
			}
			eventType, data = "", ""
		}
	}
	return scanner.Err()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
