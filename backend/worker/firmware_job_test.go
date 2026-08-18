package worker

import "testing"

func TestValidatedDellDownloadURL(t *testing.T) {
	got, filename, err := validatedDellDownloadURL("FOLDER123/update.EXE")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://downloads.dell.com/FOLDER123/update.EXE" || filename != "update.EXE" {
		t.Fatalf("got URL=%q filename=%q", got, filename)
	}
}

func TestValidatedDellDownloadURLRejectsUntrustedPaths(t *testing.T) {
	for _, value := range []string{
		"https://example.com/update.EXE",
		"//example.com/update.EXE",
		"../update.EXE",
		"FOLDER/../update.EXE",
		"FOLDER/update.EXE?token=value",
		"FOLDER\\update.EXE",
	} {
		if _, _, err := validatedDellDownloadURL(value); err == nil {
			t.Errorf("expected %q to be rejected", value)
		}
	}
}
