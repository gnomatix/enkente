package namespace

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gnomatix/enkente/pkg/graph"
)

// Dimension represents the type of namespace axis.
type Dimension string

const (
	SessionDim  Dimension = "session"  // isolate by chat session
	UserDim     Dimension = "user"     // isolate by user/participant
	SubjectDim  Dimension = "subject"  // isolate by subject matter
	ProjectDim  Dimension = "project"  // isolate by overarching project
	TemporalDim Dimension = "temporal" // isolate by time period
)

// Namespace represents a named scope that concepts can belong to.
type Namespace struct {
	ID         string            `json:"id"`
	Dimension  Dimension         `json:"dimension"`
	Label      string            `json:"label"`
	Parent     string            `json:"parent,omitempty"`   // hierarchical nesting
	Created    time.Time         `json:"created"`
	Modified   time.Time         `json:"modified"`
	Properties map[string]any    `json:"properties,omitempty"`
}

// Binding records that a concept belongs to a namespace.
type Binding struct {
	ConceptID   string    `json:"concept_id"`
	NamespaceID string    `json:"namespace_id"`
	Strength    float64   `json:"strength"`    // 0.0 to 1.0, how strongly associated
	Created     time.Time `json:"created"`
}

// Registry manages dynamic namespaces and concept-namespace bindings.
// Per requirements: "robust namespacing to isolate and contextualize data by
// session, individual user, subject matter, overarching project, or temporal bounds."
type Registry struct {
	mu         sync.RWMutex
	namespaces map[string]*Namespace          // namespace ID -> namespace
	bindings   map[string]map[string]*Binding // concept ID -> namespace ID -> binding
	nsIndex    map[string]map[string]bool     // namespace ID -> set of concept IDs
}

// NewRegistry creates a new namespace registry.
func NewRegistry() *Registry {
	return &Registry{
		namespaces: make(map[string]*Namespace),
		bindings:   make(map[string]map[string]*Binding),
		nsIndex:    make(map[string]map[string]bool),
	}
}

// CreateNamespace creates a new namespace. Returns the namespace.
func (r *Registry) CreateNamespace(dim Dimension, label string, parent string) *Namespace {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	id := fmt.Sprintf("%s:%s", dim, strings.ToLower(strings.ReplaceAll(label, " ", "_")))

	if existing, ok := r.namespaces[id]; ok {
		existing.Modified = now
		return existing
	}

	ns := &Namespace{
		ID:        id,
		Dimension: dim,
		Label:     label,
		Parent:    parent,
		Created:   now,
		Modified:  now,
	}
	r.namespaces[id] = ns
	r.nsIndex[id] = make(map[string]bool)
	return ns
}

// GetNamespace retrieves a namespace by ID.
func (r *Registry) GetNamespace(id string) *Namespace {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.namespaces[id]
}

// ListNamespaces returns all namespaces, optionally filtered by dimension.
func (r *Registry) ListNamespaces(dim Dimension) []*Namespace {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Namespace
	for _, ns := range r.namespaces {
		if dim == "" || ns.Dimension == dim {
			result = append(result, ns)
		}
	}
	return result
}

// Bind associates a concept with a namespace.
func (r *Registry) Bind(conceptID, namespaceID string, strength float64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.namespaces[namespaceID]; !ok {
		return fmt.Errorf("namespace %s not found", namespaceID)
	}

	if r.bindings[conceptID] == nil {
		r.bindings[conceptID] = make(map[string]*Binding)
	}

	existing, ok := r.bindings[conceptID][namespaceID]
	if ok {
		// Strengthen the binding
		existing.Strength = clampStrength(existing.Strength + strength)
		return nil
	}

	binding := &Binding{
		ConceptID:   conceptID,
		NamespaceID: namespaceID,
		Strength:    clampStrength(strength),
		Created:     time.Now(),
	}
	r.bindings[conceptID][namespaceID] = binding
	r.nsIndex[namespaceID][conceptID] = true
	return nil
}

// Unbind removes a concept from a namespace.
func (r *Registry) Unbind(conceptID, namespaceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if binds, ok := r.bindings[conceptID]; ok {
		delete(binds, namespaceID)
	}
	if idx, ok := r.nsIndex[namespaceID]; ok {
		delete(idx, conceptID)
	}
}

// ConceptNamespaces returns all namespaces a concept belongs to.
func (r *Registry) ConceptNamespaces(conceptID string) []*Binding {
	r.mu.RLock()
	defer r.mu.RUnlock()

	binds, ok := r.bindings[conceptID]
	if !ok {
		return nil
	}

	result := make([]*Binding, 0, len(binds))
	for _, b := range binds {
		result = append(result, b)
	}
	return result
}

