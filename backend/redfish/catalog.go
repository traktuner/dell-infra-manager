package redfish

import (
	"bufio"
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

type CatalogComponent struct {
	Name            string
	Version         string
	Path            string
	ReleaseDate     string
	DateTime        string
	ComponentType   string
	SupportedModels []string
	// ComponentIDs are Dell's stable per-device IDs from <SupportedDevices>.
	// Match these against FirmwareComponent.SoftwareId from iDRAC inventory.
	ComponentIDs []string
}

type CatalogInfo struct {
	DateTime  string `json:"date_time"`
	Version   string `json:"version"`
	FetchedAt string `json:"fetched_at"`
}

type catalog struct {
	DateTime   string             `xml:"dateTime,attr"`
	Version    string             `xml:"version,attr"`
	Components []catalogComponent `xml:"SoftwareComponent"`
}

type catalogComponent struct {
	Path          string `xml:"path,attr"`
	ReleaseDate   string `xml:"releaseDate,attr"`
	DateTime      string `xml:"dateTime,attr"`
	VendorVersion string `xml:"vendorVersion,attr"`
	ComponentType struct {
		Value string `xml:"value,attr"`
	} `xml:"ComponentType"`
	Display []struct {
		Lang  string `xml:"lang,attr"`
		Value string `xml:",chardata"`
	} `xml:"Display"`
	Name struct {
		Display []struct {
			Lang  string `xml:"lang,attr"`
			Value string `xml:",chardata"`
		} `xml:"Display"`
	} `xml:"Name"`
	SupportedSystems struct {
		Brand []struct {
			Models []struct {
				Name    string `xml:",chardata"`
				Display []struct {
					Lang  string `xml:"lang,attr"`
					Value string `xml:",chardata"`
				} `xml:"Display"`
			} `xml:"Model"`
		} `xml:"Brand"`
	} `xml:"SupportedSystems"`
	SupportedDevices struct {
		Devices []struct {
			ComponentID string `xml:"componentID,attr"`
		} `xml:"Device"`
	} `xml:"SupportedDevices"`
}

// catalogClient is a dedicated HTTP client for catalog downloads.
// DisableCompression: Dell ships Catalog.xml.gz already compressed and we want
// to save the raw bytes; otherwise Go would transparently gunzip the response.
var catalogClient = &http.Client{
	Timeout:   10 * time.Minute,
	Transport: &http.Transport{DisableCompression: true},
}

// DownloadCatalog overwrites cachePath with a fresh copy from Dell.
func DownloadCatalog(catalogURL, cachePath string) error {
	_, err := downloadCatalog(catalogURL, cachePath, false)
	return err
}

// DownloadCatalogIfModified does a conditional GET. Returns true if the file
// was updated, false on 304 Not Modified.
func DownloadCatalogIfModified(catalogURL, cachePath string) (bool, error) {
	return downloadCatalog(catalogURL, cachePath, true)
}

func downloadCatalog(catalogURL, cachePath string, conditional bool) (bool, error) {
	req, err := http.NewRequest(http.MethodGet, catalogURL, nil)
	if err != nil {
		return false, err
	}
	if conditional {
		if info, statErr := os.Stat(cachePath); statErr == nil {
			req.Header.Set("If-Modified-Since", info.ModTime().UTC().Format(http.TimeFormat))
		}
	}

	resp, err := catalogClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("download catalog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("download catalog: HTTP %d", resp.StatusCode)
	}

	dir := filepath.Dir(cachePath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return false, fmt.Errorf("create catalog directory: %w", err)
	}
	f, err := os.CreateTemp(dir, ".catalog-*.tmp")
	if err != nil {
		return false, err
	}
	tmpPath := f.Name()
	defer os.Remove(tmpPath)
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		return false, err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return false, fmt.Errorf("sync catalog download: %w", err)
	}
	if err := f.Close(); err != nil {
		return false, fmt.Errorf("close catalog download: %w", err)
	}

	// Validate the complete temporary file before replacing the last known-good
	// catalog. Interrupted downloads and HTML error pages must never destroy the
	// working cache used by every server comparison.
	components, err := LoadCatalog(tmpPath)
	if err != nil {
		return false, fmt.Errorf("validate downloaded catalog: %w", err)
	}
	if len(components) == 0 {
		return false, fmt.Errorf("validate downloaded catalog: no software components")
	}
	if err := os.Chmod(tmpPath, 0o640); err != nil {
		return false, fmt.Errorf("set catalog permissions: %w", err)
	}
	if err := os.Rename(tmpPath, cachePath); err != nil {
		return false, fmt.Errorf("replace catalog cache: %w", err)
	}
	return true, nil
}

