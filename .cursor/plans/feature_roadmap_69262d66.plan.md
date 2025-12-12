---
name: Feature Roadmap
overview: A comprehensive roadmap for enhancing HelloWorld AI with MCP integration, specialized agents, improved APIs, and system improvements. This document complements ARCHITECTURE_REVIEW.md by providing a phased implementation plan without duplicating architectural details.
todos: []
---

# Feature Roadmap: HelloWorld AI Enhancement Plan

This document provides a phased roadmap for enhancing the HelloWorld AI system with new features, improved capabilities, and specialized agents. It complements `ARCHITECTURE_REVIEW.md` by focusing on feature development rather than architectural improvements.

**Note:** This roadmap references recommendations from `ARCHITECTURE_REVIEW.md` (e.g., "See ARCHITECTURE_REVIEW.md Section 3.1 for performance optimizations") but does not duplicate those details here.

---

## Current System State

The system currently provides:

- RAG-powered Q&A over indexed markdown notes (`/api/v1/ask`)
- Health check endpoint (`/api/health`)
- Re-indexing endpoint (`/api/index`)
- Note file serving (`/notes/{vault}/*`)
- Basic web UI for RAG queries
- Two-vault support (personal + work)
- Hash-based change detection for efficient re-indexing

**See:** `ARCHITECTURE_REVIEW.md` for detailed evaluation of current architecture, performance characteristics, and quality mechanisms.

---

## Phase 1: Foundation Improvements

**Goal:** Address critical infrastructure gaps and improve system reliability before adding new features.

### 1.1 Critical Infrastructure Fixes

**Priority:** High
**Dependencies:** None
**Reference:** ARCHITECTURE_REVIEW.md Sections 1.2, 5.1

**Tasks:**

1. Implement HTTP client timeouts and connection pooling (ARCHITECTURE_REVIEW.md Section 1.2)
2. Add graceful shutdown handling (ARCHITECTURE_REVIEW.md Section 1.2)
3. Make indexing asynchronous (ARCHITECTURE_REVIEW.md Section 3.1)
4. Add health check endpoint improvements (ARCHITECTURE_REVIEW.md Section 5.1)
5. Implement structured error types (ARCHITECTURE_REVIEW.md Section 1.2)

**Deliverables:**

- HTTP clients with proper timeouts
- Graceful shutdown on SIGINT/SIGTERM
- Background indexing worker with status reporting
- Enhanced health check with dependency status
- Error type system with `errors.Is()`/`errors.As()` support

### 1.2 Performance Optimizations

**Priority:** High
**Dependencies:** 1.1
**Reference:** ARCHITECTURE_REVIEW.md Section 3.1

**Tasks:**

1. Parallelize vector searches across vaults (ARCHITECTURE_REVIEW.md Section 3.1)
2. Batch chunk fetching for reranking (ARCHITECTURE_REVIEW.md Section 3.1)
3. Add context window management with token counting (ARCHITECTURE_REVIEW.md Section 2.2)
4. Implement folder selection caching (ARCHITECTURE_REVIEW.md Section 3.1)

**Deliverables:**

- Concurrent vault searches using goroutines
- Batch `GetByIDs` method in chunk repository
- Token counting utility for context building
- LRU cache for folder selections with TTL

### 1.3 Observability and Monitoring

**Priority:** Medium
**Dependencies:** 1.1
**Reference:** ARCHITECTURE_REVIEW.md Section 5.1

**Tasks:**

1. Add Prometheus metrics collection
2. Implement request tracing/logging improvements
3. Add retry logic with exponential backoff (ARCHITECTURE_REVIEW.md Section 5.2)

**Deliverables:**

- Metrics for query duration, embedding latency, vector search latency, LLM latency
- Structured request tracing
- Retry middleware for external service calls

---

## Phase 2: Enhanced RAG Capabilities

**Goal:** Improve answer quality and retrieval precision before building specialized agents.

### 2.1 Answer Quality Improvements

**Priority:** Medium
**Dependencies:** Phase 1
**Reference:** ARCHITECTURE_REVIEW.md Section 4.2

**Tasks:**

1. Enhanced lexical scoring with TF-IDF (ARCHITECTURE_REVIEW.md Section 2.2)
2. Improved prompt engineering with few-shot examples (ARCHITECTURE_REVIEW.md Section 4.2)
3. Query expansion for alternative phrasings (ARCHITECTURE_REVIEW.md Section 2.2)
4. Chunk overlap s