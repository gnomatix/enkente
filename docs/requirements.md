# enkente: System User Requirements Specifications

## 1. Natural Language Processing (NLP) & Semantic Encoding
*   **Real-time Chat Processing:** System must intercept or ingest multi-user chat streams in real-time.
*   **Explicit Contextual Referencing & Knowledge Accumulation:** The system must recognize explicit explicit mentions (e.g., `#graph_databases` or `@user`) to instantly pull into context everything that has been accumulated about that entity—including "stories", anecdotes, and specific attributes.
*   **Active/Passive Listener (Jargon & Ambiguity Resolution):** The system must identify domain-specific "jargon" and track synonyms. It must recognize "over-loaded" terms and flag ambiguities. This listener behavior must be configurable to be either passive (flagging in the UI/metadata) or active (interrupting the chat in real-time, e.g., "Hey! Listener here, what did you mean by 'database'?"). When returning a response to a processed prompt, it must include metadata about uncertain or overloaded terms.
*   **Interactive Phrasing Assessment:** Users must be able to interactively test potential statements with the system (e.g., in a "draft" or "preview" mode). The system should evaluate alternative phrasings and provide feedback on whether it increases or reduces "uncertainty", "clarity", or "confusion" before the message is fully committed. Furthermore, other users can interact with the system's "listener" prompts to collaboratively clarify another person's ambiguous phrasing.
*   **NLTK Core:** Use standard, mature NLP libraries (specifically NLTK) for tokenization, part-of-speech tagging, and named entity recognition.
*   **Semantic Encoding:** Extract and encode contextual meaning, intent, and relationships from the text to establish the system's "understanding" of the story or brainstorm.
*   **Topic Extraction & Clustering:** The system must identify emerging themes and sub-topics from the chat stream, automatically clustering related messages to help structure brainstorming sessions.
*   **Structured Methodology Support (Auto-Recognition):** The semantic engine must be capable of *automatically recognizing* when established methodologies or conversational patterns are being used (e.g., *Socratic Method*, *Six Thinking Hats*, *SCAMPER*, *Five Whys*, *Brainwriting*, narrative storytelling, or analogies). Identifying these patterns in real-time should be used to optimize the engine's internal semantic processing performance and structurally route ideas appropriately based on the recognized phase or pattern.
*   **Concept Attribution & Interpersonal Alignment:** The system must track the provenance of concepts (who introduced them) and analyze interpersonal metrics. This includes identifying "agreement" or "alignment" between participants on specific ideas, as well as highlighting metrics of dis-similarity, disagreement, different definitions, or conflicting contextual usage.
*   **Sentiment & Tone Context:** Track the sentiment (positive, negative, neutral) and tone of the discussion to attach emotional context to the encoded relationships (e.g., separating "concerns" from "enthusiastic support").
*   **Automated Summarization:** Generate real-time or on-demand summaries of specific conceptual branches or entire brainstorming sessions based on the accumulated NLP data.
*   **Ontology Tagging (dbxref):** Integrate Controlled Vocabularies (CVs) and Ontologies to support tag-based entity recognition. The system must support loading domain-specific ontologies (e.g., standard subject area ontologies in biological research) to enrich entities.

## 2. Data Storage & Modeling
*   **Multi-Model Datastore:** Leverage mature open-source NoSQL and Graph databases to store both the document-centric chat logs and the graph-centric semantic relationships between concepts.
*   **Initial Data Ingestion (Testing):** For development and testing, the system will hook into and ingest Antigravity JSON chat logs, specifically extracting the text content of chat members' messages (along with useful metadata like timestamps).
*   **Rich Data-Encoding:** The schema must support multi-level conceptualization, allowing data to be queried and shaped for completely different visualization paradigms (hierarchical trees, force-directed graphs, chronological timelines).
*   **Dynamic Namespacing:** Implement robust namespacing to isolate and contextualize data by session, individual user, subject matter, overarching project, or temporal bounds.

## 3. Real-Time Interaction & API Layer
*   **Two-Way Curation:** Ensure the processing pipeline allows for real-time read-write operations, where users or external processes can directly curate and correct the parsed concepts, instantly updating the underlying datastore.
*   **REST API:** Expose comprehensive RESTful endpoints for CRUD operations and complex graph queries on all levels of entities and concepts.

## 4. User Interfaces & Access Tooling
*   **Web Visualization:** Provide a real-time, web-based UI for users to visualize the "mind-map", intuitively curate concepts, and interact with the data stream as it is processed.
*   **CLI Tools:** Develop comprehensive command-line interfaces for all aspects of entity management, providing power users administrators with direct access to the datastore.

## 5. Interoperability & Data Portability
*   **Domain-Specific I/O:** Support import and export of data in standard, domain-specific formats.
*   **Raw Data Access:** Ensure the datastore can export to other NoSQL paradigms or flat text for external analysis or migration.
