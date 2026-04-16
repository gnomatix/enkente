package extract

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gnomatix/enkente/pkg/graph"
	"github.com/gnomatix/enkente/pkg/parser"
)

var (
	mentionRe = regexp.MustCompile(`@(\w+)`)
	tagRe     = regexp.MustCompile(`#(\w+)`)
	quotedRe  = regexp.MustCompile(`"([^"]{2,})"`)
)

// Result holds the extracted concepts and edges from a single message.
type Result struct {
	Concepts []*graph.Concept
	Edges    []*graph.Edge
}

// FromMessage performs structural extraction on a single AntigravityMessage.
// It identifies @mentions (person), #tags (topic), "quoted terms" (quoted_term),
// the sender as a person, session participation edges, and co-occurrence edges.
func FromMessage(msg parser.AntigravityMessage) Result {
	now := time.Now()
	session := msg.SessionID
	if session == "" {
		session = "live"
	}

	var concepts []*graph.Concept
	var edges []*graph.Edge

	prov := func(user string) graph.Provenance {
		return graph.Provenance{
			Source:   "extraction",
			Session:  session,
			User:     user,
			Created:  now,
			Modified: now,
			Count:    1,
		}
	}

	// Track all concept IDs from this message for co-occurrence
	var conceptIDs []string

	// 1. Sender as a person concept (if user field is non-empty)
	if msg.User != "" {
		senderID := fmt.Sprintf("person:%s", strings.ToLower(msg.User))
		concepts = append(concepts, &graph.Concept{
			ID:         senderID,
			Type:       graph.PersonType,
			Label:      msg.User,
			Provenance: prov(msg.User),
		})
		conceptIDs = append(conceptIDs, senderID)

		// Session participation edge
		sessionID := fmt.Sprintf("session:%s", session)
		concepts = append(concepts, &graph.Concept{
			ID:         sessionID,
			Type:       graph.SessionType,
			Label:      session,
			Provenance: prov(msg.User),
		})
		edges = append(edges, &graph.Edge{
			ID:       graph.EdgeKey(senderID, sessionID, graph.Participates),
			Source:   senderID,
			Target:   sessionID,
			Type:     graph.Participates,
			Weight:   1,
			Session:  session,
			Created:  now,
			Modified: now,
		})
	}

	// 2. @mentions -> person concepts + mentions edges
	for _, match := range mentionRe.FindAllStringSubmatch(msg.Message, -1) {
		name := strings.ToLower(match[1])
		cid := fmt.Sprintf("person:%s", name)
		concepts = append(concepts, &graph.Concept{
			ID:         cid,
			Type:       graph.PersonType,
			Label:      match[1],
			Provenance: prov(msg.User),
		})
		conceptIDs = append(conceptIDs, cid)

		// If we have a sender, create a mentions edge
		if msg.User != "" {
			senderID := fmt.Sprintf("person:%s", strings.ToLower(msg.User))
			edges = append(edges, &graph.Edge{
				ID:       graph.EdgeKey(senderID, cid, graph.Mentions),
				Source:   senderID,
				Target:   cid,
				Type:     graph.Mentions,
				Weight:   1,
				Session:  session,
				Created:  now,
				Modified: now,
			})
		}
	}

	// 3. #tags -> topic concepts + discusses edges
	for _, match := range tagRe.FindAllStringSubmatch(msg.Message, -1) {
		tag := strings.ToLower(match[1])
		cid := fmt.Sprintf("topic:%s", tag)
		concepts = append(concepts, &graph.Concept{
			ID:         cid,
			Type:       graph.TopicType,
			Label:      match[1],
			Provenance: prov(msg.User),
		})
		conceptIDs = append(conceptIDs, cid)

		// Discusses edge from sender
		if msg.User != "" {
			senderID := fmt.Sprintf("person:%s", strings.ToLower(msg.User))
			edges = append(edges, &graph.Edge{
				ID:       graph.EdgeKey(senderID, cid, graph.Discusses),
				Source:   senderID,
				Target:   cid,
				Type:     graph.Discusses,
				Weight:   1,
				Session:  session,
				Created:  now,
				Modified: now,
			})
		}
	}

	// 4. "quoted terms" -> quoted_term concepts + quotes edges
	for _, match := range quotedRe.FindAllStringSubmatch(msg.Message, -1) {
		term := strings.ToLower(match[1])
		cid := fmt.Sprintf("quoted_term:%s", strings.ReplaceAll(term, " ", "_"))
		concepts = append(concepts, &graph.Concept{
			ID:         cid,
			Type:       graph.QuotedTermType,
			Label:      match[1],
			Provenance: prov(msg.User),
		})
		conceptIDs = append(conceptIDs, cid)

		// Quotes edge from sender
		if msg.User != "" {
			senderID := fmt.Sprintf("person:%s", strings.ToLower(msg.User))
			edges = append(edges, &graph.Edge{
				ID:       graph.EdgeKey(senderID, cid, graph.Quotes),
				Source:   senderID,
				Target:   cid,
				Type:     graph.Quotes,
				Weight:   1,
				Session:  session,
				Created:  now,
				Modified: now,
			})
		}
	}

	// 5. Co-occurrence edges: all non-person concepts mentioned together
	// in the same message get co-occurrence edges between each pair
	var nonPersonIDs []string
	for _, cid := range conceptIDs {
		if !strings.HasPrefix(cid, "person:") && !strings.HasPrefix(cid, "session:") {
			nonPersonIDs = append(nonPersonIDs, cid)
		}
	}
	for i := 0; i < len(nonPersonIDs); i++ {
		for j := i + 1; j < len(nonPersonIDs); j++ {
			a, b := nonPersonIDs[i], nonPersonIDs[j]
			// Canonical ordering: alphabetically smaller is source
			if a > b {
				a, b = b, a
			}
			edges = append(edges, &graph.Edge{
				ID:       graph.EdgeKey(a, b, graph.CoOccurs),
				Source:   a,
				Target:   b,
				Type:     graph.CoOccurs,
				Weight:   1,
				Session:  session,
				Created:  now,
				Modified: now,
			})
		}
	}

	return Result{
		Concepts: concepts,
		Edges:    edges,
	}
}

// ApplyResult adds all concepts and edges from a Result into a ConceptGraph.
func ApplyResult(g *graph.ConceptGraph, r Result) {
	for _, c := range r.Concepts {
		g.AddConcept(c)
	}
	for _, e := range r.Edges {
		g.AddEdge(e)
	}
}
