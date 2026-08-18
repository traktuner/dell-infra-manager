package redfish

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

const testCatalogXML = `<?xml version="1.0" encoding="utf-8"?>
<Manifest dateTime="2026-08-18T00:00:00Z" version="26.08.18">
  <SoftwareComponent path="FOLDER/update.EXE" releaseDate="2026-08-01" vendorVersion="2.0">
    <Name><Display lang="en">BIOS Update</Display></Name>
    <ComponentType value="BIOS" />
    <SupportedSystems><Brand><Model><Display lang="en">R640</Display></Model></Brand></SupportedSystems>
    <SupportedDevices><Device componentID="159" /></SupportedDevices>
  </SoftwareComponent>
</Manifest>`

func TestLoadCatalogStreamsPlainUTF8AndGzipUTF16(t *testing.T) {
	utf16Data, _, err := transform.Bytes(
		unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder(),
		[]byte(testCatalogXML),
	)
	if err != nil {
		t.Fatal(err)
	}
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err := zw.Write(utf16Data); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	for name, data := range map[string][]byte{
		"plain-utf8": []byte(testCatalogXML),
		"gzip-utf16": compressed.Bytes(),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Catalog.xml.gz")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			info, err := ReadCatalogInfo(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Version != "26.08.18" {
				t.Fatalf("unexpected catalog info: %#v", info)
			}
			components, err := LoadCatalog(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(components) != 1 || components[0].Version != "2.0" || components[0].Path != "FOLDER/update.EXE" {
				t.Fatalf("unexpected components: %#v", components)
			}
			if len(components[0].ComponentIDs) != 1 || components[0].ComponentIDs[0] != "159" {
				t.Fatalf("unexpected component IDs: %#v", components[0].ComponentIDs)
			}
			if components[0].Name != "BIOS Update" || len(components[0].SupportedModels) != 1 || components[0].SupportedModels[0] != "R640" {
				t.Fatalf("unexpected display fields: %#v", components[0])
			}
		})
	}
}
