package redfish

import (
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/net/html/charset"
)

type CatalogComponent struct {
	Name            string
	Version         string
	Path            string
	ReleaseDate     string
	ComponentType   string
	SupportedModels []string
}

// CatalogInfo is the high-level metadata Dell stamps on the catalog root —
// useful for telling the user "this catalog is from <date>".
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
// DisableCompression ensures Go does NOT transparently gunzip the response —
// the catalog is already a .gz file and we want to save the raw bytes so that
// our own gzip.NewReader calls work correctly later.
// The 10-minute timeout covers slow connections downloading the ~100 MB file.
var catalogClient = &http.Client{
	Timeout: 10 * time.Minute,
	Transport: &http.Transport{
		DisableCompression: true,
	},
}

// DownloadCatalog unconditionally downloads the catalog to cachePath.
func DownloadCatalog(catalogURL, cachePath string) error {
	req, err := http.NewRequest(http.MethodGet, catalogURL, nil)
	if err != nil {
		return fmt.Errorf("download catalog: %w", err)
	}
	resp, err := catalogClient.Do(req) //nolint:gosec
	if err != nil {
		return fmt.Errorf("download catalog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download catalog: HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(cachePath)
	if err != nil {
		return fmt.Errorf("create catalog cache: %w", err)
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

// DownloadCatalogIfModified does a conditional GET using If-Modified-Since
// against the local file's mtime. Returns (downloaded, error).
//
//	downloaded=true  → the catalog was updated on disk
//	downloaded=false → server returned 304 Not Modified, local copy is fresh
//
// Intended for the user-facing "Check for Updates" button: cheap when nothing
// changed (Dell answers in milliseconds without a body), full download only
// when a newer catalog is actually available.
func DownloadCatalogIfModified(catalogURL, cachePath string) (bool, error) {
	req, err := http.NewRequest(http.MethodGet, catalogURL, nil)
	if err != nil {
		return false, err
	}
	if info, statErr := os.Stat(cachePath); statErr == nil {
		req.Header.Set("If-Modified-Since", info.ModTime().UTC().Format(http.TimeFormat))
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
		return false, fmt.Errorf("create catalog cache: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return false, err
	}
	return true, nil
}

// openCatalogXML opens cachePath, decompresses it (gzip or plain), and returns
// an xml.Decoder that handles any encoding declared in the XML header.
//
// Two failure modes are handled gracefully:
//  1. File is plain XML (no gzip) — happens when the old downloader let Go's
//     HTTP stack transparently decompress a Content-Encoding:gzip response.
//     We detect this by trying gzip.NewReader and falling back on error.
//  2. Non-UTF-8 encoding declaration (e.g. ISO-8859-1, Windows-1252) — handled
//     by charset.NewReaderLabel which reads the <?xml encoding="..."?> header
//     and wraps the reader in the correct transcoder automatically.
//
// Returns (file, closer, decoder, error). The caller must call closer() when done.
func openCatalogXML(cachePath string) (*os.File, func(), *xml.Decoder, error) {
	f, err := os.Open(cachePath)
	if err != nil {
		return nil, nil, nil, err
	}

	var xmlReader io.Reader
	var gzCloser io.Closer

	gz, gzErr := gzip.NewReader(f)
	if gzErr != nil {
		// Not a gzip file — treat as plain XML.
		if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
			f.Close()
			return nil, nil, nil, fmt.Errorf("seek: %w", seekErr)
		}
		xmlReader = f
	} else {
		xmlReader = gz
		gzCloser = gz
	}

	closer := func() {
		if gzCloser != nil {
			gzCloser.Close()
		}
		f.Close()
	}

	dec := xml.NewDecoder(xmlReader)
	// CharsetReader lets the XML decoder handle any encoding declaration in the
	// <?xml?> header — UTF-8, ISO-8859-1, Windows-1252, etc. — without us having
	// to know in advance what Dell ships.
	dec.CharsetReader = charset.NewReaderLabel

	return f, closer, dec, nil
}

// ReadCatalogInfo returns the dateTime/version metadata without parsing every
// SoftwareComponent — cheap to call for status display.
func ReadCatalogInfo(cachePath string) (*CatalogInfo, error) {
	// Stat before opening so we can record the file mtime as fetched_at.
	stat, _ := os.Stat(cachePath)

	f, closer, dec, err := openCatalogXML(cachePath)
	if err != nil {
		return nil, err
	}
	defer closer()
	_ = f // f kept alive via closer

	// Stream tokens until we hit the root start element — it carries the
	// dateTime and version attributes. Bail immediately after to avoid
	// reading the entire (multi-MB) component list.
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		if se, ok := tok.(xml.StartElement); ok {
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
}

// LoadCatalog parses the catalog and returns all components.
func LoadCatalog(cachePath string) ([]CatalogComponent, error) {
	_, closer, dec, err := openCatalogXML(cachePath)
	if err != nil {
		return nil, fmt.Errorf("open catalog: %w", err)
	}
	defer closer()

	var cat catalog
	if err := dec.Decode(&cat); err != nil {
		return nil, fmt.Errorf("parse catalog XML: %w", err)
	}

	var result []CatalogComponent
	for _, c := range cat.Components {
		name := ""
		for _, d := range c.Display {
			if d.Lang == "en" {
				name = d.Value
				break
			}
		}

		var models []string
		for _, b := range c.SupportedSystems.Brand {
			for _, m := range b.Models {
				models = append(models, m.Name)
			}
		}

		result = append(result, CatalogComponent{
			Name:            name,
			Version:         c.VendorVersion,
			Path:            c.Path,
			ReleaseDate:     c.ReleaseDate,
			ComponentType:   c.ComponentType.Value,
			SupportedModels: models,
		})
	}
	return result, nil
}

// FilterByModel returns catalog components applicable to a given server model name.
func FilterByModel(components []CatalogComponent, modelName string) []CatalogComponent {
	var result []CatalogComponent
	modelUpper := strings.ToUpper(modelName)
	for _, c := range components {
		for _, m := range c.SupportedModels {
			if strings.Contains(strings.ToUpper(m), modelUpper) ||
				strings.Contains(modelUpper, strings.ToUpper(m)) {
				result = append(result, c)
				break
			}
		}
	}
	return result
}
