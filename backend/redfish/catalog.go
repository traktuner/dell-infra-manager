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

	"golang.org/x/text/encoding/charmap"
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

// openCatalogXML opens cachePath, decompresses it and returns an xml.Decoder
// whose input is transcoded to UTF-8.
//
// Dell's catalog XML is declared as UTF-8 but historically ships bytes from
// Windows-1252 (e.g. "®" = 0xAE in component display names). Go's xml package
// rejects any byte that is not valid UTF-8, so we pipe the decompressed stream
// through a Windows-1252 → UTF-8 transcoder. Windows-1252 is a strict superset
// of Latin-1 and maps every byte, so this is safe even for truly UTF-8 content.
func openCatalogXML(cachePath string) (*os.File, *gzip.Reader, *xml.Decoder, error) {
	f, err := os.Open(cachePath)
	if err != nil {
		return nil, nil, nil, err
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		f.Close()
		return nil, nil, nil, err
	}
	transcoded := transform.NewReader(gz, charmap.Windows1252.NewDecoder())
	dec := xml.NewDecoder(transcoded)
	return f, gz, dec, nil
}

// ReadCatalogInfo returns the dateTime/version metadata without parsing every
// SoftwareComponent — cheap to call for status display.
func ReadCatalogInfo(cachePath string) (*CatalogInfo, error) {
	f, err := os.Open(cachePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	stat, _ := f.Stat()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	transcoded := transform.NewReader(gz, charmap.Windows1252.NewDecoder())

	// We only want the root element's attributes — stream until we see it,
	// then bail. Avoids parsing the (multi-MB) component list.
	dec := xml.NewDecoder(transcoded)
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		if se, ok := tok.(xml.StartElement); ok {
			info := &CatalogInfo{}
			for _, a := range se.Attr {
				if a.Name.Local == "dateTime" {
					info.DateTime = a.Value
				}
				if a.Name.Local == "version" {
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

// LoadCatalog parses the gzipped XML catalog and returns all components.
func LoadCatalog(cachePath string) ([]CatalogComponent, error) {
	f, gz, dec, err := openCatalogXML(cachePath)
	if err != nil {
		return nil, fmt.Errorf("open catalog: %w", err)
	}
	defer gz.Close()
	defer f.Close()

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
