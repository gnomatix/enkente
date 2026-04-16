package graph

import (
	"fmt"
	"sync"
)

// ConceptGraph is a thread-safe in-memory graph with adjacency lists.
type ConceptGraph struct {
	mu       sync.RWMutex
	concepts map[string]*Concept // keyed by concept ID
	edges    map[string]*Edge    // keyed by edge ID
	outAdj   map[string][]string // concept ID -> list of edge IDs (outgoing)
	inAdj    map[string][]string // concept ID -> list of edge IDs (incoming)
}

// NewConceptGraph creates an empty graph.
func NewConceptGraph() *ConceptGraph {
	return &ConceptGraph{
		concepts: make(map[string]*Concept),
		edges:    make(map[string]*Edge),
		outAdj:   make(map[string][]string),
		inAdj:    make(map[string][]string),
	}
}

// AddConcept adds or updates a concept in the graph. If a concept with the
// same ID already exists, its provenance count is incremented and its
// modified timestamp is updated.
func (g *ConceptGraph) AddConcept(c *Concept) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if existing, ok := g.concepts[c.ID]; ok {
		existing.Provenance.Count += c.Provenance.Count
		existing.Provenance.Modified = c.Provenance.Modified
		// Merge aliases
		seen := make(map[string]bool)
		for _, a := range existing.Aliases {
			seen[a] = true
		}
		for _, a := range c.Aliases {
			if !seen[a] {
				existing.Aliases = append(existing.Aliases, a)
			}
		}
		// Merge ontology refs
		refSeen := make(map[string]bool)
		for _, r := range existing.OntologyRefs {
			refSeen[r.Source+":"+r.ID] = true
		}
		for _, r := range c.OntologyRefs {
			if !refSeen[r.Source+":"+r.ID] {
				existing.OntologyRefs = append(existing.OntologyRefs, r)
			}
		}
		return
	}
	g.concepts[c.ID] = c
}

// AddEdge adds or updates an edge. If an edge with the same ID exists,
// its weight is incremented and modified timestamp updated.
func (g *ConceptGraph) AddEdge(e *Edge) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if existing, ok := g.edges[e.ID]; ok {
		existing.Weight += e.Weight
		existing.Modified = e.Modified
		return
	}
	g.edges[e.ID] = e
	g.outAdj[e.Source] = append(g.outAdj[e.Source], e.ID)
	g.inAdj[e.Target] = append(g.inAdj[e.Target], e.ID)
}

// RemoveConcept removes a concept from the graph by ID.
// Callers should remove associated edges first via RemoveEdge.
func (g *ConceptGraph) RemoveConcept(id string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	delete(g.concepts, id)
	delete(g.outAdj, id)
	delete(g.inAdj, id)
}

// RemoveEdge removes an edge from the graph by ID and cleans up adjacency lists.
func (g *ConceptGraph) RemoveEdge(id string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	e, ok := g.edges[id]
	if !ok {
		return
	}

	// Remove from outgoing adjacency
	g.outAdj[e.Source] = removeFromSlice(g.outAdj[e.Source], id)
	// Remove from incoming adjacency
	g.inAdj[e.Target] = removeFromSlice(g.inAdj[e.Target], id)

	delete(g.edges, id)
}

// GetConcept retrieves a concept by ID. Returns nil if not found.
func (g *ConceptGraph) GetConcept(id string) *Concept {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.concepts[id]
}

// GetEdge retrieves an edge by ID. Returns nil if not found.
func (g *ConceptGraph) GetEdge(id string) *Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.edges[id]
}

// GetNeighbors returns all concepts connected to the given concept ID
// (both outgoing and incoming edges).
func (g *ConceptGraph) GetNeighbors(conceptID string) []*Concept {
	g.mu.RLock()
	defer g.mu.RUnlock()

	seen := make(map[string]bool)
	var neighbors []*Concept

	// Outgoing edges
	for _, eid := range g.outAdj[conceptID] {
		e := g.edges[eid]
		if e != nil && !seen[e.Target] {
			seen[e.Target] = true
			if c := g.concepts[e.Target]; c != nil {
				neighbors = append(neighbors, c)
			}
		}
	}

	// Incoming edges
	for _, eid := range g.inAdj[conceptID] {
		e := g.edges[eid]
		if e != nil && !seen[e.Source] {
			seen[e.Source] = true
			if c := g.concepts[e.Source]; c != nil {
				neighbors = append(neighbors, c)
			}
		}
	}

	return neighbors
}

