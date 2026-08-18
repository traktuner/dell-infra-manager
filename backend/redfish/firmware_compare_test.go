package redfish

import "testing"

func TestCompareInventoryKeepsUnknownComponentsExplicit(t *testing.T) {
	installed := []FirmwareComponent{{ID: "Installed-BIOS", Name: "BIOS", Version: "2.27.0"}}
	got := CompareInventory(installed, nil, "PowerEdge R640")
	if len(got) != 1 || got[0].ComparisonStatus != "unknown" || got[0].Outdated {
		t.Fatalf("unexpected comparison: %#v", got)
	}
}

func TestCompareInventoryUsesSoftwareIDAndNaturalVersions(t *testing.T) {
	catalog := []CatalogComponent{{
		Name: "BIOS", Version: "2.27.0", ReleaseDate: "2026-08-01", Path: "FOLDER/bios.exe",
		SupportedModels: []string{"PowerEdge R640"}, ComponentIDs: []string{"00159"},
	}}
	tests := []struct {
		version string
		status  string
	}{
		{"2.26.1", "outdated"},
		{"2.27.0", "current"},
		{"2.28.0", "newer"},
	}
	for _, tt := range tests {
		got := CompareInventory([]FirmwareComponent{{
			ID: "Installed-BIOS", Name: "BIOS", Version: tt.version, SoftwareId: "159", Updateable: true,
		}}, catalog, "PowerEdge R640")
		if len(got) != 1 || got[0].ComparisonStatus != tt.status {
			t.Fatalf("version %s: %#v", tt.version, got)
		}
	}
}

func TestCompareInventoryPreservesUpdateableFlag(t *testing.T) {
	catalog := []CatalogComponent{{
		Name: "BIOS", Version: "2.0", Path: "FOLDER/bios.exe",
		SupportedModels: []string{"PowerEdge R640"}, ComponentIDs: []string{"159"},
	}}
	got := CompareInventory([]FirmwareComponent{{
		ID: "Installed-BIOS", Name: "BIOS", Version: "1.0", SoftwareId: "159", Updateable: false,
	}}, catalog, "PowerEdge R640")
	if len(got) != 1 || got[0].Updateable || !got[0].Outdated {
		t.Fatalf("unexpected comparison: %#v", got)
	}
}

func TestCompareInventoryPreservesDuplicateNames(t *testing.T) {
	installed := []FirmwareComponent{
		{ID: "Installed-1", Name: "BIOS", Version: "1.0", SoftwareId: "1"},
		{ID: "Installed-2", Name: "BIOS", Version: "2.0", SoftwareId: "2"},
	}
	got := CompareInventory(installed, nil, "")
	if len(got) != 2 || got[0].InventoryID == got[1].InventoryID {
		t.Fatalf("duplicate display names were collapsed: %#v", got)
	}
}

func TestCompareInventorySelectsNewestCatalogDate(t *testing.T) {
	catalog := []CatalogComponent{
		{Name: "BIOS", Version: "2.0", DateTime: "2026-03-18T10:00:00Z", ComponentIDs: []string{"159"}},
		{Name: "BIOS", Version: "3.0", DateTime: "2026-11-02T10:00:00Z", ComponentIDs: []string{"159"}},
	}
	got := CompareInventory([]FirmwareComponent{{
		ID: "Installed-BIOS", Name: "BIOS", Version: "1.0", SoftwareId: "159", Updateable: true,
	}}, catalog, "")
	if len(got) != 1 || got[0].AvailableVersion != "3.0" {
		t.Fatalf("unexpected comparison: %#v", got)
	}
}
