package redfish

import (
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type CatalogComponent struct {
	Name           string
	Version        string
	Path           string
	ReleaseDate    string
	ComponentType  string
	SupportedModels []string
}

type catalog struct {
	Components []catalogComponent `xml:"SoftwareComponent"`
}

type catalogComponent struct {
	Path        string `xml:"path,attr"`
	ReleaseDate string `xml:"releaseDate,attr"`
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

// DownloadCatalog downloads and saves Catalog.xml.gz to cachePath.
func DownloadCatalog(catalogURL, cachePath string) error {
	resp, err := http.Get(catalogURL) //nolint:gosec
	if err != nil {
		return fmt.Errorf("download catalog: %w", err)
	}
	defer resp.Body.Close()

	f, err := os.Create(cachePath)
	if err != nil {
		return fmt.Errorf("create catalog cache: %w", err)
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

// LoadCatalog parses the gzipped XML catalog and returns all components.
func LoadCatalog(cachePath string) ([]CatalogComponent, error) {
	f, err := os.Open(cachePath)
	if err != nil {
		return nil, fmt.Errorf("open catalog: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("decompress catalog: %w", err)
	}
	defer gz.Close()

	var cat catalog
	if err := xml.NewDecoder(gz).Decode(&cat); err != nil {
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
