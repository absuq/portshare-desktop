package main

import (
	"os"
	"strings"
	"testing"
)

func TestWindowsManifestRequestsAdministrator(t *testing.T) {
	data, err := os.ReadFile("portshare.exe.manifest")
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(data)
	if !strings.Contains(manifest, `requestedExecutionLevel level="requireAdministrator" uiAccess="false"`) {
		t.Fatalf("manifest does not request administrator privileges:\n%s", manifest)
	}
}

func TestWindowsResourceEmbedsManifest(t *testing.T) {
	data, err := os.ReadFile("portshare.rc")
	if err != nil {
		t.Fatal(err)
	}
	rc := string(data)
	if !strings.Contains(rc, `1 24 "portshare.exe.manifest"`) {
		t.Fatalf("resource file does not embed application manifest:\n%s", rc)
	}
}