// NamespaceConcepts returns all concept IDs in a namespace.
func (r *Registry) NamespaceConcepts(namespaceID string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	idx, ok := r.nsIndex[namespaceID]
	if !ok {
		return nil
	}

	result := make([]string, 0, len(idx))
	for cid := range idx {
		result = append(result, cid)
	}
	return result
}

// FilterByNamespace returns a subgraph containing only concepts in the given namespace.
func (r *Registry) FilterByNamespace(g *graph.ConceptGraph, namespaceID string) ([]*graph.Concept, []*graph.Edge) {
	r.mu.RLock()
	conceptIDs := r.nsIndex[namespaceID]
	r.mu.RUnlock()

	if len(conceptIDs) == 0 {
		return nil, nil
	}

	conceptSet := make(map[string]bool)
	var concepts []*graph.Concept

	for cid := range conceptIDs {
		c := g.GetConcept(cid)
		if c != nil {
			concepts = append(concepts, c)
			conceptSet[cid] = true
		}
	}

	// Include edges where both endpoints are in the namespace
	var edges []*graph.Edge
	allEdges := g.AllEdges()
	for _, e := range allEdges {
		if conceptSet[e.Source] && conceptSet[e.Target] {
			edges = append(edges, e)
		}
	}

	return concepts, edges
}

// AutoBindFromExtraction automatically binds concepts to appropriate namespaces
// based on extraction metadata (session, user, concept type).
func (r *Registry) AutoBindFromExtraction(concepts []*graph.Concept) {
	for _, c := range concepts {
		// Session namespace
		if c.Provenance.Session != "" {
			sessionNS := r.CreateNamespace(SessionDim, c.Provenance.Session, "")
			_ = r.Bind(c.ID, sessionNS.ID, 1.0)
		}

		// User namespace
		if c.Provenance.User != "" {
			userNS := r.CreateNamespace(UserDim, c.Provenance.User, "")
			_ = r.Bind(c.ID, userNS.ID, 0.8)
		}

		// Subject namespace based on concept type
		switch c.Type {
		case graph.TopicType:
			subjectNS := r.CreateNamespace(SubjectDim, c.Label, "")
			_ = r.Bind(c.ID, subjectNS.ID, 1.0)
		case graph.QuotedTermType:
			// Quoted terms may represent subject-specific jargon
			subjectNS := r.CreateNamespace(SubjectDim, c.Label, "")
			_ = r.Bind(c.ID, subjectNS.ID, 0.5)
		}
	}
}

// CreateTemporalNamespace creates a time-bounded namespace.
func (r *Registry) CreateTemporalNamespace(label string, start, end time.Time) *Namespace {
	ns := r.CreateNamespace(TemporalDim, label, "")

	r.mu.Lock()
	defer r.mu.Unlock()

	if ns.Properties == nil {
		ns.Properties = make(map[string]any)
	}
	ns.Properties["start"] = start
	ns.Properties["end"] = end
	return ns
}

// BindByTime binds concepts to a temporal namespace based on their creation time.
func (r *Registry) BindByTime(g *graph.ConceptGraph, namespaceID string, start, end time.Time) {
	allConcepts := g.AllConcepts()
	for _, c := range allConcepts {
		if !c.Provenance.Created.Before(start) && c.Provenance.Created.Before(end) {
			_ = r.Bind(c.ID, namespaceID, 1.0)
		}
	}
}

// ApplyToGraph creates namespace concepts and binding edges in the graph.
func (r *Registry) ApplyToGraph(g *graph.ConceptGraph) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now()
	for _, ns := range r.namespaces {
		// Skip session namespaces -- they already exist in the graph
		if ns.Dimension == SessionDim {
			continue
		}

		conceptType := graph.TopicType
		switch ns.Dimension {
		case UserDim:
			conceptType = graph.PersonType
		case SubjectDim:
			conceptType = graph.TopicType
		case ProjectDim:
			conceptType = graph.TopicType
		}

		g.AddConcept(&graph.Concept{
			ID:    ns.ID,
			Type:  conceptType,
			Label: ns.Label,
			Provenance: graph.Provenance{
				Source:   "namespace",
				Created:  now,
				Modified: now,
				Count:    len(r.nsIndex[ns.ID]),
			},
			Properties: map[string]any{
				"namespace_dimension": string(ns.Dimension),
			},
		})

		// Parent edge
		if ns.Parent != "" {
			edgeID := graph.EdgeKey(ns.ID, ns.Parent, graph.TaggedWith)
			g.AddEdge(&graph.Edge{
				ID:       edgeID,
				Source:   ns.ID,
				Target:   ns.Parent,
				Type:     graph.TaggedWith,
				Weight:   1,
				Created:  now,
				Modified: now,
			})
		}
	}
}

func clampStrength(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
