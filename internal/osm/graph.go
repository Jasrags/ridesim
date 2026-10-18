package osm

import (
	"math"
	"sort"
)

// seg is one walkway segment between two graph nodes.
type seg struct {
	length float64
	width  float64
	class  string // highway value, for the width provenance comment
	way    int64  // source way (0 for synthetic links)
}

// graph is the working walkway graph in projected feet. OSM node ids are positive; nodes the
// importer creates (split points, plaza centres) get negative ids.
type graph struct {
	pos    map[int64]Pt
	adj    map[int64]map[int64]seg
	nextID int64
}

func newGraph() *graph {
	return &graph{pos: map[int64]Pt{}, adj: map[int64]map[int64]seg{}, nextID: -1}
}

func (g *graph) addNode(id int64, p Pt) {
	if _, ok := g.pos[id]; !ok {
		g.pos[id] = p
		g.adj[id] = map[int64]seg{}
	}
}

func (g *graph) newNode(p Pt) int64 {
	id := g.nextID
	g.nextID--
	g.addNode(id, p)
	return id
}

// link adds a segment; a parallel segment keeps the shorter one.
func (g *graph) link(a, b int64, s seg) {
	if a == b {
		return
	}
	if old, ok := g.adj[a][b]; ok && old.length <= s.length {
		return
	}
	g.adj[a][b] = s
	g.adj[b][a] = s
}

func (g *graph) unlink(a, b int64) {
	delete(g.adj[a], b)
	delete(g.adj[b], a)
}

func (g *graph) removeNode(id int64) {
	for n := range g.adj[id] {
		delete(g.adj[n], id)
	}
	delete(g.adj, id)
	delete(g.pos, id)
}

func (g *graph) ids() []int64 {
	out := make([]int64, 0, len(g.pos))
	for id := range g.pos {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (g *graph) neighbors(id int64) []int64 {
	out := make([]int64, 0, len(g.adj[id]))
	for n := range g.adj[id] {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// segRef names one segment by its endpoints, a < b.
type segRef struct{ a, b int64 }

func (g *graph) segments() []segRef {
	var out []segRef
	for _, a := range g.ids() {
		for _, b := range g.neighbors(a) {
			if a < b {
				out = append(out, segRef{a, b})
			}
		}
	}
	return out
}

// nearestSegment finds the segment closest to p that passes the filter.
func (g *graph) nearestSegment(p Pt, ok func(segRef) bool) (best segRef, at Pt, t, d float64, found bool) {
	d = math.Inf(1)
	for _, s := range g.segments() {
		if ok != nil && !ok(s) {
			continue
		}
		q, tt := closestOnSegment(p, g.pos[s.a], g.pos[s.b])
		if dd := dist(p, q); dd < d {
			best, at, t, d, found = s, q, tt, dd, true
		}
	}
	return
}

// splitAt returns a node at parameter t along segment s: an endpoint when the split point is within
// minFt of it, otherwise a new node that replaces the segment with two.
func (g *graph) splitAt(s segRef, at Pt, t, minFt float64) int64 {
	whole := g.adj[s.a][s.b]
	l := dist(g.pos[s.a], g.pos[s.b])
	if t*l < minFt {
		return s.a
	}
	if (1-t)*l < minFt {
		return s.b
	}
	n := g.newNode(at)
	g.unlink(s.a, s.b)
	g.link(s.a, n, seg{dist(g.pos[s.a], at), whole.width, whole.class, whole.way})
	g.link(n, s.b, seg{dist(at, g.pos[s.b]), whole.width, whole.class, whole.way})
	return n
}

// components returns connected components, largest first (ties by smallest node id).
func (g *graph) components() [][]int64 {
	seen := map[int64]bool{}
	var comps [][]int64
	for _, s := range g.ids() {
		if seen[s] {
			continue
		}
		comp := []int64{s}
		seen[s] = true
		for i := 0; i < len(comp); i++ {
			for _, n := range g.neighbors(comp[i]) {
				if !seen[n] {
					seen[n] = true
					comp = append(comp, n)
				}
			}
		}
		sort.Slice(comp, func(i, j int) bool { return comp[i] < comp[j] })
		comps = append(comps, comp)
	}
	sort.SliceStable(comps, func(i, j int) bool { return len(comps[i]) > len(comps[j]) })
	return comps
}

// prune removes dead ends that are not kept, repeatedly.
func (g *graph) prune(keep map[int64]bool) {
	for {
		removed := false
		for _, id := range g.ids() {
			if !keep[id] && len(g.adj[id]) <= 1 {
				g.removeNode(id)
				removed = true
			}
		}
		if !removed {
			return
		}
	}
}

// edge is a contracted walkway between two kept nodes.
type edge struct {
	a, b   int64
	length float64
	width  float64 // narrowest width along the chain
	class  string  // highway value of the narrowest segment
}

// contract merges chains of degree-2 nodes into single edges between kept nodes (kept, dead ends
// and junctions of three or more). The edge length is the summed chain length.
func (g *graph) contract(keep map[int64]bool) []edge {
	isKey := func(id int64) bool { return keep[id] || len(g.adj[id]) != 2 }
	done := map[segRef]bool{}
	ref := func(a, b int64) segRef {
		if a > b {
			a, b = b, a
		}
		return segRef{a, b}
	}
	var out []edge
	for _, u := range g.ids() {
		if !isKey(u) {
			continue
		}
		for _, v := range g.neighbors(u) {
			if done[ref(u, v)] {
				continue
			}
			s := g.adj[u][v]
			e := edge{a: u, length: s.length, width: s.width, class: s.class}
			done[ref(u, v)] = true
			prev, cur := u, v
			for !isKey(cur) {
				var next int64
				for _, n := range g.neighbors(cur) {
					if n != prev {
						next = n
					}
				}
				s := g.adj[cur][next]
				done[ref(cur, next)] = true
				e.length += s.length
				if s.width < e.width {
					e.width, e.class = s.width, s.class
				}
				prev, cur = cur, next
			}
			e.b = cur
			if e.a != e.b {
				out = append(out, e)
			}
		}
	}
	return dedupe(out)
}

// dedupe drops self-loops and keeps the shortest of parallel edges, ordered by endpoints.
func dedupe(es []edge) []edge {
	best := map[segRef]edge{}
	for _, e := range es {
		if e.a == e.b {
			continue
		}
		if e.a > e.b {
			e.a, e.b = e.b, e.a
		}
		k := segRef{e.a, e.b}
		if old, ok := best[k]; !ok || e.length < old.length {
			best[k] = e
		}
	}
	out := make([]edge, 0, len(best))
	for _, e := range best {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].a != out[j].a {
			return out[i].a < out[j].a
		}
		return out[i].b < out[j].b
	})
	return out
}
