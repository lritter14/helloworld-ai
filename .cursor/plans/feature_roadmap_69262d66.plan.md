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
- Health check endpoint (`/api/health`) ✅ **Already implemented**
- Re-indexing endpoint (`/api/index`)
- Note file serving (`/notes/{vault}/*`)
- Basic web UI for RAG queries
- Two-vault support (personal + work)
- Hash-based change detection for efficient re-indexing
- HTTP client timeouts and connection pooling ✅ **Already implemented**
- Asynchronous indexing at startup ✅ **Already implemented**

**See:** `ARCHITECTURE_REVIEW.md` for detailed evaluation of current architecture, performance characteristics, and quality mechanisms.

---

## Phase 1: Foundation Improvements

**Goal:** Address critical infrastructure gaps and improve system reliability before adding new features.

### 1.1 Critical Infrastructure Fixes

**Priority:** High
**Dependencies:** None
**Reference:** ARCHITECTURE_REVIEW.md Sections 1.2, 5.1

**Tasks:**

1. Add graceful shutdown handling (ARCHITECTURE_REVIEW.md Section 1.2)
2. Implement structured error types (ARCHITECTURE_REVIEW.md Section 1.2)

**Deliverables:**

- Graceful shutdown on SIGINT/SIGTERM with request draining
- Error type system with `errors.Is()`/`errors.As()` support
- Error mapping in handlers using structured types instead of string matching

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
4. Chunk overlap strategy for better context continuity (ARCHITECTURE_REVIEW.md Section 2.2)

**Deliverables:**

- TF-IDF weighted lexical scoring
- Few-shot examples in system prompts
- Query expansion using LLM to generate alternative phrasings
- Sliding window overlap for adjacent chunks

### 2.2 Advanced Retrieval Features

**Priority:** Low
**Dependencies:** Phase 2.1
**Reference:** ARCHITECTURE_REVIEW.md Section 4.2

**Tasks:**

1. Answer verification against retrieved context
2. Multi-step reasoning for complex queries
3. Confidence indicators in answers

**Deliverables:**

- Answer verification step using LLM
- Iterative retrieval for complex multi-part questions
- Confidence scoring in response format

---

## Phase 3: MCP (Model Context Protocol) Integration

**Goal:** Enable the system to act as an MCP server, allowing external clients to interact with notes and RAG capabilities.

### 3.1 MCP Server Foundation

**Priority:** High
**Dependencies:** Phase 1.1
**Reference:** MCP Protocol Specification

**Tasks:**

1. Implement MCP server protocol (JSON-RPC 2.0 over stdio/HTTP)
2. Register core tools: `ask_question`, `search_notes`, `get_note`, `list_vaults`
3. Add resource providers for notes and vaults
4. Implement prompt templates for common queries

**Deliverables:**

- MCP server package (`internal/mcp/`)
- JSON-RPC 2.0 handler
- Tool registration system
- Resource provider interface
- Prompt template registry

### 3.2 MCP Tools Implementation

**Priority:** High
**Dependencies:** 3.1

**Tasks:**

1. `ask_question` tool - wraps RAG engine with MCP interface
2. `search_notes` tool - semantic search over indexed notes
3. `get_note` tool - retrieve full note content by path
4. `list_vaults` tool - enumerate available vaults and folders
5. `index_vault` tool - trigger re-indexing of specific vault

**Deliverables:**

- MCP tool implementations for all core operations
- Tool parameter validation and error handling
- Integration with existing RAG engine and storage

### 3.3 MCP Resources and Prompts

**Priority:** Medium
**Dependencies:** 3.1

**Tasks:**

1. Resource provider for note files (`note://{vault}/{path}`)
2. Resource provider for vault metadata (`vault://{name}`)
3. Prompt templates for common use cases (summarization, extraction, etc.)
4. Dynamic prompt generation based on vault structure

**Deliverables:**

- Resource URI scheme implementation
- Prompt template system
- Template variables and substitution

---

## Phase 4: Enhanced APIs

**Goal:** Provide more useful APIs for programmatic access and integration.

### 4.1 Note Management APIs

**Priority:** Medium
**Dependencies:** Phase 1

**Tasks:**

1. `GET /api/v1/notes` - List notes with filtering (vault, folder, search)
2. `GET /api/v1/notes/{vault}/{path}` - Get note content and metadata
3. `POST /api/v1/notes` - Create new note
4. `PUT /api/v1/notes/{vault}/{path}` - Update existing note
5. `DELETE /api/v1/notes/{vault}/{path}` - Delete note

**Deliverables:**

- RESTful note management endpoints
- Request/response DTOs
- Validation and error handling
- Swagger documentation

### 4.2 Search and Discovery APIs

**Priority:** Medium
**Dependencies:** Phase 1

**Tasks:**

1. `POST /api/v1/search` - Semantic search with filters
2. `GET /api/v1/vaults` - List vaults and their metadata
3. `GET /api/v1/vaults/{name}/folders` - List folders in vault
4. `GET /api/v1/vaults/{name}/stats` - Get vault statistics (note count, last indexed, etc.)

**Deliverables:**

- Search API with vector and keyword options
- Vault discovery endpoints
- Statistics and metadata endpoints

### 4.3 Batch Operations API

**Priority:** Low
**Dependencies:** Phase 4.1

**Tasks:**

1. `POST /api/v1/batch/ask` - Multiple questions in single request
2. `POST /api/v1/batch/index` - Index multiple files
3. `POST /api/v1/batch/delete` - Delete multiple notes

**Deliverables:**

- Batch operation endpoints
- Transaction support where applicable
- Progress tracking for long-running batches

---

## Phase 5: Specialized Agents

