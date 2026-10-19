package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRealDataSucceeds(t *testing.T) {
	var out bytes.Buffer
	if err := dispatch([]string{"validate", "-data", "../../data"}, &out); err != nil {
		t.Fatalf("validate: %v\n%s", err, out.String())
	}
	for _, want := range []string{"WARN  continuous.spacing_vs_route", "CONFLICT ride_duration_s: wiki=82, rcdb=70", "OK: 41 file(s)"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report missing %q", want)
		}
	}
}

func TestValidateFailsOnErrorCheck(t *testing.T) {
	src, err := os.ReadFile("../../internal/config/testdata/test-fx-coaster.yaml")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	broken := bytes.Replace(src, []byte("default_in_service: 2"), []byte("default_in_service: 9"), 1)
	if err := os.WriteFile(filepath.Join(dir, "test-fx-coaster.yaml"), broken, 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := dispatch([]string{"validate", "-data", dir}, &out); !errors.Is(err, errFailed) {
		t.Fatalf("err = %v, want errFailed\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "FAIL  coaster.fleet_order") {
		t.Errorf("report missing fleet_order failure:\n%s", out.String())
	}
}
