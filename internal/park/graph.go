// Package park is the park-level crowd model (spec §9): a graph of attractions joined by walkways,
// individual party agents choosing what to do next on stale posted waits, walking with congestion and
// spillback, and Lightning Lane return windows. Real rides plug in by ride_id and run their full models
// in the same simulation, each in its own scope (events carry src = node id).
package park

import (
	"container/heap"
	"fmt"
	"math"

	"github.com/jasrags/ridesim/internal/config"
)

// Node is a park node with its index.
type Node struct {
	Idx int
	config.ParkNode
}

// Edge is a bidirectional walkway.
type Edge struct {
	Idx int
	config.ParkEdge
	from, to int // node indices

	guests int        // guests on the edge now (both directions)
	spill  func() int // guests of the adjoining ride's queue beyond its physical capacity; nil = none
	wait   []waiter   // parties waiting to enter while the edge is at jam density (FIFO)
}

// Other is the end of e that is not n.
func (e *Edge) Other(n int) int {
	if e.from == n {
		return e.to
	}
	return e.from
}

// Graph is the park layout with shortest routes between every pair of nodes: free-flow at first, and
// rerouted from current walking times when the park asks (decision.reroute_s).
type Graph struct {
	Nodes []*Node
	Edges []*Edge
	byID  map[string]int // node id -> index; lookup only, never iterated
	adj   [][]*Edge      // per node, in file order
	// next[a][b] is the first edge on the shortest route from a to b; dist is its cost in ft (length, or
	// walking time at free speed after a reroute).
	next [][]*Edge
	dist [][]float64
}

// NewGraph builds the graph from a validated park file.
func NewGraph(p *config.ParkFile) (*Graph, error) {
	g := &Graph{byID: map[string]int{}}
	for i, n := range p.Nodes {
		g.Nodes = append(g.Nodes, &Node{Idx: i, ParkNode: n})
		g.byID[n.ID] = i
	}
	g.adj = make([][]*Edge, len(g.Nodes))
	for i, e := range p.Edges {
		from, ok1 := g.byID[e.From]
		to, ok2 := g.byID[e.To]
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("edge %s: unknown endpoint", e.ID)
		}
		ed := &Edge{Idx: i, ParkEdge: e, from: from, to: to}
		g.Edges = append(g.Edges, ed)
		g.adj[from] = append(g.adj[from], ed)
		g.adj[to] = append(g.adj[to], ed)
	}
	g.routes(func(e *Edge) float64 { return e.LengthFt })
	return g, nil
}

// Node returns the node with the given id, or nil.
func (g *Graph) Node(id string) *Node {
	if i, ok := g.byID[id]; ok {
		return g.Nodes[i]
	}
	return nil
}

// Edge returns the edge with the given id, or nil.
func (g *Graph) Edge(id string) *Edge {
	for _, e := range g.Edges {
		if e.ID == id {
			return e
		}
	}
	return nil
}

// Route is the current shortest path from a to b as a list of edges (empty when a == b).
func (g *Graph) Route(a, b int) []*Edge {
	var out []*Edge
	for a != b {
		e := g.next[a][b]
		if e == nil {
			return nil
		}
		out = append(out, e)
		a = e.Other(a)
	}
	return out
}

// Distance is the current route cost from a to b in ft (walking time at free speed after a reroute).
func (g *Graph) Distance(a, b int) float64 { return g.dist[a][b] }

// Reroute recomputes every route with the given edge cost (in ft).
func (g *Graph) Reroute(cost func(*Edge) float64) { g.routes(cost) }

// routes runs Dijkstra from every node. Ties break on edge file order, so routes are deterministic.
func (g *Graph) routes(cost func(*Edge) float64) {
	n := len(g.Nodes)
	g.next = make([][]*Edge, n)
	g.dist = make([][]float64, n)
	for dst := 0; dst < n; dst++ {
		// Search from the destination; next[src][dst] is the edge src takes toward dst.
		dist := make([]float64, n)
		for i := range dist {
			dist[i] = math.Inf(1)
		}
		via := make([]*Edge, n)
		dist[dst] = 0
		pq := &nodeHeap{{dst, 0}}
		for pq.Len() > 0 {
			cur := heap.Pop(pq).(nodeDist)
			if cur.d > dist[cur.n] {
				continue
			}
			for _, e := range g.adj[cur.n] {
				o := e.Other(cur.n)
				if d := cur.d + cost(e); d < dist[o] {
					dist[o], via[o] = d, e
					heap.Push(pq, nodeDist{o, d})
				}
			}
		}
		for src := 0; src < n; src++ {
			if g.next[src] == nil {
				g.next[src] = make([]*Edge, n)
				g.dist[src] = make([]float64, n)
			}
			g.next[src][dst], g.dist[src][dst] = via[src], dist[src]
		}
	}
}

type nodeDist struct {
	n int
	d float64
}

type nodeHeap []nodeDist

func (h nodeHeap) Len() int { return len(h) }
func (h nodeHeap) Less(i, j int) bool {
	if h[i].d != h[j].d {
		return h[i].d < h[j].d
	}
	return h[i].n < h[j].n
}
func (h nodeHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *nodeHeap) Push(x any)   { *h = append(*h, x.(nodeDist)) }
func (h *nodeHeap) Pop() any {
	old := *h
	x := old[len(old)-1]
	*h = old[:len(old)-1]
	return x
}
