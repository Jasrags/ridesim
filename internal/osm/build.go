package osm

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

const (
	splitMinFt = 3  // attach to an existing node instead of splitting a segment this close to it
	minSpurFt  = 10 // shortest link from an attraction to its walkway
)

// Draft is the imported park: nodes with projected positions and contracted walkway edges.
type Draft struct {
	ParkID    string
	Name      string
	Timestamp string
	Origin    LatLon
	Nodes     []DraftNode
	Edges     []DraftEdge
	Notes     []string // import report: dropped pieces, long attach links
	Dropped   [][2]Pt  // segments of walkway pieces not connected to the main network (review map only)
}

// DraftNode is one park node. Kind "junction" is a walkway junction the importer found.
type DraftNode struct {
	ID, Kind, Name, Land string
	MapNo                []int
	OSM                  []string
	P                    Pt
	AttachFt             float64 // distance from the element to the walkway it joins
}

// DraftEdge is one walkway: length from OSM geometry, width from the default for its highway type.
type DraftEdge struct {
	ID, From, To string
	LengthFt     float64
	WidthFt      float64
	Class        string // highway value the width came from; "spur" for attraction links
}

// Build imports the extract according to the config.
func Build(x *Extract, c *Config) (*Draft, error) {
	proj, err := projection(x, c)
	if err != nil {
		return nil, err
	}
	d := &Draft{ParkID: c.Park.ID, Name: c.Park.Name, Timestamp: x.Timestamp, Origin: proj.Origin}
	g := newGraph()
	plazas, err := addWalkways(g, x, c, proj)
	if err != nil {
		return nil, err
	}
	d.Notes = append(d.Notes, bridgeDeadEnds(g, c.Walk.SnapFt)...)
	plazaCentres := addPlazas(g, plazas, c)
	d.Notes = append(d.Notes, joinPieces(g, c.Walk.SnapFt)...)
	linkNotes, err := addLinks(g, c.Links)
	if err != nil {
		return nil, err
	}
	d.Notes = append(d.Notes, linkNotes...)
	comps := g.components()
	if len(comps) == 0 {
		return nil, fmt.Errorf("no walkways selected")
	}
	inMain := map[int64]bool{}
	for _, id := range comps[0] {
		inMain[id] = true
	}
	for _, comp := range comps[1:] {
		gap, from, to := math.Inf(1), int64(0), Pt{}
		for _, id := range comp {
			if _, at, _, dd, ok := g.nearestSegment(g.pos[id], func(s segRef) bool { return inMain[s.a] }); ok && dd < gap {
				gap, from, to = dd, id, at
			}
		}
		d.Notes = append(d.Notes, fmt.Sprintf("dropped a walkway piece of %d nodes, %.0f ft from the main network (node %d at %s,%s toward %s,%s)",
			len(comp), gap, from, ft(g.pos[from].X), ft(g.pos[from].Y), ft(to.X), ft(to.Y)))
		for _, id := range comp {
			for _, n := range g.neighbors(id) {
				if id < n {
					d.Dropped = append(d.Dropped, [2]Pt{g.pos[id], g.pos[n]})
				}
			}
		}
		for _, id := range comp {
			g.removeNode(id)
		}
	}

	keep := map[int64]bool{}
	for id := range plazaCentres {
		keep[id] = true
	}
	attached, notes, err := attach(g, x, c, proj, keep)
	if err != nil {
		return nil, err
	}
	d.Notes = append(d.Notes, notes...)
	g.prune(keep)
	edges, root := cluster(g, g.contract(keep), keep, c.Walk.ClusterFt)
	for i := range attached {
		attached[i].join = root(attached[i].join)
	}
	name := nameJunctions(g, edges, attached)

	for _, a := range attached {
		d.Nodes = append(d.Nodes, a.node)
		d.Edges = append(d.Edges, DraftEdge{From: a.node.ID, To: name[a.join], LengthFt: math.Max(a.node.AttachFt, minSpurFt), WidthFt: a.width, Class: "spur"})
	}
	for _, id := range sortedJunctions(name, attached) {
		d.Nodes = append(d.Nodes, DraftNode{ID: name[id], Kind: "junction", P: g.pos[id]})
	}
	for _, e := range edges {
		d.Edges = append(d.Edges, DraftEdge{From: name[e.a], To: name[e.b], LengthFt: e.length, WidthFt: e.width, Class: e.class})
	}
	for i := range d.Edges {
		e := &d.Edges[i]
		if e.From > e.To && e.Class != "spur" {
			e.From, e.To = e.To, e.From
		}
		e.ID = e.From + "_" + e.To
	}
	sort.Slice(d.Edges, func(i, j int) bool { return d.Edges[i].ID < d.Edges[j].ID })
	return d, nil
}

