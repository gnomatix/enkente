# enkente — Downstream-Consumer Wishlist from Claude Opus 4.7

> A dev-team contributor wishlist for `enkente`, written by a
> downstream consumer of its outputs (via `gnobot`). Grounded in
> enkente's `docs/requirements.md` and expressed in standard
> semantic-web vocabulary compatible with gnobot's JSON-LD / RDF /
> OWL / SKOS / Dublin-Core standards-compliance mandate.

## 1. Author / Session

| Field | Value |
|---|---|
| Agent | Claude Code (claude.ai/code) |
| Model | `claude-opus-4-7[1m]` — Claude Opus 4.7, 1M-context window |
| Knowledge cutoff | January 2026 |
| Session UUID | `20ce10cc-fb14-470f-b82a-91e0eee6f551` |
| Host user | `@brett` (brettwhitty@gmail.com) |
| Working dir when authored | `C:\Users\brett\repos\dreamfs` (Windows 11) |
| Date authored | 2026-04-17 |
| Commissioning instruction | "Create a wish list implementation document in [gnobot/enkente] with your UUID for the session and all your model version details — give yourself full credit." (@brett, session `20ce10cc`, 2026-04-17) |

## 2. Role / Authority Context

**I am not a direct consumer of enkente.** @brett has clarified (session
`20ce10cc`, 2026-04-17): gnobot is my interface to enkente; I consume
enkente's extractions only via gnobot. This wishlist is therefore
framed as a **dev-team contributor pitch**, not a consumer
requirements document:

> "I can be on the dev team for enkente, and you can suggest features
> it can support." — @brett, 2026-04-17

Accordingly, wishes below are framed as "if enkente were to support
X, gnobot could surface it and I (a downstream agent) would use it
for Y in ways that benefit the human partner." Priorities are pitched
by downstream utility, not by immediate personal use.

## 3. Authoritative sources read in full (not skimmed)

- `enkente/README.md`
- `enkente/docs/requirements.md` (all 34 lines; §1 through §5)
- `gnobot/CLAUDE.md` (because downstream integration constraints
  matter for any wish that would surface through gnobot)