// GetByType returns all concepts of the given type.
func (g *ConceptGraph) GetByType(t ConceptType) []*Concept {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var result []*Concept
	for _, c := range g.concepts {
		if c.Type == t {
			result = append(result, c)
		}
	}
	return result
}

// AllConcepts returns all concepts in the graph.
func (g *ConceptGraph) AllConcepts() []*Concept {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make([]*Concept, 0, len(g.concepts))
	for _, c := range g.concepts {
		result = append(result, c)
	}
	return result
}

// AllEdges returns all edges in the graph.
func (g *ConceptGraph) AllEdges() []*Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make([]*Edge, 0, len(g.edges))
	for _, e := range g.edges {
		result = append(result, e)
	}
	return result
}

// EdgesFor returns all edges where the given concept ID is source or target.
func (g *ConceptGraph) EdgesFor(conceptID string) []*Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()

	seen := make(map[string]bool)
	var result []*Edge

	for _, eid := range g.outAdj[conceptID] {
		if !seen[eid] {
			seen[eid] = true
			if e := g.edges[eid]; e != nil {
				result = append(result, e)
			}
		}
	}
	for _, eid := range g.inAdj[conceptID] {
		if !seen[eid] {
			seen[eid] = true
			if e := g.edges[eid]; e != nil {
				result = append(result, e)
			}
		}
	}
	return result
}

// SubgraphFor returns a subgraph rooted at the given concept ID, traversing
// up to maxDepth hops. Returns the set of concepts and edges reachable.
func (g *ConceptGraph) SubgraphFor(rootID string, maxDepth int) ([]*Concept, []*Edge) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	visitedConcepts := make(map[string]bool)
	visitedEdges := make(map[string]bool)

	var concepts []*Concept
	var edges []*Edge

	// BFS
	queue := []struct {
		id    string
		depth int
	}{{rootID, 0}}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if visitedConcepts[item.id] {
			continue
		}
		visitedConcepts[item.id] = true

		if c := g.concepts[item.id]; c != nil {
			concepts = append(concepts, c)
		}

		if item.depth >= maxDepth {
			continue
		}

		// Traverse outgoing
		for _, eid := range g.outAdj[item.id] {
			if !visitedEdges[eid] {
				visitedEdges[eid] = true
				if e := g.edges[eid]; e != nil {
					edges = append(edges, e)
					if !visitedConcepts[e.Target] {
						queue = append(queue, struct {
							id    string
							depth int
						}{e.Target, item.depth + 1})
					}
				}
			}
		}

		// Traverse incoming
		for _, eid := range g.inAdj[item.id] {
			if !visitedEdges[eid] {
				visitedEdges[eid] = true
				if e := g.edges[eid]; e != nil {
					edges = append(edges, e)
					if !visitedConcepts[e.Source] {
						queue = append(queue, struct {
							id    string
							depth int
						}{e.Source, item.depth + 1})
					}
				}
			}
		}
	}

	return concepts, edges
}

// MergeFrom merges all concepts and edges from another graph into this one.
func (g *ConceptGraph) MergeFrom(other *ConceptGraph) {
	other.mu.RLock()
	otherConcepts := make([]*Concept, 0, len(other.concepts))
	for _, c := range other.concepts {
		otherConcepts = append(otherConcepts, c)
	}
	otherEdges := make([]*Edge, 0, len(other.edges))
	for _, e := range other.edges {
		otherEdges = append(otherEdges, e)
	}
	other.mu.RUnlock()

	for _, c := range otherConcepts {
		g.AddConcept(c)
	}
	for _, e := range otherEdges {
		g.AddEdge(e)
	}
}

// Stats returns summary statistics about the graph.
func (g *ConceptGraph) Stats() GraphStats {
	g.mu.RLock()
	defer g.mu.RUnlock()

	stats := GraphStats{
		NumConcepts:   len(g.concepts),
		NumEdges:      len(g.edges),
		ConceptCounts: make(map[string]int),
		EdgeCounts:    make(map[string]int),
	}

	for _, c := range g.concepts {
		stats.ConceptCounts[string(c.Type)]++
	}
	for _, e := range g.edges {
		stats.EdgeCounts[string(e.Type)]++
	}

	return stats
}

// EdgeKey generates a deterministic edge ID from source, target, and relation type.
func EdgeKey(source, target string, rel RelationType) string {
	return fmt.Sprintf("%s-[%s]->%s", source, rel, target)
}

// removeFromSlice removes the first occurrence of val from a string slice.
func removeFromSlice(slice []string, val string) []string {
	for i, v := range slice {
		if v == val {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}
