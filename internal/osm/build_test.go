package osm

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

const lat0, lon0 = 28.357, -81.560

// fixture builds an extract from nodes placed in feet east (x) and south (y) of (lat0, lon0).
type fixture struct {
	x    *Extract
	proj Projection
}

func newFixture() *fixture {
	return &fixture{
		x:    &Extract{Timestamp: "2026-10-18T00:00:00Z", Nodes: map[int64]*Element{}, Ways: map[int64]*Element{}, Relations: map[int64]*Element{}},
		proj: NewProjection(LatLon{lat0, lon0}),
	}
}

func (f *fixture) node(id int64, x, y float64, tags map[string]string) {
	lat := lat0 - y/f.proj.ftPerDegLat
	lon := lon0 + x/f.proj.ftPerDegLon
	f.x.Nodes[id] = &Element{Type: "node", ID: id, Lat: lat, Lon: lon, Tags: tags}
}

func (f *fixture) way(id int64, tags map[string]string, nodes ...int64) {
	w := &Element{Type: "way", ID: id, Nodes: nodes, Tags: tags}
	f.x.Ways[id] = w
	f.x.ways = append(f.x.ways, w)
}

func footway() map[string]string { return map[string]string{"highway": "footway"} }

func testConfig(nodes ...NodeSpec) *Config {
	return &Config{
		Park: ParkSpec{ID: "fx", Name: "Fixture Park", Extract: "fx.json"},
		Walk: WalkSpec{
			Highways: []string{"footway", "pedestrian"}, ExcludeAccess: []string{"private"},
			ExcludeNameContains: []string{"Queue"}, SnapFt: 30, ClusterFt: 25,
			WidthsFt: map[string]float64{"footway": 15, "pedestrian": 40},
		},
		Nodes: nodes,
	}
}

func edgeBetween(t *testing.T, d *Draft, a, b string) DraftEdge {
	t.Helper()
	for _, e := range d.Edges {
		if (e.From == a && e.To == b) || (e.From == b && e.To == a) {
			return e
		}
	}
	t.Fatalf("no edge %s-%s in %+v", a, b, d.Edges)
	return DraftEdge{}
}

func nodeByID(d *Draft, id string) *DraftNode {
	for i := range d.Nodes {
		if d.Nodes[i].ID == id {
			return &d.Nodes[i]
		}
	}
	return nil
}

// pathLen is the shortest walking distance between two draft nodes.
func pathLen(d *Draft, from, to string) float64 {
	distTo := map[string]float64{from: 0}
	done := map[string]bool{}
	for {
		cur, best := "", math.Inf(1)
		for n, v := range distTo {
			if !done[n] && (v < best || (v == best && n < cur)) {
				cur, best = n, v
			}
		}
		if cur == "" {
			return math.Inf(1)
		}
		if cur == to {
			return best
		}
		done[cur] = true
		for _, e := range d.Edges {
			var o string
			switch cur {
			case e.From:
				o = e.To
			case e.To:
				o = e.From
			default:
				continue
			}
			if v, ok := distTo[o]; !ok || best+e.LengthFt < v {
				distTo[o] = best + e.LengthFt
			}
		}
	}
}

func TestProjectionScale(t *testing.T) {
	p := NewProjection(LatLon{lat0, lon0})
	// One degree of latitude at 28.4 N is about 110,870 m; of longitude about 97,990 m.
	north := p.Project(LatLon{lat0 - 0.001, lon0})
	if got, want := north.Y, 110870*0.001/0.3048; math.Abs(got-want) > 1 {
		t.Errorf("0.001 deg lat = %.1f ft, want %.1f", got, want)
	}
	east := p.Project(LatLon{lat0, lon0 + 0.001})
	if got, want := east.X, 97990*0.001/0.3048; math.Abs(got-want) > 2 {
		t.Errorf("0.001 deg lon = %.1f ft, want %.1f", got, want)
	}
}