// openCatalogDecoder streams gzip decompression, UTF-16 conversion, and XML
// parsing. The Dell catalog can expand to tens of megabytes, so the appliance
// must not keep both the full XML and a second decoded object tree in memory.
func openCatalogDecoder(cachePath string) (*xml.Decoder, func() error, error) {
	f, err := os.Open(cachePath)
	if err != nil {
		return nil, nil, err
	}
	fileReader := bufio.NewReader(f)
	raw := io.Reader(fileReader)
	var gz *gzip.Reader
	header, _ := fileReader.Peek(2)
	if len(header) == 2 && header[0] == 0x1f && header[1] == 0x8b {
		gz, err = gzip.NewReader(fileReader)
		if err != nil {
			_ = f.Close()
			return nil, nil, fmt.Errorf("open gzip catalog: %w", err)
		}
		raw = gz
	}
	closeCatalog := func() error {
		if gz != nil {
			_ = gz.Close()
		}
		return f.Close()
	}

	bufferedRaw := bufio.NewReader(raw)
	sample, _ := bufferedRaw.Peek(16)
	decoded := io.Reader(bufferedRaw)
	if isUTF16(sample) {
		decoded = transform.NewReader(
			bufferedRaw,
			unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder(),
		)
	}

	// Strip a BOM or other harmless prefix without buffering the document.
	clean := bufio.NewReader(decoded)
	for {
		b, readErr := clean.ReadByte()
		if readErr != nil {
			_ = closeCatalog()
			return nil, nil, fmt.Errorf("find catalog XML start: %w", readErr)
		}
		if b == '<' {
			if err := clean.UnreadByte(); err != nil {
				_ = closeCatalog()
				return nil, nil, err
			}
			break
		}
	}

	dec := xml.NewDecoder(clean)
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	return dec, closeCatalog, nil
}

// isUTF16 returns true if data looks like UTF-16 (BOM or null-byte pattern).
// We check the first 16 bytes — enough to disambiguate from any plausible
// UTF-8 / Windows-1252 content (XML always starts with '<' or whitespace,
// neither of which produces alternating null bytes in single-byte encodings).
func isUTF16(data []byte) bool {
	// BOMs first.
	if len(data) >= 2 {
		if (data[0] == 0xFF && data[1] == 0xFE) || (data[0] == 0xFE && data[1] == 0xFF) {
			return true
		}
	}
	// BOM-less detection: Dell's catalog. ASCII chars in UTF-16 LE always have
	// 0x00 at every odd index. Sample the first 16 bytes.
	n := len(data)
	if n > 16 {
		n = 16
	}
	if n < 4 {
		return false
	}
	zerosAtOdd := 0
	for i := 1; i < n; i += 2 {
		if data[i] == 0 {
			zerosAtOdd++
		}
	}
	// If ≥ 75% of odd-index bytes are zero, treat as UTF-16 LE.
	return zerosAtOdd*4 >= (n/2)*3
}

// ReadCatalogInfo returns root-level metadata (dateTime/version) without
// parsing the multi-MB component list.
func ReadCatalogInfo(cachePath string) (*CatalogInfo, error) {
	stat, _ := os.Stat(cachePath)

	dec, closeCatalog, err := openCatalogDecoder(cachePath)
	if err != nil {
		return nil, err
	}
	defer closeCatalog()

	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		info := &CatalogInfo{}
		for _, a := range se.Attr {
			switch a.Name.Local {
			case "dateTime":
				info.DateTime = a.Value
			case "version":
				info.Version = a.Value
			}
		}
		if stat != nil {
			info.FetchedAt = stat.ModTime().UTC().Format(time.RFC3339)
		}
		return info, nil
	}
}

// LoadCatalog parses the catalog and returns all components.
func LoadCatalog(cachePath string) ([]CatalogComponent, error) {
	dec, closeCatalog, err := openCatalogDecoder(cachePath)
	if err != nil {
		return nil, fmt.Errorf("open catalog: %w", err)
	}
	defer closeCatalog()

	result := make([]CatalogComponent, 0, 1024)
	for {
		token, tokenErr := dec.Token()
		if tokenErr == io.EOF {
			break
		}
		if tokenErr != nil {
			return nil, fmt.Errorf("parse catalog XML after %d components: %w", len(result), tokenErr)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "SoftwareComponent" {
			continue
		}
		var c catalogComponent
		if err := dec.DecodeElement(&c, &start); err != nil {
			return nil, fmt.Errorf("parse catalog component %d: %w", len(result)+1, err)
		}
		name := ""
		displays := c.Display
		if len(c.Name.Display) > 0 {
			displays = c.Name.Display
		}
		for _, d := range displays {
			if d.Lang == "en" {
				name = strings.TrimSpace(d.Value)
				break
			}
		}
		var supportedModels []string
		for _, b := range c.SupportedSystems.Brand {
			for _, m := range b.Models {
				modelName := strings.TrimSpace(m.Name)
				for _, d := range m.Display {
					if d.Lang == "en" {
						modelName = strings.TrimSpace(d.Value)
						break
					}
				}
				if modelName != "" {
					supportedModels = append(supportedModels, modelName)
				}
			}
		}
		var componentIDs []string
		for _, d := range c.SupportedDevices.Devices {
			if d.ComponentID != "" {
				componentIDs = append(componentIDs, d.ComponentID)
			}
		}
		result = append(result, CatalogComponent{
			Name:            name,
			Version:         c.VendorVersion,
			Path:            c.Path,
			ReleaseDate:     c.ReleaseDate,
			DateTime:        c.DateTime,
			ComponentType:   c.ComponentType.Value,
			SupportedModels: supportedModels,
			ComponentIDs:    componentIDs,
		})
	}
	return result, nil
}

// FilterByModel returns catalog components applicable to a given server model name.
func FilterByModel(components []CatalogComponent, modelName string) []CatalogComponent {
	if modelName == "" {
		return components
	}
	modelUpper := strings.ToUpper(modelName)
	result := make([]CatalogComponent, 0, len(components))
	for _, c := range components {
		for _, m := range c.SupportedModels {
			mu := strings.ToUpper(m)
			if strings.Contains(mu, modelUpper) || strings.Contains(modelUpper, mu) {
				result = append(result, c)
				break
			}
		}
	}
	return result
}