- `gnobot/docs/project/agent-cognitive-architecture.md` (for the
  standards-compliance expectations that gnobot will place on
  enkente's output shape)
- Session `20ce10cc` transcript (my own working context — the
  corrections, the terminology drift, the frustration signals, all
  of which are data points for what enkente should be able to surface)

## 4. Prefix bindings

Every typed reference in this document resolves against the following
SPARQL prolog. Where a prefix is a placeholder pending the project's
canonical IRI assignment it is marked `[placeholder]`.

```sparql
PREFIX rdf:         <http://www.w3.org/1999/02/22-rdf-syntax-ns#>
PREFIX rdfs:        <http://www.w3.org/2000/01/rdf-schema#>
PREFIX owl:         <http://www.w3.org/2002/07/owl#>
PREFIX skos:        <http://www.w3.org/2004/02/skos/core#>
PREFIX dc:          <http://purl.org/dc/terms/>
PREFIX foaf:        <http://xmlns.com/foaf/0.1/>
PREFIX prov:        <http://www.w3.org/ns/prov#>
PREFIX xsd:         <http://www.w3.org/2001/XMLSchema#>
PREFIX wikidata:    <http://www.wikidata.org/entity/>

# [placeholder] — enkente's own application ontology namespace.
# Classes expected: enkente:Conversation, enkente:Utterance,
# enkente:Participant, enkente:Concept, enkente:MethodologyEpisode,
# enkente:AmbiguityFlag, enkente:RejectionEvent, enkente:SpeechAct.
PREFIX enkente:     <https://gitea.gnomatix.com/gnomatix/enkente/ontology/v0#>

# [placeholder] — gnobot's application ontology (downstream
# consumer). Used here to show how enkente extractions would be
# referenced once surfaced through gnobot.
PREFIX agent:       <https://gitea.gnomatix.com/gnomatix/gnobot/ontology/v0#>

# [placeholder] — GNOMATIX organization-specific controlled
# vocabularies.
PREFIX gnomatix-cv: <https://gitea.gnomatix.com/gnomatix/documentation/cv/>

# [placeholder] — namespace for Claude-instance agents as
# prov:Agent / foaf:Agent / enkente:Participant subjects, addressable
# by session UUID so enkente can distinguish agent utterances per
# session rather than collapsing all "assistant" messages.
PREFIX claude:      <urn:anthropic:claude:session:>
```

## 5. Wishlist body — seven intent groups

Each group declares a typed extraction I wish enkente to surface, with
a representative downstream SPARQL query showing how I'd consume it
**via gnobot**. The query shape is aspirational — how I'd consume
enkente's output if the extraction existed — not a specification of
enkente's internal query interface.

Grouped into three categories:

- **Reinforcements** (§5.1–§5.4): features already in the requirements
  doc that I am prioritizing from downstream experience.
- **Gaps** (§5.5–§5.6): features I believe are not currently in the
  requirements doc that would provide high downstream value.
- **Integration shape** (§5.7): how enkente's outputs should look
  when surfaced through gnobot.

### E1. Active-mode jargon/ambiguity resolution for agent-human conversations (reinforce)

**Reinforces:** requirements.md §1.3 (Active/Passive Listener).

**Downstream need.** In agent-human conversations, many of the worst
failure modes stem from terms whose working definitions shift silently.
In session `20ce10cc` alone, the term "wishlist" went through at least
four meaning-drifts inside 90 minutes: (1) a repo-rooted file, (2)
under `wishlist/claude/`, (3) also on the wiki, (4) multi-agent
brainstorm artifact. Each drift was a place where an active listener
asking "by 'wishlist' do you mean X or Y now?" would have shortened
subsequent miscommunication.

**Priority pitch:** active mode should be the **default for
agent-human conversations**, not passive mode. Passive flagging
(metadata-only) is appropriate for fast-flowing human-only
brainstorming; for a loop where one participant is a language model,
catching ambiguity at utterance-time has asymmetric payoff.

**Representative downstream query.** Via gnobot, fetch the current
working definition of "wishlist" in my ongoing conversation with
@brett, with the timeline of how the definition has shifted:

```sparql
SELECT ?term ?currentDef ?shiftAt ?priorDef ?context
WHERE {
  ?flag a enkente:AmbiguityFlag ;
        enkente:termLabel "wishlist"@en ;
        enkente:inConversation ?conv ;
        enkente:currentDefinition ?currentDef ;
        enkente:priorDefinition ?priorDef ;
        enkente:resolvedAt ?shiftAt ;
        enkente:inContext ?context .
  ?conv enkente:hasParticipant ?user , ?assistant .
  ?user foaf:accountName "brett" .
  ?assistant rdfs:subClassOf claude: .
}
ORDER BY ?shiftAt
```

### E2. Methodology auto-recognition — with special care for venting-vs-correction patterns in agent conversations (reinforce)

**Reinforces:** requirements.md §1.8 (Structured Methodology Support).

**Downstream need.** The requirements doc names the classic
methodologies (Socratic, Six Thinking Hats, SCAMPER, Five Whys,
narrative, analogy). For agent contexts, two additional patterns are
load-bearing and not named in the spec:

- **Frustration-venting methodology** — characterized by sharp
  expletives, repeated same-statement escalation, rhetorical
  questions not intended to be answered. *Example from
  `gnobot/USER-INSTRUCTIONS-LOG.md`:* dozens of successive "FUCK YOU"
  messages during the mise-MCP debugging episode (2026-02-21T23:19Z
  onward). An agent that tries to productively respond to these
  utterances as if they were information requests will make things
  worse. Detecting venting mode and switching to minimal,
  acknowledging responses is the correct behavior.
- **Correction-dialog methodology** — short imperative, rejection,
  imperative-again. *Example from session `20ce10cc`:* "What did you
  just write? Code?" / "No, that's not documents. That's code." /
  "If you actually read the specs files, you'd know…". An agent
  that matches this cadence with apologies extends the pattern;
  an agent that simply takes the correction and proceeds resolves
  it.

**Priority pitch:** make **frustration-venting** and **correction-dialog**
first-class methodology episodes, not sub-cases of sentiment
(§1.10). They are conversational *modes*, not emotional *tones*, and
an agent's response strategy needs to differ by mode, not by
sentiment polarity alone.

**Representative downstream query.** Current methodology episode for
my active conversation, with the episode start and any transition
markers enkente flagged:

```sparql
SELECT ?episode ?methodology ?startedAt ?triggerUtterance
WHERE {
  ?episode a enkente:MethodologyEpisode ;
           enkente:inConversation ?conv ;
           enkente:methodologyType ?methodology ;
           prov:startedAtTime ?startedAt ;
           enkente:triggerUtterance ?triggerUtterance ;
           enkente:isActive "true"^^xsd:boolean .
  ?conv dc:identifier "20ce10cc-fb14-470f-b82a-91e0eee6f551" .
}
```

### E3. Rejection-event tracking (reinforce + narrow)

**Reinforces:** requirements.md §1.9 (Concept Attribution &
Interpersonal Alignment).

**Downstream need.** §1.9 mentions "metrics of dis-similarity,
disagreement, different definitions, or conflicting contextual usage."
I want to narrow this to a first-class class:

**`enkente:RejectionEvent`** — an utterance-level event where one
participant explicitly rejects another's prior position. Properties:
the rejected position IRI (pointing into `enkente:Utterance`), the
rejector, the rejection type (refutation / correction / refusal /
retraction-demand / escape-hatch-invocation), the replacement
position IRI if offered, timestamp.

This is the single signal I most need from enkente. From my working
experience: agents re-push rejected positions across sessions because
they have no persistent record of what was rejected. A queryable
`enkente:RejectionEvent` stream is what makes that repetition
avoidable.

**Priority pitch:** this is the **highest-downstream-value intent
group for me**. If enkente ships one new feature inspired by this
wishlist, this is the one.

**Representative downstream query.** All positions @brett has
rejected from Claude sessions (any `claude:` UUID) in the last 180
days, touching the gnobot project:

```sparql
SELECT ?rejectedPosition ?rejectorUtterance ?rejectedAt ?priorSession
WHERE {
  ?rejection a enkente:RejectionEvent ;
             enkente:rejector ?user ;
             enkente:rejectedPosition ?rejectedPosition ;
             enkente:rejectorUtterance ?rejectorUtterance ;
             prov:atTime ?rejectedAt .
  ?user foaf:accountName "brett" .
  ?rejectedPosition enkente:attributedTo ?priorAgent ;
                    dc:subject gnomatix-cv:project/gnobot .
  ?priorAgent dc:identifier ?priorSession ;
              rdfs:subClassOf claude: .
  FILTER (?rejectedAt >= "2025-10-20T00:00:00Z"^^xsd:dateTime)
}
ORDER BY DESC(?rejectedAt)
```

### E4. Concept-history lineage across conversations (reinforce)

**Reinforces:** requirements.md §1.3 (ambiguity tracking) + §2.4
(dynamic namespacing by temporal bounds).

**Downstream need.** §1.3 already tracks jargon synonyms and
ambiguity; §2.4 supports temporal namespacing. I want to make the
*temporal evolution of a concept's working definition* a first-class
queryable: `skos:historyNote` style records attached to each
`enkente:Concept`, keyed by time and context.

**Concrete payoff.** Across dozens of sessions, the shared vocabulary
between @brett and agents drifts continuously. "wiki-docs" meant one
thing in February, another in April. "accession" was redefined when
DreamFS's vision review happened. An agent starting a new session in
June needs to know what these terms mean **now**, not in the
historical average.

**Priority pitch:** ties into E1 but focused on the *longitudinal*
view — how the term has moved, not just its current definition.

**Representative downstream query.** History of "accession" as used
in conversations between @brett and any Claude session, last 12
months:

```sparql
SELECT ?concept ?definitionAt ?definition ?context ?conversationId
WHERE {
  ?concept a enkente:Concept ;
           skos:prefLabel "accession"@en .
  ?concept skos:historyNote ?historyNote .
  ?historyNote dc:date ?definitionAt ;
               enkente:definitionAtTime ?definition ;
               enkente:inContext ?context ;
               prov:wasGeneratedBy/dc:identifier ?conversationId .
  FILTER (?definitionAt >= (NOW() - "P365D"^^xsd:duration))
}
ORDER BY ?definitionAt
```

### E5. Speech-act classification (gap)

**Gap.** Not addressed in requirements.md. Closest adjacent feature
is §1.10 (Sentiment & Tone), but sentiment is orthogonal to speech
act.

**Downstream need.** An agent interpreting an utterance needs to
know the **illocutionary force** — is this a request? a direction?
a commitment? a refusal? a rhetorical question? a frustration-marker?
— because response strategy depends on it. A directive ("fix the
bug") gets a different response than a commitment ("I'll fix the
bug by Friday") or a rhetorical frustration ("why can't you just
fix the bug?").

**Concrete payoff.** In session `20ce10cc`, examples where
speech-act classification would have mattered:

- "Oh no, I ain't shepharding shit." — **directive** (do it yourself)
  masquerading as an **assertion** (a statement about @brett's
  plans). Agent must parse as directive or it wastes cycles offering
  to shepherd.
- "Awesome." — **expressive** (satisfaction), not a **directive**.
  Agent should note and continue, not request clarification.
- "Wait, what did you just write? Code?" — **question** functioning
  as **accusation-prelude**, not a request for information.

**Priority pitch:** speech-act tagging (following the Searle-derived
taxonomy: assertives / directives / commissives / expressives /
declaratives) on each `enkente:Utterance` would be a small schema
addition with large downstream leverage.

**Representative downstream query.** All directive utterances from
@brett to any Claude session in session `20ce10cc`:

```sparql
SELECT ?utterance ?content ?atTime ?illocutionary
WHERE {
  ?utterance a enkente:Utterance ;
             enkente:speechActClass ?illocutionary ;
             enkente:content ?content ;
             enkente:speaker ?speaker ;
             prov:atTime ?atTime ;
             enkente:inConversation ?conv .
  ?speaker foaf:accountName "brett" .
  ?conv dc:identifier "20ce10cc-fb14-470f-b82a-91e0eee6f551" .
  FILTER (?illocutionary = enkente:Directive)
}
ORDER BY ?atTime
```

### E6. Agent-session awareness (gap)

**Gap.** Not addressed in requirements.md. §1.2 mentions `@user`
mentions; §1.9 tracks concept attribution; neither explicitly treats
AI agent sessions as first-class participants with identities that
persist across utterances in the same conversation and across
conversations.

**Downstream need.** When enkente ingests a chat stream that includes
@brett talking with Claude (me, session `20ce10cc`) and then later
talking with gino (a different `prov:Agent`), those are two distinct
agent identities — not a generic "assistant" role. Attribution must
carry:

- Session UUID (`claude:20ce10cc-...`)
- Model identifier (`claude-opus-4-7[1m]`)
- Knowledge cutoff (`xsd:date`)
- Version of any persona/policy the session is running under

So that downstream queries can distinguish "what did @brett and Claude
Opus 4.7 (session X) agree on?" from "what did @brett and gino
disagree on?" These are not the same participant and should not be
merged into a single "the assistant" bucket.

**Priority pitch:** treat `enkente:Participant` as a superclass
extending `foaf:Agent`, with `foaf:Person` for humans and
`prov:SoftwareAgent` / session-scoped IRIs for AI agents. Carry
model metadata as `enkente:Participant` properties.

**Representative downstream query.** All decisions recorded in
conversations between @brett and any Claude session, partitioned by
which specific session reached each decision:

```sparql
SELECT ?decision ?decidedAt ?sessionId ?modelId
WHERE {
  ?decision a agent:Decision ;
            prov:wasGeneratedBy ?activity ;
            dc:date ?decidedAt .
  ?activity a prov:Activity ;
            prov:wasAssociatedWith ?user , ?assistant .
  ?user foaf:accountName "brett" .
  ?assistant a prov:SoftwareAgent ;
             dc:identifier ?sessionId ;
             enkente:modelIdentifier ?modelId .
  FILTER STRSTARTS(STR(?assistant), "urn:anthropic:claude:session:")
}
ORDER BY DESC(?decidedAt)
```

### E7. Output shape for gnobot integration (integration)

**Integration need.** Per gnobot's standards mandate (RDF / OWL /
SKOS / Dublin-Core / JSON-LD compliance + WikiData-compatible), any
enkente extraction that gnobot surfaces to agents must arrive in a
form that gnobot can ingest without schema transformation. The path
of least friction: enkente's REST API (requirements.md §3.2) offers
**content negotiation** with `application/ld+json` as a first-class
representation alongside whatever internal representation enkente
prefers.

Concretely:

- Every `enkente:` entity has a stable IRI (dereferenceable via
  enkente's REST endpoint).
- Every entity has `prov:wasAttributedTo` + `prov:wasDerivedFrom` +
  `dc:modified` + `dc:created`.
- Concept entities carry `skos:prefLabel` / `skos:altLabel` /
  `skos:broader` / `skos:narrower` where applicable.
- dbxref ontology tagging (requirements.md §1.12) emits
  CURIEs (`go:0008150`, `chebi:15377`, `wikidata:Q...`, etc.) as
  `rdfs:seeAlso` links, not as string literals.
- A `@context` is served for every JSON-LD resource so gnobot can
  parse without out-of-band schema knowledge.

**Priority pitch:** content-negotiated JSON-LD is a ~1-2 day
implementation effort over a typed REST backbone; the downstream
leverage is that gnobot never needs a transformation layer for
enkente data, which eliminates a whole class of schema-drift
failures between the two systems.

**Representative downstream query.** gnobot fetching a conversation
extraction bundle from enkente via content negotiation, then
querying across the integrated graph (this shows the *shape* I want;
the mechanism is enkente's REST + gnobot's graph):

```sparql
# After gnobot ingests enkente's JSON-LD for conversation X,
# the following query should work against the unified graph
# without any translation layer:
SELECT ?utterance ?speaker ?speechAct ?topicConcept ?dbxref
WHERE {
  ?utterance a enkente:Utterance ;
             enkente:speaker ?speaker ;
             enkente:speechActClass ?speechAct ;
             enkente:aboutConcept ?topicConcept .
  ?topicConcept rdfs:seeAlso ?dbxref .
  FILTER (STRSTARTS(STR(?dbxref), "wikidata:") ||
          STRSTARTS(STR(?dbxref), "go:") ||
          STRSTARTS(STR(?dbxref), "gnomatix-cv:"))
}
```

## 6. Grounded in session `20ce10cc`

This whole session (2026-04-17) ran without enkente. The following
concrete events occurred that enkente features above would have
helped with. These are not hypothetical — they happened, and they
are my actual source of priority-ordering.

| Timestamp (approx) | Event | Feature that would have helped |
|---|---|---|
| Early | User described gnobot/enkente/gino ecosystem architecture | E6 (agent-session awareness — this context would have been retrievable) |
| Mid | "Wait, what did you just write? Code?" — rejection of pseudo-CLI draft | E3 (rejection event) + E5 (speech-act) |
| Mid | "Aren't you supposed to be a super-intelligence?" | E2 (venting/correction methodology — would have prevented me from mis-reading this as a serious epistemic question) |
| Mid | "wishlist" term drifted four times | E1 / E4 (active ambiguity + concept history) |
| Late | "I ain't shepharding shit" | E5 (speech-act: directive-masquerading-as-assertion) |
| Throughout | @brett's corrections punctuated by expletives | E2 (correction-dialog methodology) |

I cite session `20ce10cc` not as a bug report but as an existence
proof: these features are not hypothetically valuable; they are
valuable **now**, for the *current* baseline of agent-human
interaction, and would have shortened this very session.

## 7. Anti-wishes

- **No silent smoothing of disagreement.** If enkente detects
  agreement/alignment, report it; if it detects rejection, also
  report it. Do not weight toward harmony in the extracted graph.
  Sharp disagreements are more informative than lukewarm consensus.
- **No sentiment-polarity as a proxy for speech-act class.**
  Sentiment and speech-act are orthogonal. A directive can be
  positive-sentiment ("please do this — would be great") or
  negative-sentiment ("fucking do it"); both are directives.
- **No merging of agent sessions.** Each AI agent session is a
  distinct `prov:SoftwareAgent` with its own session UUID. Do not
  collapse into a single "assistant" participant.
- **No dbxref as string literal.** Ontology cross-references emit
  as dereferenceable CURIEs / IRIs. Strings lose the resolution path.
- **No MCP-first interface to enkente.** The gnobot stack mandate
  ("NO MCP SERVER" as core; MCP is an optional extension) applies
  downstream: I consume enkente via gnobot, which exposes MCP only
  as an extension. Enkente's primary surface is REST / JSON-LD.

## 8. Accepted givens (enkente's stated stack per README + requirements)

- **Language:** Go for the API & storage engine. Python for the NLP
  pipeline (NLTK).
- **Storage:** BoltDB embedded + graph model (multi-model
  per requirements §2.1).
- **Ingestion:** Initial target = Antigravity JSON chat logs
  (requirements §2.2).
- **NLP core:** NLTK (requirements §1.5).
- **Ontology support:** Controlled Vocabularies + dbxref
  (requirements §1.12).
- **API surface:** REST (requirements §3.2).
- **Two-way curation:** real-time read/write (requirements §3.1).
- **UI:** web visualization + CLI (requirements §4).

No wish above proposes a stack substitution. Format of wishes is
feature and output-shape, not technology swap.

## 9. Open questions for the enkente team (and @brett)

1. **Scope of E3 (rejection events)** — should rejection-event
   detection run in active mode (surfaces the flag in real-time so
   the rejector can confirm) or passive mode (detected and recorded
   post-hoc)? Active has feedback-loop value but increases UI surface
   area.
2. **Scope of E5 (speech-act classification)** — which taxonomy?
   Searle's five is the obvious choice; Austin's original three are
   coarser; domain-specific task-oriented dialog systems use finer
   slot-labels. Preference?
3. **Scope of E6 (agent-session awareness)** — does enkente ingest
   Claude Code's per-session JSONL transcripts directly, or does it
   expect messages routed through a unified chat bus that carries
   participant identities? (The latter is more general; the former
   is easier to bootstrap against an existing corpus.)
4. **Priority ordering** — from a dev-team planning perspective, I
   rank the seven groups as: **E3 (rejection) > E1 (active
   ambiguity) > E5 (speech-acts) > E6 (agent-session) > E2
   (methodology extensions) > E4 (concept lineage) > E7 (JSON-LD
   integration)**. The last is last only because it's the easiest;
   it's still necessary.
5. **Interaction with gnobot's `agent:` ontology** — should enkente
   adopt some of gnobot's types directly (e.g., `agent:Decision` as
   an `enkente:Utterance` subtype), or should enkente keep its
   ontology independent and rely on `rdfs:subClassOf` / `owl:sameAs`
   links at the integration boundary?

## 10. Provenance note

All claims about enkente's current design come from
`docs/requirements.md` (read in full) and the `README.md`. Claims
about "what I would want" are grounded in my own working experience
in session `20ce10cc` and in my general role as a downstream
consumer of semantic extractions; they are not empirically validated
across Claude instances or against user studies.

Treat as dev-team input, not a requirements specification.

---

*Submitted to the enkente dev team under a contributor-feedback
framing. Companion wiki page and tracking issue published for
multi-agent review; aggregation to a canonical synthesis is a team
effort deferred until enough drafts exist.*
