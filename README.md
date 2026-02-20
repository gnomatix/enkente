# enkente

!["enkente"](assets/images/enkente-o-image.png)

**Real-time Multi-faceted "Mind-Mapping" Datastore for Collaborative Storytelling & Brainstorming**

Let's all do "**The Weave**".

## Overview

**enkente** is a real-time collaborative platform designed to ingest multi-user chat streams and continuously encode semantic and contextual relationships. By leveraging structural Natural Language Processing (NLP) techniques, **enkente** transforms unstructured human conversations into rich, multi-dimensional, queryable mind-maps.

Whether the group is engaging in formal brainstorming methodologies (like Six Thinking Hats, SCAMPER, or Five Whys), evaluating interpersonal alignment and disagreement, or telling abstract narrative stories, **enkente** actively tracks, parses, and connects ideas in real time. 

## Features

* **Real-time Semantic Extraction:** Ingests live conversations and encodes them into graph-centric models for real-time visualization and complex relationship mapping.
* **Auto-Recognition of Methodologies:** Automatically recognizes structures like the Socratic Method or narrative storytelling to reflexively optimize the internal semantic processing engine.
* **Jargon & Ambiguity Resolution:** Interactively highlights and seeks clarity on ambiguous topics, reducing contextual uncertainty across the group.
* **Concept Attribution:** Tracks the provenance of concepts and provides metrics on team alignment, agreement, or contextual disagreement.
* **User-Space & Embedded:** Built with a true zero-install development philosophy, utilizing self-contained tools (`mise`) and embedded databases (`BoltDB`).

## Technology Stack

The project relies on a deeply unified split-architecture:
* **API & Storage Engine (Go):** High-performance ingestion API and data management using an embedded BoltDB instance.
* **NLP Pipeline (Python):** Python-powered text analysis employing the widely-used NLTK libraries for tokenization, Named Entity Recognition, and ontology tagging.

<img src="https://outrage.dataglut.org/assets/badges/ioa-aware-badge-provisional.svg" width="300px">

## Documentation

For an in-depth dive into the technical capabilities, architecture, and requirements, please refer to the `docs/` directory:
* [System Requirements Spec](docs/requirements.md)
* [Design Docs & Summaries](docs/design-docs/system-specs-summary.md)


## License: Business Source License (BSL)
Copyright © 2026 GNOMATIX. All rights reserved.

This software is licensed under the Business Source License (BSL) version 1.1. Usage of this software is subject to the terms of the BSL. See the LICENSE file for details.

!["KILLBOTS ACTIVATE!" ](assets/images/gnomatix-killbots-activate-xs.png "KILLBOTS ACTIVATE!")

!["GNOMATIX" ](assets/images/gnomatix-new-xs.png "GNOMATIX")