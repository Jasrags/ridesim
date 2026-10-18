package config

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultStudySeeds is the replications per variant when a study gives none (decision 2026-10-17).
const DefaultStudySeeds = 20

// StudyFile is an A/B comparison: a base scenario and variants that override it. Every variant runs on
// the same seeds as the base (common random numbers), so differences are paired.
type StudyFile struct {
	SchemaVersion string `yaml:"schema_version"`
	Study         Study  `yaml:"study"`
}

type Study struct {
	ID          string    `yaml:"id"`
	Description string    `yaml:"description"`
	Base        string    `yaml:"base"` // scenario id
	Seeds       int       `yaml:"seeds"`
	ReportNode  string    `yaml:"report_node"` // park studies: the node whose metrics are compared
	Variants    []Variant `yaml:"variants"`
}

// Variant changes the base. Set is deep-merged into the scenario section, RideSet into the ride file
// (in memory only; data files are never rewritten), and Scale multiplies arrival rates.
type Variant struct {
	ID      string             `yaml:"id"`
	Note    string             `yaml:"note"`
	Set     yaml.Node          `yaml:"set"`
	RideSet yaml.Node          `yaml:"ride_set"`
	Scale   map[string]float64 `yaml:"scale"` // keys: arrivals, lightning_lane
}

// Scale keys.
const (
	ScaleArrivals      = "arrivals"
	ScaleLightningLane = "lightning_lane"
)

var studyChecks = []check[*StudyFile]{
	{"study.basics", SevError, checkStudyBasics},
}

