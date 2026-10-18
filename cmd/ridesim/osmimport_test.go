package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOSMImportWritesDraftOnce(t *testing.T) {
	out := filepath.Join(t.TempDir(), "draft.yaml")
	args := []string{"-config", "../../osm/wdw-dhs/import.yaml", "-out", out}
	if err := runOSMImport(args, io.Discard); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"© OpenStreetMap contributors", "id: rock_n_roller_coaster, kind: ride", "kind: junction"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("draft lacks %q", want)
		}
	}
	if err := runOSMImport(args, io.Discard); err == nil || !strings.Contains(err.Error(), "never rewrites") {
		t.Errorf("second import over the draft: err = %v, want a refusal", err)
	}
}
