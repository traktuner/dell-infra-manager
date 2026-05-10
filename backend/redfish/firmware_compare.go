package redfish

// ComponentStatus is one row of catalog comparison data — produced by both the
// /firmware/available API endpoint and the daily firmware-update digest job.
//
// Outdated == false means we have catalog data for this component and the
// installed version equals the latest catalog version.
type ComponentStatus struct {
	Component        string `json:"component"`
	InstalledVersion string `json:"installed_version"`
	AvailableVersion string `json:"available_version"`
	ReleaseDate      string `json:"release_date"`
	CatalogPath      string `json:"catalog_path"`
	Outdated         bool   `json:"outdated"`
}

// CompareInventory matches installed firmware against the Dell catalog using
// SoftwareId ↔ <SupportedDevices>.componentID — the stable identifier Dell's
// own DRM uses. Display names and component types are unreliable matchers.
//
// Components with no catalog match are NOT included in the result; callers
// that need to render "unknown" status look them up by absence.
func CompareInventory(installed []FirmwareComponent, catalog []CatalogComponent, model string) []ComponentStatus {
	catalogForModel := FilterByModel(catalog, model)

	// componentID → newest catalog component for that ID.
	byComponentID := make(map[string]CatalogComponent, len(catalogForModel))
	for _, cat := range catalogForModel {
		for _, cid := range cat.ComponentIDs {
			existing, ok := byComponentID[cid]
			if !ok || cat.ReleaseDate > existing.ReleaseDate {
				byComponentID[cid] = cat
			}
		}
	}

	out := make([]ComponentStatus, 0, len(installed))
	for _, inst := range installed {
		if inst.SoftwareId == "" {
			continue
		}
		cat, ok := byComponentID[inst.SoftwareId]
		if !ok {
			continue
		}
		out = append(out, ComponentStatus{
			Component:        inst.Name,
			InstalledVersion: inst.Version,
			AvailableVersion: cat.Version,
			ReleaseDate:      cat.ReleaseDate,
			CatalogPath:      cat.Path,
			Outdated:         cat.Version != inst.Version,
		})
	}
	return out
}
