package graph

import "time"

// ConceptType classifies graph nodes.
type ConceptType string

const (
	PersonType       ConceptType = "person"
	TopicType        ConceptType = "topic"
	QuotedTermType   ConceptType = "quoted_term"
	SessionType      ConceptType = "session"
	MethodologyType  ConceptType = "methodology"
	OrganizationType ConceptType = "organization"
)

// RelationType classifies graph edges.
type RelationType string

const (
	Mentions     RelationType = "mentions"
	Discusses    RelationType = "discusses"
	CoOccurs     RelationType = "co_occurs"
	Participates RelationType = "participates_in"
	Quotes       RelationType = "quotes"
	TaggedWith   RelationType = "tagged_with"
	RelatedTo    RelationType = "related_to"
)

// OntologyRef holds a cross-reference to an external ontology or controlled vocabulary.
type OntologyRef struct {
	Source string `json:"source"` // e.g. "wikidata", "mesh", "go"
	ID     string `json:"id"`     // e.g. "Q12345"
	Label  string `json:"label"`  // human-readable label
}

// Provenance records who introduced a concept, when, and in which session.
type Provenance struct {
	Source    string    `json:"source"`     // e.g. "extraction", "manual", "nlp"
	Session  string    `json:"session"`     // session ID where this was first seen
	User     string    `json:"user"`        // user who introduced it
	Created  time.Time `json:"created"`     // first seen timestamp
	Modified time.Time `json:"modified"`    // last updated timestamp
	Count    int       `json:"count"`       // number of times observed
}

// Concept represents a node in the concept graph.
type Concept struct {
	ID           string        `json:"id"`                      // unique key, e.g. "person:alice", "topic:graph_databases"
	Type         ConceptType   `json:"type"`                    // node classification
	Label        string        `json:"label"`                   // display label
	Aliases      []string      `json:"aliases,omitempty"`       // alternative names
	OntologyRefs []OntologyRef `json:"ontology_refs,omitempty"` // dbxref tags
	Provenance   Provenance    `json:"provenance"`              // tracking metadata
	Properties   map[string]any `json:"properties,omitempty"`   // extensible metadata
}

// Edge represents a directed relationship between two concepts.
type Edge struct {
	ID         string       `json:"id"`                    // unique edge key
	Source     string       `json:"source"`                // source concept ID
	Target     string       `json:"target"`                // target concept ID
	Type       RelationType `json:"type"`                  // relationship classification
	Weight     float64      `json:"weight"`                // strength/frequency
	Session    string       `json:"session"`               // session where this edge was observed
	Created    time.Time    `json:"created"`               // first seen
	Modified   time.Time    `json:"modified"`              // last updated
	Properties map[string]any `json:"properties,omitempty"` // extensible metadata
}

// GraphStats holds summary statistics about the concept graph.
type GraphStats struct {
	NumConcepts   int            `json:"num_concepts"`
	NumEdges      int            `json:"num_edges"`
	ConceptCounts map[string]int `json:"concept_counts"` // count by ConceptType
	EdgeCounts    map[string]int `json:"edge_counts"`    // count by RelationType
}