// A main path with intermediate nodes, a gate at one end, a ride at the other and an unused branch.
func chainFixture() *fixture {
	f := newFixture()
	f.node(1, 0, 0, nil)
	f.node(2, 100, 0, nil)
	f.node(3, 200, 0, nil)
	f.node(4, 300, 0, nil)
	f.node(5, 200, 150, nil) // branch to nowhere
	f.way(10, footway(), 1, 2, 3, 4)
	f.way(11, footway(), 3, 5)
	f.node(90, -20, 0, map[string]string{"entrance": "main"})
	f.node(91, 320, 0, map[string]string{"attraction": "roller_coaster"})
	return f
}

func TestBuildContractsChainsAndPrunesDeadEnds(t *testing.T) {
	f := chainFixture()
	d, err := Build(f.x, testConfig(
		NodeSpec{ID: "gate", Kind: "entrance", OSM: []string{"n90"}},
		NodeSpec{ID: "ride", Kind: "ride", OSM: []string{"n91"}},
	))
	if err != nil {
		t.Fatal(err)
	}
	var junctions int
	for _, n := range d.Nodes {
		if n.Kind == "junction" {
			junctions++
		}
	}
	if junctions != 2 {
		t.Errorf("junctions = %d, want 2 (the two path ends; the unused branch is pruned)", junctions)
	}
	if got := pathLen(d, "gate", "ride"); math.Abs(got-340) > 1 {
		t.Errorf("gate to ride = %.1f ft, want 340 (20 + 300 + 20)", got)
	}
	e := edgeBetween(t, d, "j001", "j002")
	if math.Abs(e.LengthFt-300) > 1 || e.WidthFt != 15 || e.Class != "footway" {
		t.Errorf("contracted edge = %+v, want 300 ft footway 15 ft", e)
	}
}

func TestBuildExcludesBackstageAndQueues(t *testing.T) {
	f := chainFixture()
	f.node(6, 0, 100, nil)
	f.node(7, 300, 100, nil)
	f.way(12, map[string]string{"highway": "footway", "access": "private"}, 6, 7)
	f.node(8, 0, -60, nil)
	f.node(9, 300, -60, nil)
	f.way(13, map[string]string{"highway": "footway", "name": "Ride Standby Queue"}, 8, 9)
	f.node(92, 60, 80, map[string]string{"amenity": "restaurant"}) // closer to the private path
	d, err := Build(f.x, testConfig(
		NodeSpec{ID: "gate", Kind: "entrance", OSM: []string{"n90"}},
		NodeSpec{ID: "food", Kind: "dining", OSM: []string{"n92"}},
	))
	if err != nil {
		t.Fatal(err)
	}
	if n := nodeByID(d, "food"); n == nil || math.Abs(n.AttachFt-80) > 1 {
		t.Fatalf("food attaches %.1f ft away, want 80 (to the public path, not the private one)", n.AttachFt)
	}
}