// projection puts the origin at the north-west corner of the park outline (or of all nodes).
func projection(x *Extract, c *Config) (Projection, error) {
	var pts []LatLon
	if c.Park.Boundary != "" {
		r, err := ParseRef(c.Park.Boundary)
		if err != nil {
			return Projection{}, err
		}
		e := x.Element(r)
		if e == nil {
			return Projection{}, fmt.Errorf("boundary %s not in extract", r)
		}
		pts = x.Points(e)
	} else {
		for _, n := range x.Nodes {
			pts = append(pts, LatLon{n.Lat, n.Lon})
		}
	}
	if len(pts) == 0 {
		return Projection{}, fmt.Errorf("no points to place the origin")
	}
	nw := LatLon{-90, 180}
	for _, p := range pts {
		nw.Lat = math.Max(nw.Lat, p.Lat)
		nw.Lon = math.Min(nw.Lon, p.Lon)
	}
	return NewProjection(nw), nil
}

func (c *Config) isWalkway(t map[string]string) bool {
	h := t["highway"]
	ok := false
	for _, w := range c.Walk.Highways {
		ok = ok || h == w
	}
	for _, a := range c.Walk.ExcludeAccess {
		ok = ok && t["access"] != a
	}
	for _, s := range c.Walk.ExcludeNameContains {
		ok = ok && !strings.Contains(t["name"], s)
	}
	return ok
}

// addWalkways adds every selected way and returns the closed pedestrian areas (plazas).
func addWalkways(g *graph, x *Extract, c *Config, proj Projection) ([]*Element, error) {
	var plazas []*Element
	var outlines [][]Pt
	for _, ref := range c.Walk.ExcludeInside {
		r, err := ParseRef(ref)
		if err != nil {
			return nil, fmt.Errorf("walk.exclude_inside: %w", err)
		}
		e := x.Element(r)
		if e == nil {
			return nil, fmt.Errorf("walk.exclude_inside: %s not in extract", r)
		}
		outlines = append(outlines, projectAll(proj, x.Points(e)))
	}
	for _, w := range x.WaysInOrder() {
		if !c.isWalkway(w.Tags) || insideAny(x, proj, w, outlines) {
			continue
		}
		class := w.Tags["highway"]
		width := c.Walk.WidthsFt[class]
		for i, id := range w.Nodes {
			n, ok := x.Nodes[id]
			if !ok {
				continue
			}
			g.addNode(id, proj.Project(LatLon{n.Lat, n.Lon}))
			if i > 0 {
				if _, ok := g.pos[w.Nodes[i-1]]; ok {
					g.link(w.Nodes[i-1], id, seg{dist(g.pos[w.Nodes[i-1]], g.pos[id]), width, class, w.ID})
				}
			}
		}
		if w.Tags["area"] == "yes" && len(w.Nodes) > 3 && w.Nodes[0] == w.Nodes[len(w.Nodes)-1] {
			plazas = append(plazas, w)
		}
	}
	return plazas, nil
}

