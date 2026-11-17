package storage

import (
	"sort"
	"strings"
	"time"

	"go.etcd.io/bbolt"

	"github.com/gnomatix/enkente/pkg/parser"
)

// Graph is a node-link view of who talks about what, in the shape consumed
// by force-directed layouts (d3-force, Cytoscape, Gephi JSON importers).
type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// GraphNode is a participant or a referenced entity. ID is "<kind>:<name>"
// with kind "user" or "tag"; message authors and @mentions share the "user"
// namespace, so @alice and messages written by alice are one node.
type GraphNode struct {
	ID   string             `json:"id"`
	Kind parser.MentionKind `json:"kind"`
	Name string             `json:"name"`
	// Messages counts messages that reference the node (as a mention) or,
	// for users, that the user wrote.
	Messages  int       `json:"messages"`
	FirstSeen time.Time `json:"firstSeen"`
	LastSeen  time.Time `json:"lastSeen"`
}

// Edge relations.
const (
	// RelMentions: Source (a message author) referenced Target in a message.
	RelMentions = "mentions"
	// RelCoOccurs: Source and Target (both tags) appeared in the same
	// message. Source < Target, so each pair appears once.
	RelCoOccurs = "co-occurs"
)

// GraphEdge is a weighted relation; Weight counts messages.
type GraphEdge struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Relation string `json:"relation"`
	Weight   int    `json:"weight"`
}

// Graph builds the who-mentions-what graph over stored messages that match
// filter (zero filter = all messages). Pass sessionID "" for every session.
func (s *BoltStorage) Graph(sessionID string, filter MessageFilter) (*Graph, error) {
	nodes := map[string]*GraphNode{}
	type edgeKey struct{ src, dst, rel string }
	edges := map[edgeKey]int{}

	touch := func(kind parser.MentionKind, name string, ts time.Time) string {
		id := string(kind) + ":" + name
		n := nodes[id]
		if n == nil {
			n = &GraphNode{ID: id, Kind: kind, Name: name, FirstSeen: ts, LastSeen: ts}
			nodes[id] = n
		}
		n.Messages++
		if ts.Before(n.FirstSeen) {
			n.FirstSeen = ts
		}
		if ts.After(n.LastSeen) {
			n.LastSeen = ts
		}
		return id
	}

	visit := func(msg parser.AntigravityMessage) error {
		if !filter.Matches(msg) {
			return nil
		}
		if sessionID != "" && msg.SessionID != sessionID {
			return nil
		}
		ts := msg.Timestamp.UTC()
		author := strings.ToLower(strings.TrimSpace(msg.User))
		var authorID string
		if author != "" {
			authorID = touch(parser.MentionUser, author, ts)
		}
		var tags []string
		for _, m := range parser.ExtractMentions(msg.Message) {
			if m.Kind == parser.MentionUser && m.Name == author {
				continue // self-mention adds nothing
			}
			id := touch(m.Kind, m.Name, ts)
			if authorID != "" {
				edges[edgeKey{authorID, id, RelMentions}]++
			}
			if m.Kind == parser.MentionTag {
				tags = append(tags, id)
			}
		}
		sort.Strings(tags)
		for i := range tags {
			for j := i + 1; j < len(tags); j++ {
				edges[edgeKey{tags[i], tags[j], RelCoOccurs}]++
			}
		}
		return nil
	}

	err := s.db.View(func(tx *bbolt.Tx) error {
		if sessionID != "" {
			b, err := sessionBucket(tx, sessionID, false)
			if err != nil {
				return err
			}
			if b == nil {
				return ErrNotFound
			}
		}
		return forEachMessage(tx, visit)
	})
	if err != nil {
		return nil, err
	}

	g := &Graph{Nodes: make([]GraphNode, 0, len(nodes)), Edges: make([]GraphEdge, 0, len(edges))}
	for _, n := range nodes {
		g.Nodes = append(g.Nodes, *n)
	}
	sort.Slice(g.Nodes, func(i, j int) bool { return g.Nodes[i].ID < g.Nodes[j].ID })
	for k, w := range edges {
		g.Edges = append(g.Edges, GraphEdge{Source: k.src, Target: k.dst, Relation: k.rel, Weight: w})
	}
	sort.Slice(g.Edges, func(i, j int) bool {
		a, b := g.Edges[i], g.Edges[j]
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		return a.Relation < b.Relation
	})
	return g, nil
}
