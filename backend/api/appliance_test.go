package api

import "testing"

func TestChecksumForAsset(t *testing.T) {
	want := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	got, err := checksumForAsset([]byte(want+"  dell-infra-manager-linux-amd64\n"), "dell-infra-manager-linux-amd64")
	if err != nil || got != want {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestChecksumForAssetRejectsMissingEntry(t *testing.T) {
	if _, err := checksumForAsset([]byte("bad data\n"), "dell-infra-manager-linux-amd64"); err == nil {
		t.Fatal("expected an error")
	}
}