**Goal:** Create domain-specific agents that interact with notes in prescribed ways for specific use cases.

### 5.1 Recipe Agent

**Priority:** Medium
**Dependencies:** Phase 3, Phase 4.1

**Tasks:**

1. Recipe detection and parsing from notes
2. Recipe storage schema (ingredients, steps, metadata)
3. `POST /api/v1/agents/recipes/add` - Add new recipe from note or structured input
4. `GET /api/v1/agents/recipes` - List recipes with filtering
5. `GET /api/v1/agents/recipes/{id}` - Get recipe details
6. Recipe search by ingredients, cuisine, dietary restrictions
7. Recipe suggestions based on available ingredients

**Deliverables:**

- Recipe agent service (`internal/agents/recipe/`)
- Recipe data model and storage
- Recipe parsing from markdown notes
- Recipe management APIs
- Ingredient-based search and suggestions

### 5.2 Workout Agent

**Priority:** Medium
**Dependencies:** Phase 3, Phase 4.1

**Tasks:**

1. Workout log parsing from notes (date, exercises, sets, reps, weights)
2. Workout plan parsing (program structure, progression rules)
3. Historical performance analysis
4. `POST /api/v1/agents/workouts/log` - Log completed workout
5. `GET /api/v1/agents/workouts/history` - Get workout history with filtering
6. `GET /api/v1/agents/workouts/plans` - List available workout plans
7. `POST /api/v1/agents/workouts/generate` - Generate new workout based on plan and history
8. Progress tracking and visualization data

**Deliverables:**

- Workout agent service (`internal/agents/workout/`)
- Workout log and plan data models
- Historical performance analysis engine
- Workout generation algorithm using plan + history
- Workout management APIs

### 5.3 Agent Framework

**Priority:** High
**Dependencies:** Phase 3

**Tasks:**

1. Generic agent interface and base implementation
2. Agent registration system
3. Agent-specific prompt templates
4. Agent context management (vault scoping, folder preferences)
5. Agent execution pipeline

**Deliverables:**

- Agent framework (`internal/agents/framework/`)
- Base agent interface
- Agent registry
- Context management utilities
- Execution pipeline with error handling

---

## Phase 6: Setup and Configuration Improvements

**Goal:** Improve initial setup, embedding quality, and AI model configuration.

### 6.1 Setup Improvements

**Priority:** High
**Dependencies:** None

**Tasks:**

1. Interactive setup wizard for first-time configuration
2. Configuration validation and diagnostics
3. Automatic vault discovery
4. Model download and verification
5. Setup health checks (verify all dependencies)

**Deliverables:**

- Setup wizard CLI tool or web interface
- Configuration validator
- Dependency checker
- Model management utilities

### 6.2 Embedding Improvements

**Priority:** Medium
**Dependencies:** Phase 1

**Tasks:**

1. Support for multiple embedding models
2. Embedding model comparison and selection
3. Embedding quality metrics
4. Fine-tuning support (future)
5. Hybrid embeddings (combine multiple models)

**Deliverables:**

- Multi-model embedding support
- Model comparison utilities
- Quality metrics collection
- Embedding strategy interface

### 6.3 AI Model Configuration

**Priority:** Medium
**Dependencies:** Phase 1

**Tasks:**

1. Model configuration profiles (fast, balanced, quality)
2. Dynamic model switching based on query complexity
3. Model performance monitoring
4. Temperature and parameter tuning
5. Model-specific prompt optimization

**Deliverables:**

- Model configuration system
- Performance-based model selection
- Parameter tuning utilities
- Prompt optimization per model

---

## Phase 7: Advanced Features

**Goal:** Add advanced capabilities for power users and complex workflows.

### 7.1 File Watching and Auto-Indexing

**Priority:** Low
**Dependencies:** Phase 1

**Tasks:**

1. File system watcher for vault directories
2. Incremental indexing on file changes
3. Debouncing for rapid changes
4. Conflict resolution for concurrent edits

**Deliverables:**

- File watcher implementation
- Incremental indexing pipeline
- Change detection and processing

### 7.2 Multi-Model RAG

**Priority:** Low
**Dependencies:** Phase 6.2

**Tasks:**

1. Ensemble retrieval using multiple embedding models
2. Result fusion and ranking
3. Model-specific query routing

**Deliverables:**

- Multi-model retrieval system
- Result fusion algorithms
- Query routing logic

### 7.3 Advanced Query Features

**Priority:** Low
**Dependencies:** Phase 2

**Tasks:**

1. Conversational context across queries
2. Query refinement suggestions
3. Related question generation
4. Query history and learning

**Deliverables:**

- Conversation context management
- Query suggestion engine
- Related question generator

---

## Implementation Order Summary

**Recommended sequence:**

1. **Phase 1.1** - Critical infrastructure (graceful shutdown, error types)
2. **Phase 1.2** - Performance optimizations (parallel searches, batch fetching)
3. **Phase 3.1-3.2** - MCP server foundation and core tools
4. **Phase 5.3** - Agent framework
5. **Phase 4.1** - Note management APIs (needed by agents)
6. **Phase 5.1** - Recipe agent
7. **Phase 5.2** - Workout agent
8. **Phase 6.1** - Setup improvements
9. **Phase 1.3** - Observability
10. **Phase 2.1** - RAG quality improvements
11. **Phase 4.2-4.3** - Additional APIs
12. **Phase 6.2-6.3** - Embedding and AI improvements
13. **Phase 7** - Advanced features

---

## Notes

- Each phase should be implemented incrementally with tests
- Reference `ARCHITECTURE_REVIEW.md` for architectural patterns and best practices
- Follow existing layer boundaries and interface patterns
- Maintain backward compatibility where possible
- Document all new APIs with Swagger annotations