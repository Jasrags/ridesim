// Package osm turns a saved OpenStreetMap (Overpass JSON) extract of a park into a draft park file:
// real-scale node positions and walkway lengths. It is an offline import tool; the simulation never
// reads OSM data. OSM data is © OpenStreetMap contributors, available under the ODbL.
package osm

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

// Element is one OSM node, way or relation as Overpass JSON writes it.
type Element struct {
	Type    string            `json:"type"`
	ID      int64             `json:"id"`
	Lat     float64           `json:"lat"`
	Lon     float64           `json:"lon"`
	Nodes   []int64           `json:"nodes"`
	Members []Member          `json:"members"`
	Tags    map[string]string `json:"tags"`
}

// Member is one member of a relation.
type Member struct {
	Type string `json:"type"`
	Ref  int64  `json:"ref"`
	Role string `json:"role"`
}

// Extract is a parsed Overpass response with lookups by id.
type Extract struct {
	Timestamp string // OSM base timestamp of the extract
	Nodes     map[int64]*Element
	Ways      map[int64]*Element
	Relations map[int64]*Element
	ways      []*Element // file order, for deterministic iteration
}

type overpass struct {
	OSM3S struct {
		Timestamp string `json:"timestamp_osm_base"`
	} `json:"osm3s"`
	Elements []Element `json:"elements"`
}

// Parse reads an Overpass JSON extract.
func Parse(r io.Reader) (*Extract, error) {
	var raw overpass
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("parse overpass json: %w", err)
	}
	x := &Extract{Timestamp: raw.OSM3S.Timestamp, Nodes: map[int64]*Element{}, Ways: map[int64]*Element{}, Relations: map[int64]*Element{}}
	for i := range raw.Elements {
		e := &raw.Elements[i]
		switch e.Type {
		case "node":
			// "out body" and "out skel" can both list a node; keep the one with tags.
			if old, ok := x.Nodes[e.ID]; ok && len(old.Tags) > 0 {
				continue
			}
			x.Nodes[e.ID] = e
		case "way":
			if _, ok := x.Ways[e.ID]; !ok {
				x.ways = append(x.ways, e)
			}
			x.Ways[e.ID] = e
		case "relation":
			x.Relations[e.ID] = e
		}
	}
	return x, nil
}

// WaysInOrder returns the ways in file order.
func (x *Extract) WaysInOrder() []*Element { return x.ways }

// Ref is an element reference written like "n123", "w456" or "r789".
type Ref struct {
	Type byte // 'n', 'w' or 'r'
	ID   int64
}

func (r Ref) String() string { return string(r.Type) + strconv.FormatInt(r.ID, 10) }

// ParseRef parses "n123", "w456" or "r789".
func ParseRef(s string) (Ref, error) {
	if len(s) < 2 || (s[0] != 'n' && s[0] != 'w' && s[0] != 'r') {
		return Ref{}, fmt.Errorf("osm ref %q: want n<id>, w<id> or r<id>", s)
	}
	id, err := strconv.ParseInt(s[1:], 10, 64)
	if err != nil || id <= 0 {
		return Ref{}, fmt.Errorf("osm ref %q: bad id", s)
	}
	return Ref{s[0], id}, nil
}

// Element returns the referenced element, or nil.
func (x *Extract) Element(r Ref) *Element {
	switch r.Type {
	case 'n':
		return x.Nodes[r.ID]
	case 'w':
		return x.Ways[r.ID]
	case 'r':
		return x.Relations[r.ID]
	}
	return nil
}

// Points returns the coordinates of an element's geometry: the node itself, a way's nodes, or the
// nodes of a relation's member ways and nodes (one level). Missing nodes are skipped.
func (x *Extract) Points(e *Element) []LatLon {
	var out []LatLon
	add := func(id int64) {
		if n, ok := x.Nodes[id]; ok {
			out = append(out, LatLon{n.Lat, n.Lon})
		}
	}
	switch e.Type {
	case "node":
		out = append(out, LatLon{e.Lat, e.Lon})
	case "way":
		for _, id := range e.Nodes {
			add(id)
		}
	case "relation":
		for _, m := range e.Members {
			switch m.Type {
			case "node":
				add(m.Ref)
			case "way":
				if w, ok := x.Ways[m.Ref]; ok {
					for _, id := range w.Nodes {
						add(id)
					}
				}
			}
		}
	}
	return out
}
