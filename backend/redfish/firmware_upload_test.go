package redfish

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestUploadFirmwareStreamsCompleteMultipartWithOnReset(t *testing.T) {
	payload := []byte("firmware-package-bytes")
	client := &Client{
		baseURL: "https://idrac.example/redfish/v1",
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/redfish/v1/UpdateService/MultipartUpload" {
				t.Fatalf("unexpected path %q", req.URL.Path)
			}
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatal(err)
			}
			if req.ContentLength != int64(len(body)) {
				t.Fatalf("Content-Length=%d, body=%d", req.ContentLength, len(body))
			}
			clone := req.Clone(req.Context())
			clone.Body = io.NopCloser(bytes.NewReader(body))
			reader, err := clone.MultipartReader()
			if err != nil {
				t.Fatal(err)
			}
			paramsPart, err := reader.NextPart()
			if err != nil {
				t.Fatal(err)
			}
			var params UpdateParams
			if err := json.NewDecoder(paramsPart).Decode(&params); err != nil {
				t.Fatal(err)
			}
			if params.RedfishOperationApplyTime != "OnReset" {
				t.Fatalf("unexpected apply time %q", params.RedfishOperationApplyTime)
			}
			filePart, err := reader.NextPart()
			if err != nil {
				t.Fatal(err)
			}
			got, err := io.ReadAll(filePart)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, payload) {
				t.Fatalf("firmware payload=%q", got)
			}
			if _, err := reader.NextPart(); err != io.EOF {
				t.Fatalf("multipart did not end cleanly: %v", err)
			}
			return &http.Response{
				StatusCode: http.StatusAccepted,
				Header:     http.Header{"Location": []string{"/redfish/v1/TaskService/Tasks/JID_1"}},
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		})},
	}

	location, err := client.UploadFirmware("update.exe", bytes.NewReader(payload), int64(len(payload)), "OnReset")
	if err != nil {
		t.Fatal(err)
	}
	if location != "/redfish/v1/TaskService/Tasks/JID_1" {
		t.Fatalf("unexpected location %q", location)
	}
}

func TestUploadFirmwareRejectsImmediateApply(t *testing.T) {
	client := &Client{httpClient: &http.Client{}}
	if _, err := client.UploadFirmware("update.exe", strings.NewReader("x"), 1, "Immediate"); err == nil {
		t.Fatal("expected Immediate apply to be rejected")
	}
}