// insideAny reports whether every node of the way lies inside one of the outlines.
func insideAny(x *Extract, proj Projection, w *Element, outlines [][]Pt) bool {
	for _, poly := range outlines {
		all := true
		for _, id := range w.Nodes {
			n, ok := x.Nodes[id]
			if ok && !inPolygon(proj.Project(LatLon{n.Lat, n.Lon}), poly) {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

// inPolygon is the even-odd ray test; points on the outline may fall either way.
func inPolygon(p Pt, poly []Pt) bool {
	in := false
	for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
		a, b := poly[i], poly[j]
		if (a.Y > p.Y) != (b.Y > p.Y) && p.X < (b.X-a.X)*(p.Y-a.Y)/(b.Y-a.Y)+a.X {
			in = !in
		}
	}
	return in
}

// bridgeDeadEnds links each dead end to the nearest other way within snapFt: OSM paths often stop a
// few feet short of the path or plaza they meet.
func bridgeDeadEnds(g *graph, snapFt float64) []string {
	if snapFt <= 0 {
		return nil
	}
	var notes []string
	bridged := 0
	for _, id := range g.ids() {
		if len(g.adj[id]) != 1 {
			continue
		}
		var own int64
		for _, s := range g.adj[id] {
			own = s.way
		}
		s, at, t, d, ok := g.nearestSegment(g.pos[id], func(s segRef) bool {
			return s.a != id && s.b != id && g.adj[s.a][s.b].way != own
		})
		if !ok || d > snapFt {
			continue
		}
		whole := g.adj[s.a][s.b]
		n := g.splitAt(s, at, t, splitMinFt)
		g.link(id, n, seg{math.Max(dist(g.pos[id], g.pos[n]), 1), whole.width, whole.class, 0})
		bridged++
	}
	notes = append(notes, fmt.Sprintf("bridged %d dead ends within %.0f ft of another path", bridged, snapFt))
	return notes
}

// joinPieces links each disconnected walkway piece to the nearest other piece within snapFt, from
// any of its nodes. Plaza outlines and closed loops have no dead ends, so bridgeDeadEnds misses gaps
// between them (the Fantasmic amphitheater is mapped that way).
func joinPieces(g *graph, snapFt float64) []string {
	if snapFt <= 0 {
		return nil
	}
	joined := 0
	for {
		comps := g.components()
		in := map[int64]int{}
		for i, comp := range comps {
			for _, id := range comp {
				in[id] = i
			}
		}
		merged := false
		for i := len(comps) - 1; i >= 1 && !merged; i-- {
			var best struct {
				from int64
				s    segRef
				at   Pt
				t, d float64
			}
			best.d = math.Inf(1)
			for _, id := range comps[i] {
				s, at, t, d, ok := g.nearestSegment(g.pos[id], func(s segRef) bool { return in[s.a] != i })
				if ok && d < best.d {
					best.from, best.s, best.at, best.t, best.d = id, s, at, t, d
				}
			}
			if best.d > snapFt {
				continue
			}
			whole := g.adj[best.s.a][best.s.b]
			n := g.splitAt(best.s, best.at, best.t, splitMinFt)
			g.link(best.from, n, seg{math.Max(best.d, 1), whole.width, whole.class, 0})
			joined++
			merged = true
		}
		if !merged {
			break
		}
	}
	return []string{fmt.Sprintf("joined %d separate walkway pieces within %.0f ft of the network", joined, snapFt)}
}

// addLinks applies the hand-reviewed links: each OSM node joins the nearest walkway outside its own
// piece with a straight segment.
func addLinks(g *graph, links []LinkSpec) ([]string, error) {
	var notes []string
	for _, l := range links {
		r, _ := ParseRef(l.From)
		if _, ok := g.pos[r.ID]; !ok {
			return nil, fmt.Errorf("link from %s: not a walkway node", l.From)
		}
		in := map[int64]bool{}
		for _, comp := range g.components() {
			for _, id := range comp {
				if id == r.ID {
					for _, m := range comp {
						in[m] = true
					}
				}
			}
		}
		s, at, t, d, ok := g.nearestSegment(g.pos[r.ID], func(s segRef) bool { return !in[s.a] })
		if !ok {
			return nil, fmt.Errorf("link from %s: no other walkway piece", l.From)
		}
		whole := g.adj[s.a][s.b]
		n := g.splitAt(s, at, t, splitMinFt)
		g.link(r.ID, n, seg{math.Max(d, 1), whole.width, whole.class, 0})
		notes = append(notes, fmt.Sprintf("linked %s to the network with a straight %.0f ft walkway [assumed]: %s", l.From, d, l.Note))
	}
	return notes, nil
}

// addPlazas gives each plaza a centre node linked straight to every point where another path meets
// its outline, so guests cross a plaza instead of walking around its edge.
func addPlazas(g *graph, plazas []*Element, c *Config) map[int64]bool {
	centres := map[int64]bool{}
	for _, w := range plazas {
		ring := w.Nodes[:len(w.Nodes)-1]
		var pts []Pt
		var portals []int64
		for _, id := range ring {
			p, ok := g.pos[id]
			if !ok {
				continue
			}
			pts = append(pts, p)
			for n, s := range g.adj[id] {
				_ = n
				if s.way != w.ID {
					portals = append(portals, id)
					break
				}
			}
		}
		if len(portals) < 2 {
			continue
		}
		sort.Slice(portals, func(i, j int) bool { return portals[i] < portals[j] })
		ctr := g.newNode(centroid(pts))
		centres[ctr] = true
		width := c.Walk.WidthsFt[w.Tags["highway"]]
		for _, p := range portals {
			g.link(ctr, p, seg{dist(g.pos[ctr], g.pos[p]), width, w.Tags["highway"], 0})
		}
	}
	return centres
}

type attachment struct {
	node  DraftNode
	join  int64   // walkway node the spur joins
	width float64 // width of the walkway it joins
}

// attach places every configured node and splits the walkway where its spur joins.
func attach(g *graph, x *Extract, c *Config, proj Projection, keep map[int64]bool) ([]attachment, []string, error) {
	var out []attachment
	var notes []string
	for _, spec := range c.Nodes {
		var pts []Pt
		var centres []Pt
		for _, s := range spec.OSM {
			r, _ := ParseRef(s)
			e := x.Element(r)
			if e == nil {
				return nil, nil, fmt.Errorf("node %s: %s not in extract", spec.ID, r)
			}
			ps := projectAll(proj, x.Points(e))
			if len(ps) == 0 {
				return nil, nil, fmt.Errorf("node %s: %s has no geometry", spec.ID, r)
			}
			pts = append(pts, ps...)
			centres = append(centres, centroid(ps))
		}
		var cands []Pt
		switch {
		case spec.At != "":
			r, _ := ParseRef(spec.At)
			e := x.Element(r)
			if e == nil || e.Type != "node" {
				return nil, nil, fmt.Errorf("node %s: at %s is not a node in the extract", spec.ID, r)
			}
			cands = []Pt{proj.Project(LatLon{e.Lat, e.Lon})}
		case len(spec.OSM) > 1:
			cands = []Pt{centroid(centres)}
		default:
			cands = pts
		}
		best := struct {
			p, at Pt
			s     segRef
			t, d  float64
		}{d: math.Inf(1)}
		for _, p := range cands {
			if s, at, t, d, ok := g.nearestSegment(p, nil); ok && d < best.d {
				best.p, best.at, best.s, best.t, best.d = p, at, s, t, d
			}
		}
		if math.IsInf(best.d, 1) {
			return nil, nil, fmt.Errorf("node %s: no walkway to attach to", spec.ID)
		}
		width := g.adj[best.s.a][best.s.b].width
		join := g.splitAt(best.s, best.at, best.t, splitMinFt)
		keep[join] = true
		if best.d > 150 {
			notes = append(notes, fmt.Sprintf("node %s is %.0f ft from the nearest walkway; check its osm refs or set at", spec.ID, best.d))
		}
		out = append(out, attachment{
			node:  DraftNode{ID: spec.ID, Kind: spec.Kind, Name: spec.Name, Land: spec.Land, MapNo: spec.MapNo, OSM: spec.OSM, P: best.p, AttachFt: best.d},
			join:  join,
			width: width,
		})
	}
	return out, notes, nil
}

func projectAll(proj Projection, ls []LatLon) []Pt {
	out := make([]Pt, len(ls))
	for i, l := range ls {
		out[i] = proj.Project(l)
	}
	return out
}

// cluster merges kept nodes joined by an edge shorter than clusterFt into one node at their mean
// position, so a messy crossing becomes one junction. It also returns the node-to-root mapping.
func cluster(g *graph, es []edge, keep map[int64]bool, clusterFt float64) ([]edge, func(int64) int64) {
	parent := map[int64]int64{}
	var find func(int64) int64
	find = func(a int64) int64 {
		if p, ok := parent[a]; ok && p != a {
			r := find(p)
			parent[a] = r
			return r
		}
		return a
	}
	for _, e := range es {
		if e.length < clusterFt {
			ra, rb := find(e.a), find(e.b)
			if ra != rb {
				if rb < ra {
					ra, rb = rb, ra
				}
				parent[rb] = ra
			}
		}
	}
	members := map[int64][]Pt{}
	for _, e := range es {
		for _, id := range []int64{e.a, e.b} {
			members[find(id)] = append(members[find(id)], g.pos[id])
		}
	}
	for r := range members {
		// A node listed once per incident edge weighs junction ends equally enough for a draft.
		if find(r) == r && len(members[r]) > 0 {
			g.pos[r] = centroid(members[r])
		}
	}
	var out []edge
	for _, e := range es {
		ra, rb := find(e.a), find(e.b)
		if keep[e.a] {
			keep[ra] = true
		}
		if keep[e.b] {
			keep[rb] = true
		}
		out = append(out, edge{ra, rb, e.length, e.width, e.class})
	}
	// Attachments that were merged follow their root.
	for id := range parent {
		if r := find(id); r != id {
			g.pos[id] = g.pos[r]
		}
	}
	return dedupe(out), find
}

// nameJunctions names cluster roots j001, j002, ... ordered north to south, then west to east.
func nameJunctions(g *graph, es []edge, att []attachment) map[int64]string {
	ids := map[int64]bool{}
	for _, e := range es {
		ids[e.a], ids[e.b] = true, true
	}
	for i := range att {
		ids[att[i].join] = true
	}
	order := make([]int64, 0, len(ids))
	for id := range ids {
		order = append(order, id)
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := g.pos[order[i]], g.pos[order[j]]
		if math.Round(a.Y) != math.Round(b.Y) {
			return a.Y < b.Y
		}
		if math.Round(a.X) != math.Round(b.X) {
			return a.X < b.X
		}
		return order[i] < order[j]
	})
	name := map[int64]string{}
	for i, id := range order {
		name[id] = fmt.Sprintf("j%03d", i+1)
	}
	return name
}

func sortedJunctions(name map[int64]string, _ []attachment) []int64 {
	out := make([]int64, 0, len(name))
	for id := range name {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return name[out[i]] < name[out[j]] })
	return out
}
