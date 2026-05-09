package redfish

import (
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
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
	ComponentType   string
	SupportedModels []string
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
	VendorVersion string `xml:"vendorVersion,attr"`
	ComponentType struct {
		Value string `xml:"value,attr"`
	} `xml:"ComponentType"`
	Display []struct {
		Lang  string `xml:"lang,attr"`
		Value string `xml:",chardata"`
	} `xml:"Display"`
	SupportedSystems struct {
		Brand []struct {
			Models []struct {
				Name string `xml:",chardata"`
			} `xml:"Model"`
		} `xml:"Brand"`
	} `xml:"SupportedSystems"`
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

	f, err := os.Create(cachePath)
	if err != nil {
		return false, err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return false, err
	}
	return true, nil
}

// readCatalogBytes returns the catalog as a UTF-8 byte slice, decompressing
// gzip and transcoding from UTF-16 if needed.
//
// Reality check on Dell's format (verified May 2026): they ship the catalog
// gzipped, with the inner XML encoded as UTF-16 LE — not UTF-8 as we'd assumed.
// The XML declaration even states it: <?xml version="1.0" encoding="utf-16"?>.
// Go's encoding/xml only handles UTF-8, so we transcode here.
func readCatalogBytes(cachePath string) ([]byte, error) {
	f, err := os.Open(cachePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var raw io.Reader = f
	if gz, gzErr := gzip.NewReader(f); gzErr == nil {
		defer gz.Close()
		raw = gz
	} else if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
		return nil, seekErr
	}

	data, err := io.ReadAll(raw)
	if err != nil {
		return nil, err
	}

	// UTF-16 detection — handles both BOM-prefixed and BOM-less Dell catalogs.
	// ExpectBOM means: consume a BOM if present, otherwise treat as the chosen
	// endianness. Dell ships LE without a BOM in our observed cases.
	if isUTF16(data) {
		dec := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder()
		out, _, err := transform.Bytes(dec, data)
		if err != nil {
			return nil, fmt.Errorf("transcode UTF-16 catalog: %w", err)
		}
		data = out
	}

	// Strip any bytes before the first '<' — handles UTF-8 BOM, stray
	// whitespace, leftover surrogates, etc.
	if idx := bytes.IndexByte(data, '<'); idx > 0 {
		data = data[idx:]
	}
	return data, nil
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

// newCatalogDecoder returns a UTF-8-clean xml.Decoder over the catalog file.
// CharsetReader is identity because we already produced valid UTF-8 above —
// the parser must NOT try to re-transcode based on the <?xml encoding=...?>
// declaration (which is often a lie in Dell's catalog).
func newCatalogDecoder(cachePath string) (*xml.Decoder, error) {
	data, err := readCatalogBytes(cachePath)
	if err != nil {
		return nil, err
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	return dec, nil
}

// ReadCatalogInfo returns root-level metadata (dateTime/version) without
// parsing the multi-MB component list.
func ReadCatalogInfo(cachePath string) (*CatalogInfo, error) {
	stat, _ := os.Stat(cachePath)

	dec, err := newCatalogDecoder(cachePath)
	if err != nil {
		return nil, err
	}

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
	data, err := readCatalogBytes(cachePath)
	if err != nil {
		return nil, fmt.Errorf("open catalog: %w", err)
	}

	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }

	var cat catalog
	if err := dec.Decode(&cat); err != nil {
		// Surface a sample of the bytes we tried to parse — without this,
		// "expected element name after <" leaves the user guessing whether the
		// file is HTML, gzip with the wrong header, half-downloaded, etc.
		head := data
		if len(head) > 120 {
			head = head[:120]
		}
		return nil, fmt.Errorf("parse catalog XML (%d bytes, head=%q): %w", len(data), head, err)
	}

	result := make([]CatalogComponent, 0, len(cat.Components))
	for _, c := range cat.Components {
		name := ""
		for _, d := range c.Display {
			if d.Lang == "en" {
				name = d.Value
				break
			}
		}
		var supportedModels []string
		for _, b := range c.SupportedSystems.Brand {
			for _, m := range b.Models {
				supportedModels = append(supportedModels, m.Name)
			}
		}
		result = append(result, CatalogComponent{
			Name:            name,
			Version:         c.VendorVersion,
			Path:            c.Path,
			ReleaseDate:     c.ReleaseDate,
			ComponentType:   c.ComponentType.Value,
			SupportedModels: supportedModels,
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
