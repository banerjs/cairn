package version

import "testing"

func TestVersionMetadata(t *testing.T) {
	if Name == "" {
		t.Fatal("Name")
	}
	if Version == "" {
		t.Fatal("Version")
	}
	if len(ManifestSchemas) < 1 {
		t.Fatal("ManifestSchemas")
	}
}
