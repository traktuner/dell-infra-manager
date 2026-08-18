package redfish

import "testing"

func TestNormalizeFirmwareInventoryExcludesHistoryAndDeduplicatesEquivalentState(t *testing.T) {
	components := []FirmwareComponent{
		{ID: "Current-159-2.27.0__BIOS.Setup.1-1", Name: "BIOS", Version: "2.27.0", SoftwareId: "159"},
		{ID: "Installed-159-2.27.0__BIOS.Setup.1-1", Name: "BIOS", Version: "2.27.0", SoftwareId: "159"},
		{ID: "Previous-159-2.26.1__BIOS.Setup.1-1", Name: "BIOS", Version: "2.26.1", SoftwareId: "159"},
		{ID: "Installed-25227-7.00.00.184__iDRAC.Embedded.1-1", Name: "Integrated Remote Access Controller", Version: "7.00.00.184", SoftwareId: "25227"},
		{ID: "Previous-25227-7.00.00.183__iDRAC.Embedded.1-1", Name: "Integrated Remote Access Controller", Version: "7.00.00.183", SoftwareId: "25227"},
		{ID: "Current-105008-36.11.73.00__NIC.Integrated.1-1-1", Name: "Broadcom NIC", Version: "36.11.73.00", SoftwareId: "105008"},
		{ID: "Installed-105008-36.11.73.00__NIC.Integrated.1-1-1", Name: "Broadcom NIC", Version: "36.11.73.00", SoftwareId: "105008"},
		{ID: "Current-105008-36.11.73.00__NIC.Integrated.1-2-1", Name: "Broadcom NIC", Version: "36.11.73.00", SoftwareId: "105008"},
		{ID: "Installed-105008-36.11.73.00__NIC.Integrated.1-2-1", Name: "Broadcom NIC", Version: "36.11.73.00", SoftwareId: "105008"},
	}

	got := normalizeFirmwareInventory(components)
	if len(got) != 4 {
		t.Fatalf("expected four running components, got %#v", got)
	}
	for _, component := range got {
		if len(component.ID) >= len("Previous-") && component.ID[:len("Previous-")] == "Previous-" {
			t.Fatalf("rollback history was retained: %#v", component)
		}
	}
	if got[0].ID != "Installed-159-2.27.0__BIOS.Setup.1-1" {
		t.Fatalf("Installed record was not preferred: %#v", got[0])
	}
	if got[2].ID == got[3].ID {
		t.Fatalf("distinct physical NIC targets were collapsed: %#v", got)
	}
}

func TestNormalizeFirmwareInventoryKeepsConflictingCurrentAndInstalledVersions(t *testing.T) {
	components := []FirmwareComponent{
		{ID: "Current-159-2.27.0__BIOS.Setup.1-1", Name: "BIOS", Version: "2.27.0", SoftwareId: "159"},
		{ID: "Installed-159-2.26.1__BIOS.Setup.1-1", Name: "BIOS", Version: "2.26.1", SoftwareId: "159"},
	}

	got := normalizeFirmwareInventory(components)
	if len(got) != 2 {
		t.Fatalf("conflicting running records must remain visible: %#v", got)
	}
}
