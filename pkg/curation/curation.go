package curation

import (
	"fmt"
	"strings"
	"time"

	"github.com/gnomatix/enkente/pkg/graph"
	"github.com/gnomatix/enkente/pkg/storage"
)

// ConceptUpdate represents a user's request to update a concept.
type ConceptUpdate struct {
	Label        *string           `json:"label,omitempty"`
	Aliases      []string          `json:"aliases,omitempty"`
	Properties   map[string]any    `json:"properties,omitempty"`
	OntologyRefs []graph.OntologyRef `json:"ontology_refs,omitempty"`
}

// EdgeUpdate represents a user's request to update an edge.
type EdgeUpdate struct {
	Weight     *float64       `json:"weight,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
}

// NewConceptRequest represents a user's request to create a new concept.
type NewConceptRequest struct {
	ID           string              `json:"id"`
	Type         graph.ConceptType   `json:"type"`
	Label        string              `json:"label"`
	Aliases      []string            `json:"aliases,omitempty"`
	OntologyRefs []graph.OntologyRef `json:"ontology_refs,omitempty"`
	Properties   map[string]any      `json:"properties,omitempty"`
	Session      string              `json:"session,omitempty"`
}

// NewEdgeRequest represents a user's request to create a new edge.
type NewEdgeRequest struct {
	Source     string             `json:"source"`
	Target     string             `json:"target"`
	Type       graph.RelationType `json:"type"`
	Weight     float64            `json:"weight"`
	Properties map[string]any     `json:"properties,omitempty"`
	Session    string             `json:"session,omitempty"`
}

// DeleteRequest represents a request to remove a concept or edge.
type DeleteRequest struct {
	ID string `json:"id"`
}

// Curator provides read-write operations on parsed concepts,
// enabling two-way curation per the requirements:
// "ensure the processing pipeline allows for real-time read-write operations,
// where users or external processes can directly curate and correct the parsed
// concepts, instantly updating the underlying datastore."
type Curator struct {
	g     *graph.ConceptGraph
	store *storage.BoltStorage
}

// NewCurator creates a curator. store may be nil for in-memory only.
func NewCurator(g *graph.ConceptGraph, store *storage.BoltStorage) *Curator {
	return &Curator{g: g, store: store}
}

// CreateConcept adds a user-curated concept to the graph and persists it.
func (c *Curator) CreateConcept(req NewConceptRequest) (*graph.Concept, error) {
	if req.ID == "" {
		return nil, fmt.Errorf("concept ID is required")
	}
	if req.Label == "" {
		return nil, fmt.Errorf("concept label is required")
	}

	// Check if it already exists
	if existing := c.g.GetConcept(req.ID); existing != nil {
		return nil, fmt.Errorf("concept %s already exists", req.ID)
	}

	now := time.Now()
	session := req.Session
	if session == "" {
		session = "curated"
	}

	concept := &graph.Concept{
		ID:           req.ID,
		Type:         req.Type,
		Label:        req.Label,
		Aliases:      req.Aliases,
		OntologyRefs: req.OntologyRefs,
		Properties:   req.Properties,
		Provenance: graph.Provenance{
			Source:   "manual",
			Session:  session,
			Created:  now,
			Modified: now,
			Count:    1,
		},
	}

	c.g.AddConcept(concept)

	// Persist to storage
	if c.store != nil {
		if err := c.store.PutJSON(storage.ConceptBucket, concept.ID, concept); err != nil {
			return concept, fmt.Errorf("persisted to graph but storage failed: %w", err)
		}
		if session != "" {
			_ = c.store.AddToIndex(storage.ConceptsBySessionBucket, session, concept.ID)
		}
	}

	return concept, nil
}

// UpdateConcept modifies an existing concept in the graph and persists changes.
func (c *Curator) UpdateConcept(conceptID string, update ConceptUpdate) (*graph.Concept, error) {
	concept := c.g.GetConcept(conceptID)
	if concept == nil {
		return nil, fmt.Errorf("concept %s not found", conceptID)
	}

	now := time.Now()

	// Apply updates
	if update.Label != nil {
		concept.Label = *update.Label
	}

	if update.Aliases != nil {
		// Merge aliases
		seen := make(map[string]bool)
		for _, a := range concept.Aliases {
			seen[a] = true
		}
		for _, a := range update.Aliases {
			if !seen[a] {
				concept.Aliases = append(concept.Aliases, a)
			}
		}
	}

	if update.Properties != nil {
		if concept.Properties == nil {
			concept.Properties = make(map[string]any)
		}
		for k, v := range update.Properties {
			concept.Properties[k] = v
		}
	}

	if update.OntologyRefs != nil {
		refSeen := make(map[string]bool)
		for _, r := range concept.OntologyRefs {
			refSeen[r.Source+":"+r.ID] = true
		}
		for _, r := range update.OntologyRefs {
			if !refSeen[r.Source+":"+r.ID] {
				concept.OntologyRefs = append(concept.OntologyRefs, r)
			}
		}
	}

	concept.Provenance.Modified = now
	concept.Provenance.Source = "curated"

	// Persist
	if c.store != nil {
		if err := c.store.PutJSON(storage.ConceptBucket, concept.ID, concept); err != nil {
			return concept, fmt.Errorf("updated graph but storage failed: %w", err)
		}
	}

	return concept, nil
}

// DeleteConcept removes a concept and its associated edges from the graph.
func (c *Curator) DeleteConcept(conceptID string) error {
	concept := c.g.GetConcept(conceptID)
	if concept == nil {
		return fmt.Errorf("concept %s not found", conceptID)
	}

	// Get all edges for this concept and remove them
	edges := c.g.EdgesFor(conceptID)
	for _, e := range edges {
		c.g.RemoveEdge(e.ID)
		if c.store != nil {
			_ = c.store.Delete(storage.EdgeBucket, e.ID)
		}
	}

	// Remove the concept
	c.g.RemoveConcept(conceptID)
	if c.store != nil {
		_ = c.store.Delete(storage.ConceptBucket, conceptID)
	}

	return nil
}

// CreateEdge adds a user-curated edge to the graph and persists it.
func (c *Curator) CreateEdge(req NewEdgeRequest) (*graph.Edge, error) {
	if req.Source == "" || req.Target == "" {
		return nil, fmt.Errorf("source and target are required")
	}

	// Verify endpoints exist
	if c.g.GetConcept(req.Source) == nil {
		return nil, fmt.Errorf("source concept %s not found", req.Source)
	}
	if c.g.GetConcept(req.Target) == nil {
		return nil, fmt.Errorf("target concept %s not found", req.Target)
	}

	now := time.Now()
	session := req.Session
	if session == "" {
		session = "curated"
	}

	weight := req.Weight
	if weight == 0 {
		weight = 1
	}

	edge := &graph.Edge{
		ID:         graph.EdgeKey(req.Source, req.Target, req.Type),
		Source:     req.Source,
		Target:     req.Target,
		Type:       req.Type,
		Weight:     weight,
		Session:    session,
		Created:    now,
		Modified:   now,
		Properties: req.Properties,
	}

	c.g.AddEdge(edge)

	if c.store != nil {
		if err := c.store.PutJSON(storage.EdgeBucket, edge.ID, edge); err != nil {
			return edge, fmt.Errorf("persisted to graph but storage failed: %w", err)
		}
		_ = c.store.AddToIndex(storage.EdgesBySourceBucket, edge.Source, edge.ID)
		_ = c.store.AddToIndex(storage.EdgesByTargetBucket, edge.Target, edge.ID)
	}

	return edge, nil
}

// UpdateEdge modifies an existing edge's weight and properties.
func (c *Curator) UpdateEdge(edgeID string, update EdgeUpdate) (*graph.Edge, error) {
	edge := c.g.GetEdge(edgeID)
	if edge == nil {
		return nil, fmt.Errorf("edge %s not found", edgeID)
	}

	if update.Weight != nil {
		edge.Weight = *update.Weight
	}

	if update.Properties != nil {
		if edge.Properties == nil {
			edge.Properties = make(map[string]any)
		}
		for k, v := range update.Properties {
			edge.Properties[k] = v
		}
	}

	edge.Modified = time.Now()

	if c.store != nil {
		if err := c.store.PutJSON(storage.EdgeBucket, edge.ID, edge); err != nil {
			return edge, fmt.Errorf("updated graph but storage failed: %w", err)
		}
	}

	return edge, nil
}

// DeleteEdge removes an edge from the graph and storage.
func (c *Curator) DeleteEdge(edgeID string) error {
	edge := c.g.GetEdge(edgeID)
	if edge == nil {
		return fmt.Errorf("edge %s not found", edgeID)
	}

	c.g.RemoveEdge(edgeID)

	if c.store != nil {
		_ = c.store.Delete(storage.EdgeBucket, edgeID)
	}

	return nil
}

// MergeConcepts merges two concepts into one: conceptB is absorbed into conceptA.
// All edges referencing conceptB are rewired to conceptA.
// conceptB's aliases and properties are merged into conceptA.
func (c *Curator) MergeConcepts(conceptAID, conceptBID string) (*graph.Concept, error) {
	a := c.g.GetConcept(conceptAID)
	b := c.g.GetConcept(conceptBID)
	if a == nil {
		return nil, fmt.Errorf("concept %s not found", conceptAID)
	}
	if b == nil {
		return nil, fmt.Errorf("concept %s not found", conceptBID)
	}

	// Merge aliases
	aliasSet := make(map[string]bool)
	for _, alias := range a.Aliases {
		aliasSet[alias] = true
	}
	aliasSet[b.Label] = true // B's label becomes an alias
	for _, alias := range b.Aliases {
		aliasSet[alias] = true
	}
	a.Aliases = nil
	for alias := range aliasSet {
		a.Aliases = append(a.Aliases, alias)
	}

	// Merge properties
	if b.Properties != nil {
		if a.Properties == nil {
			a.Properties = make(map[string]any)
		}
		for k, v := range b.Properties {
			if _, exists := a.Properties[k]; !exists {
				a.Properties[k] = v
			}
		}
	}

	// Merge ontology refs
	refSeen := make(map[string]bool)
	for _, r := range a.OntologyRefs {
		refSeen[r.Source+":"+r.ID] = true
	}
	for _, r := range b.OntologyRefs {
		if !refSeen[r.Source+":"+r.ID] {
			a.OntologyRefs = append(a.OntologyRefs, r)
		}
	}

	// Rewire edges from B to A
	edges := c.g.EdgesFor(conceptBID)
	for _, e := range edges {
		newSource := e.Source
		newTarget := e.Target
		if e.Source == conceptBID {
			newSource = conceptAID
		}
		if e.Target == conceptBID {
			newTarget = conceptAID
		}
		// Skip self-loops
		if newSource == newTarget {
			c.g.RemoveEdge(e.ID)
			continue
		}
		// Create rewired edge
		newEdgeID := graph.EdgeKey(newSource, newTarget, e.Type)
		newEdge := &graph.Edge{
			ID:         newEdgeID,
			Source:     newSource,
			Target:     newTarget,
			Type:       e.Type,
			Weight:     e.Weight,
			Session:    e.Session,
			Created:    e.Created,
			Modified:   time.Now(),
			Properties: e.Properties,
		}
		c.g.RemoveEdge(e.ID)
		c.g.AddEdge(newEdge)

		if c.store != nil {
			_ = c.store.Delete(storage.EdgeBucket, e.ID)
			_ = c.store.PutJSON(storage.EdgeBucket, newEdge.ID, newEdge)
		}
	}

	// Remove concept B
	c.g.RemoveConcept(conceptBID)
	if c.store != nil {
		_ = c.store.Delete(storage.ConceptBucket, conceptBID)
	}

	// Update concept A
	a.Provenance.Modified = time.Now()
	a.Provenance.Count += b.Provenance.Count
	if c.store != nil {
		_ = c.store.PutJSON(storage.ConceptBucket, a.ID, a)
	}

	return a, nil
}

// SplitConcept splits a concept into two: the original retains its ID,
// and a new concept is created with the specified subset of properties.
func (c *Curator) SplitConcept(conceptID string, newReq NewConceptRequest, edgeIDsToMove []string) (*graph.Concept, *graph.Concept, error) {
	original := c.g.GetConcept(conceptID)
	if original == nil {
		return nil, nil, fmt.Errorf("concept %s not found", conceptID)
	}

	// Create the new concept
	newConcept, err := c.CreateConcept(newReq)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create split concept: %w", err)
	}

	// Move specified edges to the new concept
	for _, eid := range edgeIDsToMove {
		edge := c.g.GetEdge(eid)
		if edge == nil {
			continue
		}

		newSource := edge.Source
		newTarget := edge.Target
		if edge.Source == conceptID {
			newSource = newConcept.ID
		}
		if edge.Target == conceptID {
			newTarget = newConcept.ID
		}

		newEdgeID := graph.EdgeKey(newSource, newTarget, edge.Type)
		newEdge := &graph.Edge{
			ID:         newEdgeID,
			Source:     newSource,
			Target:     newTarget,
			Type:       edge.Type,
			Weight:     edge.Weight,
			Session:    edge.Session,
			Created:    edge.Created,
			Modified:   time.Now(),
			Properties: edge.Properties,
		}

		c.g.RemoveEdge(eid)
		c.g.AddEdge(newEdge)

		if c.store != nil {
			_ = c.store.Delete(storage.EdgeBucket, eid)
			_ = c.store.PutJSON(storage.EdgeBucket, newEdge.ID, newEdge)
		}
	}

	// Create a related_to edge between original and split
	relEdge := &graph.Edge{
		ID:       graph.EdgeKey(conceptID, newConcept.ID, graph.RelatedTo),
		Source:   conceptID,
		Target:   newConcept.ID,
		Type:     graph.RelatedTo,
		Weight:   1,
		Session:  "curated",
		Created:  time.Now(),
		Modified: time.Now(),
		Properties: map[string]any{
			"relationship": "split_from",
		},
	}
	c.g.AddEdge(relEdge)
	if c.store != nil {
		_ = c.store.PutJSON(storage.EdgeBucket, relEdge.ID, relEdge)
	}

	return original, newConcept, nil
}

// SearchConcepts finds concepts matching a text query against labels and aliases.
func (c *Curator) SearchConcepts(query string) []*graph.Concept {
	query = strings.ToLower(query)
	allConcepts := c.g.AllConcepts()
	var results []*graph.Concept

	for _, concept := range allConcepts {
		if strings.Contains(strings.ToLower(concept.Label), query) {
			results = append(results, concept)
			continue
		}
		for _, alias := range concept.Aliases {
			if strings.Contains(strings.ToLower(alias), query) {
				results = append(results, concept)
				break
			}
		}
	}

	return results
}
