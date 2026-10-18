package osm

import (
	"bytes"
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// Config is the hand-edited mapping file: which OSM ways are guest walkways, and which OSM elements
// become park nodes under which ids. It names the extract it reads (relative to the config file).
type Config struct {
	Park  ParkSpec   `yaml:"park"`
	Walk  WalkSpec   `yaml:"walk"`
	Nodes []NodeSpec `yaml:"nodes"`
	Links []LinkSpec `yaml:"links"`
}

// LinkSpec joins an OSM walkway node to the nearest walkway of another piece, whatever the distance:
// a hand-reviewed fix for a path OSM is missing. The straight link's length is an assumption.
type LinkSpec struct {
	From string `yaml:"from"` // OSM node ref
	Note string `yaml:"note"` // why the link exists
}

// ParkSpec identifies the park and the extract.
type ParkSpec struct {
	ID       string `yaml:"id"`
	Name     string `yaml:"name"`
	Extract  string `yaml:"extract"`
	Boundary string `yaml:"boundary"` // OSM ref of the park outline; its north-west corner is the origin
}

// WalkSpec selects guest walkways and sets the graph-building tolerances.
type WalkSpec struct {
	Highways            []string           `yaml:"highways"`              // highway=* values that are walkways
	ExcludeAccess       []string           `yaml:"exclude_access"`        // access=* values that are backstage
	ExcludeNameContains []string           `yaml:"exclude_name_contains"` // e.g. queue lines
	SnapFt              float64            `yaml:"snap_ft"`               // bridge a dead end to a path this close
	ClusterFt           float64            `yaml:"cluster_ft"`            // merge junctions joined by a shorter edge
	WidthsFt            map[string]float64 `yaml:"widths_ft"`             // default width per highway value [assumed]
	ExcludeInside       []string           `yaml:"exclude_inside"`        // outlines whose inner paths are dropped (seating aisles)
}

// NodeSpec maps OSM elements to one park node. A single element attaches at its point nearest the
// walkway network; several elements (a land's shops) attach at their centroid. At overrides the
// attach position with a specific OSM node.
type NodeSpec struct {
	ID    string   `yaml:"id"`
	Kind  string   `yaml:"kind"`
	Name  string   `yaml:"name"`
	Land  string   `yaml:"land"`
	MapNo []int    `yaml:"map_no"` // guide-map numbers, for review
	OSM   []string `yaml:"osm"`
	At    string   `yaml:"at"`
}

// LoadConfig reads and checks a mapping file.
func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := c.check(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

func (c *Config) check() error {
	if c.Park.ID == "" || c.Park.Extract == "" {
		return fmt.Errorf("park.id and park.extract are required")
	}
	if len(c.Walk.Highways) == 0 {
		return fmt.Errorf("walk.highways is empty")
	}
	if c.Walk.SnapFt < 0 || c.Walk.ClusterFt < 0 {
		return fmt.Errorf("walk.snap_ft and walk.cluster_ft must be >= 0")
	}
	for _, h := range c.Walk.Highways {
		if c.Walk.WidthsFt[h] <= 0 {
			return fmt.Errorf("walk.widths_ft has no positive width for highway %q", h)
		}
	}
	seen := map[string]bool{}
	for _, n := range c.Nodes {
		if n.ID == "" || n.Kind == "" {
			return fmt.Errorf("node %q: id and kind are required", n.ID)
		}
		if seen[n.ID] {
			return fmt.Errorf("duplicate node id %q", n.ID)
		}
		seen[n.ID] = true
		if len(n.OSM) == 0 && n.At == "" {
			return fmt.Errorf("node %q: needs osm refs or at", n.ID)
		}
		for _, r := range append(append([]string{}, n.OSM...), n.At) {
			if r == "" {
				continue
			}
			if _, err := ParseRef(r); err != nil {
				return fmt.Errorf("node %q: %w", n.ID, err)
			}
		}
	}
	for _, l := range c.Links {
		r, err := ParseRef(l.From)
		if err != nil || r.Type != 'n' {
			return fmt.Errorf("link from %q: want an OSM node ref", l.From)
		}
		if l.Note == "" {
			return fmt.Errorf("link from %s: note is required", l.From)
		}
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
