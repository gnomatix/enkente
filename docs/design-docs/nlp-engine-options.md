# NLP engine: options for the decision (enk-bbw)

Decision needed: whether summaries, topic clustering and entity recognition
stay on the Go baseline or move to the NLTK pipeline named in
`docs/requirements.md` §1 ("NLTK Core").

## Current state (in this repo)

- `pkg/summary` implements `summary.Baseline`. Keywords are scored by TF-IDF
  against all stored sessions, highlights are extracted, and topics are
  formed from tags and keyword seeds. API responses report it as
  `"method": "tfidf-baseline"`.
- The `summary.Summarizer` interface and the JSON shapes of
  `GET /sessions/{id}/summary` and `GET /sessions/{id}/topics` are
  independent of the engine. A different engine can sit behind them without
  changing the API.
- `pkg/listener` (jargon and ambiguity) uses a JSON lexicon. It does no
  tokenization beyond word splitting and no part-of-speech tagging.
- There is no Python code in the repo. The README's "NLP Pipeline (Python)"
  describes a planned component.

## What the requirements name that the baseline does not do

From `docs/requirements.md` §1:
- part-of-speech tagging and named-entity recognition;
- sentiment and tone;
- automatic recognition of methodologies such as the Socratic method and
  SCAMPER;
- ontology tagging.

## Options

1. **Keep the Go baseline and extend it in Go.** The build and deployment
   stay single-language. The NLP features above would need Go
   implementations or Go libraries. Which libraries are suitable has not
   been evaluated.
2. **Add an NLTK service in Python.** The Go server calls it over HTTP or
   through a subprocess, behind `summary.Summarizer` and a parallel
   interface for named-entity and part-of-speech results. This matches the
   requirements as written, and adds a second runtime to build, test and
   deploy (`mise` can pin the Python version).
3. **Both.** The Go baseline is the default, and the NLTK engine is used
   when it is configured, selected per request or at server start. The
   `method` field in the responses shows which engine answered.

## Not decided here

Which option to take, and whether new Python code is wanted in this
project, is the owner's decision.