func TestBuildBridgesShortGaps(t *testing.T) {
	f := chainFixture()
	f.node(20, 100, 12, nil) // stops 12 ft short of the main path
	f.node(21, 100, 200, nil)
	f.way(14, footway(), 20, 21)
	f.node(93, 120, 200, map[string]string{"amenity": "theatre"})
	cfg := testConfig(
		NodeSpec{ID: "gate", Kind: "entrance", OSM: []string{"n90"}},
		NodeSpec{ID: "show", Kind: "show", OSM: []string{"n93"}},
	)
	d, err := Build(f.x, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := pathLen(d, "gate", "show"); math.Abs(got-(20+100+12+188+20)) > 1 {
		t.Errorf("gate to show = %.1f ft, want 340 via the bridge", got)
	}

	cfg.Walk.SnapFt = 0
	d, err = Build(f.x, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := nodeByID(d, "show").AttachFt; got < 50 {
		t.Errorf("without bridging the show should attach to the main path far away, got %.1f ft", got)
	}
	if !strings.Contains(strings.Join(d.Notes, "\n"), "dropped a walkway piece") {
		t.Errorf("notes %q should report the dropped piece", d.Notes)
	}
}

func TestBuildCrossesPlazas(t *testing.T) {
	f := newFixture()
	// A square plaza 200 ft across; paths meet it at the west and east sides.
	f.node(1, 0, 0, nil)
	f.node(2, 200, 0, nil)
	f.node(3, 200, 200, nil)
	f.node(4, 0, 200, nil)
	f.node(5, 0, 100, nil)
	f.node(6, 200, 100, nil)
	f.way(30, map[string]string{"highway": "pedestrian", "area": "yes"}, 1, 2, 6, 3, 4, 5, 1)
	f.node(7, -100, 100, nil)
	f.node(8, 300, 100, nil)
	f.way(31, footway(), 7, 5)
	f.way(32, footway(), 6, 8)
	f.node(94, -110, 100, map[string]string{"entrance": "main"})
	f.node(95, 310, 100, map[string]string{"attraction": "dark_ride"})
	d, err := Build(f.x, testConfig(
		NodeSpec{ID: "gate", Kind: "entrance", OSM: []string{"n94"}},
		NodeSpec{ID: "ride", Kind: "ride", OSM: []string{"n95"}},
	))
	if err != nil {
		t.Fatal(err)
	}
	// Across the plaza: 10 + 100 + 200 + 100 + 10; around its edge would add 200.
	if got := pathLen(d, "gate", "ride"); math.Abs(got-420) > 1 {
		t.Errorf("gate to ride = %.1f ft, want 420 straight across the plaza", got)
	}
}

func TestBuildJoinsPiecesWithoutDeadEnds(t *testing.T) {
	f := newFixture()
	// Two square plazas 15 ft apart: closed outlines have no dead ends to bridge.
	sq := func(base int64, x0 float64) {
		f.node(base, x0, 0, nil)
		f.node(base+1, x0+100, 0, nil)
		f.node(base+2, x0+100, 100, nil)
		f.node(base+3, x0, 100, nil)
		f.way(base, map[string]string{"highway": "pedestrian", "area": "yes"}, base, base+1, base+2, base+3, base)
	}
	sq(100, 0)
	sq(200, 115)
	f.node(98, -10, 50, map[string]string{"entrance": "main"})
	f.node(99, 225, 50, map[string]string{"amenity": "theatre"})
	d, err := Build(f.x, testConfig(
		NodeSpec{ID: "gate", Kind: "entrance", OSM: []string{"n98"}},
		NodeSpec{ID: "show", Kind: "show", OSM: []string{"n99"}},
	))
	if err != nil {
		t.Fatal(err)
	}
	if got := pathLen(d, "gate", "show"); math.IsInf(got, 1) || got > 400 {
		t.Errorf("gate to show = %.1f ft; the plazas should be joined across the 15 ft gap", got)
	}
	if strings.Contains(strings.Join(d.Notes, "\n"), "dropped") {
		t.Errorf("nothing should be dropped: %q", d.Notes)
	}
}

func TestBuildExcludesPathsInsideOutlines(t *testing.T) {
	f := chainFixture()
	// A theatre outline south of the path with aisles inside it.
	f.node(40, 120, 20, nil)
	f.node(41, 280, 20, nil)
	f.node(42, 280, 120, nil)
	f.node(43, 120, 120, nil)
	f.way(40, map[string]string{"amenity": "theatre"}, 40, 41, 42, 43, 40)
	f.node(44, 150, 50, nil)
	f.node(45, 250, 50, nil)
	f.node(46, 200, 100, nil)
	f.way(41, footway(), 44, 45)
	f.way(42, footway(), 44, 46, 45)
	cfg := testConfig(
		NodeSpec{ID: "gate", Kind: "entrance", OSM: []string{"n90"}},
		NodeSpec{ID: "show", Kind: "show", OSM: []string{"w40"}},
	)
	cfg.Walk.ExcludeInside = []string{"w40"}
	d, err := Build(f.x, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if n := nodeByID(d, "show"); n == nil || n.AttachFt > 21 {
		t.Fatalf("show should attach from its outline to the main path 20 ft away, got %+v", n)
	}
	for _, n := range d.Nodes {
		if n.Kind == "junction" && n.P.Y > 30 {
			t.Errorf("junction %s at %+v is inside the theatre", n.ID, n.P)
		}
	}
}

func TestBuildClustersNearbyJunctions(t *testing.T) {
	f := chainFixture()
	f.node(22, 110, 0, nil) // a second branch 10 ft from the first junction
	f.node(23, 110, -150, nil)
	f.way(10, footway(), 1, 2, 22, 3, 4)
	f.way(15, footway(), 22, 23)
	f.node(96, 110, -170, map[string]string{"shop": "gift"})
	f.node(97, 100, 170, map[string]string{"shop": "gift"})
	f.node(24, 100, 150, nil)
	f.way(16, footway(), 2, 24)
	d, err := Build(f.x, testConfig(
		NodeSpec{ID: "gate", Kind: "entrance", OSM: []string{"n90"}},
		NodeSpec{ID: "north", Kind: "shop", OSM: []string{"n96"}},
		NodeSpec{ID: "south", Kind: "shop", OSM: []string{"n97"}},
	))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range d.Edges {
		if e.LengthFt < 25 && e.Class != "spur" {
			t.Errorf("edge %s is %.1f ft; junctions closer than 25 ft should merge", e.ID, e.LengthFt)
		}
	}
}

func TestDraftIsDeterministic(t *testing.T) {
	render := func() string {
		f := chainFixture()
		d, err := Build(f.x, testConfig(
			NodeSpec{ID: "gate", Kind: "entrance", Name: "Main Entrance", OSM: []string{"n90"}},
			NodeSpec{ID: "ride", Kind: "ride", Land: "sunset", MapNo: []int{57}, OSM: []string{"n91"}},
		))
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		if err := d.WriteYAML(&b, "fx.json"); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	a := render()
	for i := 0; i < 5; i++ {
		if b := render(); b != a {
			t.Fatalf("draft differs between runs:\n%s\n---\n%s", a, b)
		}
	}
	for _, want := range []string{"© OpenStreetMap contributors", "ODbL", "[osm] n91 · sunset · map 57", "width [assumed] footway", "kind: junction"} {
		if !strings.Contains(a, want) {
			t.Errorf("draft lacks %q:\n%s", want, a)
		}
	}
}

func TestParseRef(t *testing.T) {
	for _, s := range []string{"n1", "w357021885", "r3919136"} {
		if r, err := ParseRef(s); err != nil || r.String() != s {
			t.Errorf("ParseRef(%q) = %v, %v", s, r, err)
		}
	}
	for _, s := range []string{"", "x1", "w", "w-3", "wabc"} {
		if _, err := ParseRef(s); err == nil {
			t.Errorf("ParseRef(%q) accepted", s)
		}
	}
}

// Overpass can list a way twice (out skel, then out body); the tagged copy must win in both lookups.
func TestParseKeepsTaggedDuplicateWay(t *testing.T) {
	js := `{"elements":[
		{"type":"way","id":7,"nodes":[1,2]},
		{"type":"way","id":7,"nodes":[1,2],"tags":{"highway":"footway"}}]}`
	x, err := Parse(strings.NewReader(js))
	if err != nil {
		t.Fatal(err)
	}
	if ws := x.WaysInOrder(); len(ws) != 1 || ws[0].Tags["highway"] != "footway" || x.Ways[7] != ws[0] {
		t.Fatalf("ways in order %+v, by id %+v; want the one tagged copy in both", ws, x.Ways[7])
	}
}

func TestExcludeInsideRefsAreChecked(t *testing.T) {
	cfg := testConfig(NodeSpec{ID: "gate", Kind: "entrance", OSM: []string{"n90"}})
	cfg.Walk.ExcludeInside = []string{"x40"}
	if err := cfg.check(); err == nil {
		t.Error("a malformed exclude_inside ref passed the config check")
	}
	cfg.Walk.ExcludeInside = []string{"w404"}
	if _, err := Build(chainFixture().x, cfg); err == nil {
		t.Error("an exclude_inside outline missing from the extract was ignored")
	}
}
