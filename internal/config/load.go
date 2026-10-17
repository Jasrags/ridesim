package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Kind tells what a loaded file is.
type Kind int

const (
	KindCoaster Kind = iota + 1
	KindContinuous
	KindPark
)

func (k Kind) String() string {
	switch k {
	case KindCoaster:
		return ArchetypeMultiTrainCoaster
	case KindContinuous:
		return ArchetypeContinuousLoad
	case KindPark:
		return "park"
	}
	return "unknown"
}

// File is one loaded data file. Exactly one of Coaster, Continuous, Park is set.
type File struct {
	Path       string
	Kind       Kind
	Coaster    *CoasterFile
	Continuous *ContinuousFile
	Park       *ParkFile
}

// RideID returns the ride id for ride files and "" for park files.
func (f *File) RideID() string {
	switch f.Kind {
	case KindCoaster:
		return f.Coaster.Ride.ID
	case KindContinuous:
		return f.Continuous.Ride.ID
	}
	return ""
}

// RideInfo returns the ride section for ride files and nil for park files.
func (f *File) RideInfo() *RideInfo {
	switch f.Kind {
	case KindCoaster:
		return &f.Coaster.Ride
	case KindContinuous:
		return &f.Continuous.Ride
	}
	return nil
}

// ErrUnknownVersion is returned (wrapped) when schema_version is missing or not SchemaVersion.
var ErrUnknownVersion = errors.New("unsupported schema_version")

// header is the minimal probe decoded before dispatching to a typed struct.
type header struct {
	SchemaVersion *string `yaml:"schema_version"`
	Ride          *struct {
		Archetype string `yaml:"archetype"`
	} `yaml:"ride"`
	Park *yaml.Node `yaml:"park"`
}

// LoadBytes parses one file. name is used for error messages and the filename invariant.
func LoadBytes(name string, data []byte) (*File, error) {
	var h header
	if err := yaml.Unmarshal(data, &h); err != nil {
		return nil, fmt.Errorf("%s: parse: %w", name, err)
	}
	if h.SchemaVersion == nil {
		return nil, fmt.Errorf("%s: %w: missing (want %q)", name, ErrUnknownVersion, SchemaVersion)
	}
	if *h.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("%s: %w %q (want %q)", name, ErrUnknownVersion, *h.SchemaVersion, SchemaVersion)
	}

	f := &File{Path: name}
	switch {
	case h.Ride != nil && h.Park != nil:
		return nil, fmt.Errorf("%s: file has both ride and park sections", name)
	case h.Park != nil:
		f.Kind, f.Park = KindPark, &ParkFile{}
		return f, decodeStrict(name, data, f.Park)
	case h.Ride == nil:
		return nil, fmt.Errorf("%s: no ride or park section", name)
	}

	switch h.Ride.Archetype {
	case ArchetypeMultiTrainCoaster:
		f.Kind, f.Coaster = KindCoaster, &CoasterFile{}
		return f, decodeStrict(name, data, f.Coaster)
	case ArchetypeContinuousLoad:
		f.Kind, f.Continuous = KindContinuous, &ContinuousFile{}
		return f, decodeStrict(name, data, f.Continuous)
	default:
		return nil, fmt.Errorf("%s: unknown ride.archetype %q", name, h.Ride.Archetype)
	}
}

func decodeStrict(name string, data []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("%s: decode: %w", name, err)
	}
	return nil
}

// LoadFile reads and parses one file from disk.
func LoadFile(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return LoadBytes(path, data)
}

// LoadError records a file that could not be loaded at all.
type LoadError struct {
	Path string
	Err  error
}

// Dataset is every file found under a data directory, in lexical path order.
type Dataset struct {
	Files      []*File
	LoadErrors []LoadError
}

// LoadDir loads every *.yaml / *.yml file under dir. A file that fails to load is recorded in
// LoadErrors rather than aborting, so the validate report can show every problem at once.
func LoadDir(dir string) (*Dataset, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		ext := strings.ToLower(filepath.Ext(p))
		if !d.IsDir() && (ext == ".yaml" || ext == ".yml") {
			paths = append(paths, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", dir, err)
	}
	sort.Strings(paths)

	ds := &Dataset{}
	for _, p := range paths {
		f, err := LoadFile(p)
		if err != nil {
			ds.LoadErrors = append(ds.LoadErrors, LoadError{Path: p, Err: err})
			continue
		}
		ds.Files = append(ds.Files, f)
	}
	return ds, nil
}

// Ride returns the loaded ride file with the given id, or nil.
func (d *Dataset) Ride(id string) *File {
	for _, f := range d.Files {
		if f.RideID() == id {
			return f
		}
	}
	return nil
}
