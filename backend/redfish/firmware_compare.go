package redfish

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ComponentStatus is one row of catalog comparison data — produced by both the
// /firmware/available API endpoint and the daily firmware-update digest job.
//
// Outdated == false means we have catalog data for this component and the
// installed version equals the latest catalog version.
type ComponentStatus struct {
	Component        string `json:"component"`
	InventoryID      string `json:"inventory_id"`
	SoftwareID       string `json:"software_id"`
	InstalledVersion string `json:"installed_version"`
	AvailableVersion string `json:"available_version"`
	ReleaseDate      string `json:"release_date"`
	CatalogPath      string `json:"catalog_path"`
	Updateable       bool   `json:"updateable"`
	Outdated         bool   `json:"outdated"`
	Matched          bool   `json:"matched"`
	ComparisonStatus string `json:"comparison_status"`
	Reason           string `json:"reason,omitempty"`
}

// CompareInventory matches installed firmware against the Dell catalog using
// SoftwareId ↔ <SupportedDevices>.componentID — the stable identifier Dell's
// own DRM uses. Display names and component types are unreliable matchers.
//
// Every installed component is included. A missing match stays explicit so a
// caller cannot mistake "no comparison was possible" for "up to date".
func CompareInventory(installed []FirmwareComponent, catalog []CatalogComponent, model string) []ComponentStatus {
	// Normalize here as well as at collection time so caches created by older
	// releases stop producing false warnings immediately after an upgrade.
	installed = normalizeFirmwareInventory(installed)
	catalogForModel := FilterByModel(catalog, model)

	// componentID → newest catalog component for that ID.
	byComponentID := make(map[string]CatalogComponent, len(catalogForModel))
	for _, cat := range catalogForModel {
		for _, cid := range cat.ComponentIDs {
			key := normalizeComponentID(cid)
			if key == "" {
				continue
			}
			existing, ok := byComponentID[key]
			if !ok || catalogComponentIsNewer(cat, existing) {
				byComponentID[key] = cat
			}
		}
	}

	out := make([]ComponentStatus, 0, len(installed))
	for _, inst := range installed {
		row := ComponentStatus{
			Component:        inst.Name,
			InventoryID:      inst.ID,
			SoftwareID:       inst.SoftwareId,
			InstalledVersion: inst.Version,
			Updateable:       inst.Updateable,
			ComparisonStatus: "unknown",
		}
		if strings.TrimSpace(inst.SoftwareId) == "" {
			row.Reason = "iDRAC did not report a SoftwareId for this inventory item"
			out = append(out, row)
			continue
		}
		cat, ok := byComponentID[normalizeComponentID(inst.SoftwareId)]
		if !ok {
			row.Reason = "no matching component for this server model in the Dell catalog"
			out = append(out, row)
			continue
		}
		row.Matched = true
		row.AvailableVersion = cat.Version
		row.ReleaseDate = cat.ReleaseDate
		row.CatalogPath = cat.Path
		switch compareFirmwareVersions(inst.Version, cat.Version) {
		case -1:
			row.Outdated = true
			row.ComparisonStatus = "outdated"
			if !row.Updateable {
				row.Reason = "iDRAC reports that this inventory item is not updateable"
			}
		case 1:
			row.ComparisonStatus = "newer"
			row.Reason = "installed version is newer than the catalog version"
		default:
			row.ComparisonStatus = "current"
		}
		out = append(out, row)
	}
	return out
}

func catalogComponentIsNewer(candidate, existing CatalogComponent) bool {
	for _, value := range []struct {
		candidate string
		existing  string
		layout    string
	}{
		{candidate.DateTime, existing.DateTime, time.RFC3339},
		{candidate.ReleaseDate, existing.ReleaseDate, "January 2, 2006"},
	} {
		candidateTime, candidateErr := time.Parse(value.layout, value.candidate)
		existingTime, existingErr := time.Parse(value.layout, value.existing)
		if candidateErr == nil && existingErr == nil {
			return candidateTime.After(existingTime)
		}
		if candidateErr == nil && existingErr != nil {
			return true
		}
	}
	return compareFirmwareVersions(existing.Version, candidate.Version) < 0
}

func normalizeComponentID(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "0X")
	if _, err := strconv.ParseUint(value, 10, 64); err == nil {
		value = strings.TrimLeft(value, "0")
		if value == "" {
			return "0"
		}
	}
	return value
}

var firmwareVersionToken = regexp.MustCompile(`[0-9]+|[A-Za-z]+`)

// compareFirmwareVersions performs a natural comparison. Dell versions mix
// dotted decimal parts and alphanumeric suffixes, for example 7.00.00.184 and
// 4301A73. It returns -1 when installed is older, 0 when equivalent, and 1
// when installed is newer.
func compareFirmwareVersions(installed, available string) int {
	a := firmwareVersionToken.FindAllString(strings.TrimSpace(installed), -1)
	b := firmwareVersionToken.FindAllString(strings.TrimSpace(available), -1)
	if len(a) == 0 || len(b) == 0 {
		return strings.Compare(strings.ToUpper(installed), strings.ToUpper(available))
	}
	max := len(a)
	if len(b) > max {
		max = len(b)
	}
	for i := 0; i < max; i++ {
		av, bv := "0", "0"
		if i < len(a) {
			av = a[i]
		}
		if i < len(b) {
			bv = b[i]
		}
		an, aErr := strconv.ParseUint(av, 10, 64)
		bn, bErr := strconv.ParseUint(bv, 10, 64)
		if aErr == nil && bErr == nil {
			if an < bn {
				return -1
			}
			if an > bn {
				return 1
			}
			continue
		}
		cmp := strings.Compare(strings.ToUpper(av), strings.ToUpper(bv))
		if cmp != 0 {
			return cmp
		}
	}
	return 0
}
