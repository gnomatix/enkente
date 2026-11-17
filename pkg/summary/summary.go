// Package summary produces on-demand session summaries from stored messages
// with a dependency-free baseline: keywords scored by TF-IDF against the
// other sessions, and an extractive summary made of the highest-scoring
// messages. It deliberately has no NLP library dependency; a richer engine
// (the NLTK stack named in docs/requirements.md) can replace it behind the
// Summarizer interface.
package summary

import (
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/gnomatix/enkente/pkg/parser"
)

// Keyword is a term that characterizes the session.
type Keyword struct {
	Term  string  `json:"term"`
	Count int     `json:"count"`
	Score float64 `json:"score"`
}

// Participant is a message author with their message count.
type Participant struct {
	User     string `json:"user"`
	Messages int    `json:"messages"`
}

// Summary is the on-demand digest of one session.
type Summary struct {
	SessionID    string                      `json:"sessionId"`
	Messages     int                         `json:"messages"`
	Start        time.Time                   `json:"start"`
	End          time.Time                   `json:"end"`
	Participants []Participant               `json:"participants"`
	Tags         []string                    `json:"tags"`
	Keywords     []Keyword                   `json:"keywords"`
	Highlights   []parser.AntigravityMessage `json:"highlights"`
	// Method names the engine so clients can tell baseline output apart.
	Method string `json:"method"`
}

// Summarizer builds a Summary for one session given the corpus of all
// sessions (used for document frequency).
type Summarizer interface {
	Summarize(sessionID string, corpus map[string][]parser.AntigravityMessage, keywords, highlights int) Summary
}

// Baseline is the built-in TF-IDF summarizer.
type Baseline struct{}

// Method is the value Baseline reports in Summary.Method.
const Method = "tfidf-baseline"

var stopwords = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`a about above after again against all am an and any are as at be because been
		before being below between both but by can could did do does doing down during each few for from further had
		has have having he her here hers herself him himself his how i if in into is it its itself just let me more most
		my myself no nor not now of off on once only or other our ours ourselves out over own same she should so some
		such than that the their theirs them themselves then there these they this those through to too under until up
		very was we were what when where which while who whom why will with would you your yours yourself yourselves
		also yes ok okay like get got one two use using used can't don't it's i'm we're that's there's what's let's
		able want need think know see make sure thing things way well really just maybe much many lot`) {
		stopwords[w] = true
	}
}

// Tokens returns the content words of text: lower-cased, at least three
// characters, not a stopword, not a number, with #/@ mentions excluded
// (they are reported separately as tags and participants).
func Tokens(text string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(text, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\'' || r == '#' || r == '@' || r == '_' || r == '-')
	}) {
		if strings.HasPrefix(f, "#") || strings.HasPrefix(f, "@") {
			continue
		}
		w := strings.Trim(strings.ToLower(f), "'-_")
		if len([]rune(w)) < 3 || stopwords[w] || !strings.ContainsFunc(w, unicode.IsLetter) {
			continue
		}
		out = append(out, w)
	}
	return out
}

// Summarize implements Summarizer.
func (Baseline) Summarize(sessionID string, corpus map[string][]parser.AntigravityMessage, keywords, highlights int) Summary {
	msgs := corpus[sessionID]
	s := Summary{SessionID: sessionID, Messages: len(msgs), Method: Method,
		Participants: []Participant{}, Tags: []string{}, Keywords: []Keyword{}, Highlights: []parser.AntigravityMessage{}}
	if len(msgs) == 0 {
		return s
	}

	// Document frequency: in how many sessions does each term appear.
	df := map[string]int{}
	for _, sessMsgs := range corpus {
		seen := map[string]bool{}
		for _, m := range sessMsgs {
			for _, t := range Tokens(m.Message) {
				if !seen[t] {
					seen[t] = true
					df[t]++
				}
			}
		}
	}
	nDocs := float64(len(corpus))
	idf := func(t string) float64 { return math.Log((1+nDocs)/(1+float64(df[t]))) + 1 }

	tf := map[string]int{}
	users := map[string]int{}
	var userOrder []string
	tagSeen := map[string]bool{}
	s.Start, s.End = msgs[0].Timestamp, msgs[0].Timestamp
	for _, m := range msgs {
		if m.Timestamp.Before(s.Start) {
			s.Start = m.Timestamp
		}
		if m.Timestamp.After(s.End) {
			s.End = m.Timestamp
		}
		if u := strings.TrimSpace(m.User); u != "" {
			if users[u] == 0 {
				userOrder = append(userOrder, u)
			}
			users[u]++
		}
		for _, men := range parser.ExtractMentions(m.Message) {
			if men.Kind == parser.MentionTag && !tagSeen[men.Name] {
				tagSeen[men.Name] = true
				s.Tags = append(s.Tags, men.Name)
			}
		}
		for _, t := range Tokens(m.Message) {
			tf[t]++
		}
	}

	for _, u := range userOrder {
		s.Participants = append(s.Participants, Participant{User: u, Messages: users[u]})
	}
	sort.SliceStable(s.Participants, func(i, j int) bool { return s.Participants[i].Messages > s.Participants[j].Messages })

	score := map[string]float64{}
	for t, c := range tf {
		score[t] = float64(c) * idf(t)
	}
	for t, c := range tf {
		s.Keywords = append(s.Keywords, Keyword{Term: t, Count: c, Score: math.Round(score[t]*1000) / 1000})
	}
	sort.Slice(s.Keywords, func(i, j int) bool {
		if s.Keywords[i].Score != s.Keywords[j].Score {
			return s.Keywords[i].Score > s.Keywords[j].Score
		}
		return s.Keywords[i].Term < s.Keywords[j].Term
	})
	if keywords >= 0 && len(s.Keywords) > keywords {
		s.Keywords = s.Keywords[:keywords]
	}

	// Extractive highlights: messages ranked by the mean score of their
	// distinct tokens (so long messages are not favoured by length alone),
	// returned in chronological order.
	type ranked struct {
		idx   int
		score float64
	}
	var rs []ranked
	for i, m := range msgs {
		toks := Tokens(m.Message)
		if len(toks) == 0 {
			continue
		}
		seen := map[string]bool{}
		total := 0.0
		for _, t := range toks {
			if !seen[t] {
				seen[t] = true
				total += score[t]
			}
		}
		rs = append(rs, ranked{i, total / math.Sqrt(float64(len(seen)))})
	}
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].score > rs[j].score })
	if highlights >= 0 && len(rs) > highlights {
		rs = rs[:highlights]
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].idx < rs[j].idx })
	for _, r := range rs {
		s.Highlights = append(s.Highlights, msgs[r.idx])
	}
	return s
}
