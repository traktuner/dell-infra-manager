package redfish

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

func NewClient(hostname string, port int, username, password string, tlsVerify bool) *Client {
	transport := &http.Transport{
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: !tlsVerify}, //nolint:gosec
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	return &Client{
		baseURL:  fmt.Sprintf("https://%s:%d/redfish/v1", hostname, port),
		username: username,
		password: password,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
		},
	}
}

func (c *Client) get(path string, out interface{}) error {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("unauthorized: invalid credentials")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("redfish %s returned %d: %s", path, resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) post(path string, body io.Reader, contentType string) (*http.Response, error) {
	return c.postWithLength(path, body, contentType, -1)
}

func (c *Client) postWithLength(path string, body io.Reader, contentType string, contentLength int64) (*http.Response, error) {
	return c.postWithLengthAndTimeout(path, body, contentType, contentLength, c.httpClient.Timeout)
}

func (c *Client) postWithLengthAndTimeout(path string, body io.Reader, contentType string, contentLength int64, timeout time.Duration) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if contentLength >= 0 {
		req.ContentLength = contentLength
	}
	client := *c.httpClient
	client.Timeout = timeout
	return client.Do(req)
}

func (c *Client) patch(path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPatch, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	return c.httpClient.Do(req)
}

func (c *Client) delete(path string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "application/json")
	return c.httpClient.Do(req)
}

// Ping checks if iDRAC is reachable and credentials are valid.
func (c *Client) Ping() error {
	var result map[string]interface{}
	return c.get("/Systems/System.Embedded.1", &result)
}

// newSSERequest creates a request for the SSE stream (no timeout — long-lived).
func (c *Client) newSSERequest(path string) (*http.Request, *http.Client, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, nil, err
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	transport := c.httpClient.Transport.(*http.Transport).Clone()
	sseClient := &http.Client{
		Transport: transport,
		Timeout:   0, // no timeout for SSE streams
	}
	return req, sseClient, nil
}