func checkStudyBasics(f *StudyFile) (bool, string) {
	s := f.Study
	var bad []string
	if s.ID == "" || s.Base == "" {
		bad = append(bad, "study needs id and base")
	}
	if s.Seeds < 0 {
		bad = append(bad, "seeds must be >= 0 (0 = default)")
	}
	ids := map[string]bool{}
	for _, v := range s.Variants {
		if v.ID == "" || ids[v.ID] || v.ID == "base" {
			bad = append(bad, fmt.Sprintf("variant id %q must be unique, non-empty and not \"base\"", v.ID))
		}
		ids[v.ID] = true
		for k, x := range v.Scale {
			if (k != ScaleArrivals && k != ScaleLightningLane) || x <= 0 {
				bad = append(bad, fmt.Sprintf("variant %s: scale %s=%g (keys arrivals, lightning_lane; > 0)", v.ID, k, x))
			}
		}
		for name, n := range map[string]*yaml.Node{"set": &v.Set, "ride_set": &v.RideSet} {
			if n.Kind != 0 && n.Kind != yaml.MappingNode {
				bad = append(bad, fmt.Sprintf("variant %s: %s must be a mapping", v.ID, name))
			}
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return false, strings.Join(bad, "; ")
	}
	seeds := s.Seeds
	if seeds == 0 {
		seeds = DefaultStudySeeds
	}
	return true, fmt.Sprintf("base %s, %d variant(s), %d seeds", s.Base, len(s.Variants), seeds)
}

// Scenario returns the loaded scenario file with the given id, or nil.
func (d *Dataset) Scenario(id string) *File {
	for _, f := range d.Files {
		if f.Kind == KindScenario && f.Scenario.Scenario.ID == id {
			return f
		}
	}
	return nil
}

// ResolvedVariant is a study variant turned into concrete files.
type ResolvedVariant struct {
	ID       string
	Scenario *ScenarioFile
	Ride     *File // nil = the dataset's ride unchanged
	Scale    map[string]float64
}

// Resolve builds the base and every variant. Overrides are deep-merged (mappings merge key by key,
// anything else replaces) and the result is strictly decoded and checked like a data file.
func (s *Study) Resolve(ds *Dataset) ([]ResolvedVariant, error) {
	base := ds.Scenario(s.Base)
	if base == nil {
		return nil, fmt.Errorf("study %s: base scenario %s not found", s.ID, s.Base)
	}
	if base.Scenario.Scenario.IsPark() {
		return s.resolvePark(ds, base)
	}
	if s.ReportNode != "" {
		return nil, fmt.Errorf("study %s: report_node is for park studies", s.ID)
	}
	rideFile := ds.Ride(base.Scenario.Scenario.RideID)
	if rideFile == nil {
		return nil, fmt.Errorf("study %s: ride %s not found", s.ID, base.Scenario.Scenario.RideID)
	}
	out := []ResolvedVariant{{ID: "base", Scenario: base.Scenario}}
	for _, v := range s.Variants {
		sc, err := mergeInto(base.Scenario, "scenario", &v.Set, "study "+s.ID+" variant "+v.ID)
		if err != nil {
			return nil, err
		}
		rv := ResolvedVariant{ID: v.ID, Scenario: sc.Scenario, Scale: v.Scale}
		if v.RideSet.Kind != 0 {
			var rideStruct any = rideFile.Coaster
			if rideFile.Kind == KindContinuous {
				rideStruct = rideFile.Continuous
			}
			merged, err := mergeInto(rideStruct, "", &v.RideSet, "study "+s.ID+" variant "+v.ID+" ride_set")
			if err != nil {
				return nil, err
			}
			rv.Ride = merged.File
		}
		if err := checkResolved(ds, rideFile, rv); err != nil {
			return nil, fmt.Errorf("study %s variant %s: %w", s.ID, v.ID, err)
		}
		out = append(out, rv)
	}
	return out, nil
}

type merged struct {
	Scenario *ScenarioFile
	File     *File
}

// mergeInto marshals v, deep-merges over into the mapping at key ("" = document root) and re-decodes
// strictly into the same kind of file.
func mergeInto(v any, key string, over *yaml.Node, name string) (merged, error) {
	raw, err := yaml.Marshal(v)
	if err != nil {
		return merged{}, fmt.Errorf("%s: %w", name, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return merged{}, fmt.Errorf("%s: %w", name, err)
	}
	target := doc.Content[0]
	if key != "" {
		target = mapValue(target, key)
		if target == nil {
			return merged{}, fmt.Errorf("%s: no %s section", name, key)
		}
	}
	if over.Kind != 0 {
		deepMerge(target, over)
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return merged{}, fmt.Errorf("%s: %w", name, err)
	}
	f, err := LoadBytes(name+".yaml", out)
	if err != nil {
		return merged{}, fmt.Errorf("%s: %w", name, err)
	}
	return merged{Scenario: f.Scenario, File: f}, nil
}

func mapValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// deepMerge writes over into dst: mappings merge key by key, anything else replaces the value.
func deepMerge(dst, over *yaml.Node) {
	for i := 0; i+1 < len(over.Content); i += 2 {
		k, v := over.Content[i], over.Content[i+1]
		cur := mapValue(dst, k.Value)
		switch {
		case cur == nil:
			dst.Content = append(dst.Content, k, v)
		case cur.Kind == yaml.MappingNode && v.Kind == yaml.MappingNode:
			deepMerge(cur, v)
		default:
			*cur = *v
		}
	}
}

// checkResolved runs the data checks on a variant's scenario and ride; any failing error check is fatal.
func checkResolved(ds *Dataset, rideFile *File, rv ResolvedVariant) error {
	ride := rideFile
	if rv.Ride != nil {
		ride = rv.Ride
		ride.Path = rideFile.Path // keep the filename convention check meaningful
	}
	files := []*File{ride, {Path: "variant.yaml", Kind: KindScenario, Scenario: rv.Scenario}}
	for _, f := range ds.Files {
		if f.Kind == KindPark {
			files = append(files, f)
		}
	}
	var bad bytes.Buffer
	for _, fr := range Validate(&Dataset{Files: files}).Files {
		for _, r := range fr.Results {
			if r.Status == StatusFail && r.CheckID != "dataset.unique_ride_ids" && r.CheckID != "park.ride_refs" {
				fmt.Fprintf(&bad, "%s: %s; ", r.CheckID, r.Detail)
			}
		}
	}
	if bad.Len() > 0 {
		return fmt.Errorf("variant fails checks: %s", strings.TrimSuffix(bad.String(), "; "))
	}
	return nil
}

// resolvePark resolves a study on a park scenario: scenario overrides only (ride_set would change a ride
// inside the park and is not supported yet), and report_node must name an activity node of the park.
func (s *Study) resolvePark(ds *Dataset, base *File) ([]ResolvedVariant, error) {
	pf := ds.Park(base.Scenario.Scenario.ParkID)
	if pf == nil {
		return nil, fmt.Errorf("study %s: park %s not found", s.ID, base.Scenario.Scenario.ParkID)
	}
	found := false
	for _, n := range pf.Nodes {
		if n.ID == s.ReportNode && n.Kind != NodeEntrance && n.Kind != NodeExit && n.Kind != NodePlaza && n.Kind != NodeJunction {
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("study %s: report_node %q must be an activity node of park %s", s.ID, s.ReportNode, pf.Park.ID)
	}
	out := []ResolvedVariant{{ID: "base", Scenario: base.Scenario}}
	for _, v := range s.Variants {
		if v.RideSet.Kind != 0 {
			return nil, fmt.Errorf("study %s variant %s: ride_set is not supported on park studies", s.ID, v.ID)
		}
		sc, err := mergeInto(base.Scenario, "scenario", &v.Set, "study "+s.ID+" variant "+v.ID)
		if err != nil {
			return nil, err
		}
		rv := ResolvedVariant{ID: v.ID, Scenario: sc.Scenario, Scale: v.Scale}
		files := []*File{{Path: "variant.yaml", Kind: KindScenario, Scenario: rv.Scenario}}
		for _, f := range ds.Files {
			if f.Kind == KindPark || f.Kind == KindCoaster || f.Kind == KindContinuous {
				files = append(files, f)
			}
		}
		var bad []string
		for _, r := range Validate(&Dataset{Files: files}).FileReport("variant.yaml").Results {
			if r.Status == StatusFail {
				bad = append(bad, r.CheckID+": "+r.Detail)
			}
		}
		if len(bad) > 0 {
			return nil, fmt.Errorf("study %s variant %s: variant fails checks: %s", s.ID, v.ID, strings.Join(bad, "; "))
		}
		out = append(out, rv)
	}
	return out, nil
}
