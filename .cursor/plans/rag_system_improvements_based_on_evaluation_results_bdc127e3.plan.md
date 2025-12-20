---
name: RAG System Improvements Based on Evaluation Results
overview: This plan addresses low performance metrics identified in evaluation run 20251219_050759. The evaluation shows critical issues with groundedness (0-3 scores), attribution hit rate (often 0), scope miss rate (high), and retrieval metrics (Recall@K, MRR, Precision@K often 0). The plan proposes 7 code-only improvements to the RAG engine that will improve citation behavior, reference extraction, context formatting, and system observability without requiring model changes.
todos:
  - id: improve-system-prompt
    content: "Strengthen system prompt to require explicit citations in structured format. Update systemPrompt in Ask() method (lines 839-841) to require citations for all major claims using format '[File: filename.md, Section: section name]'. Add explicit instruction to not make unsupported claims. Expected impact: Improves Groundedness (reduces unsupported claims) and Attribution Hit Rate (encourages proper citations)."
    status: completed
  - id: extract-citations-from-answer
    content: "Implement extractCitationsFromAnswer() method to parse citations from LLM answer and filter references to only include cited chunks. Replace current reference building logic (lines 879-888) that includes all chunks. Method should match filename and section names mentioned in answer. Fall back to all chunks if no citations found (backward compatibility). Expected impact: Significantly improves Attribution Hit Rate by aligning references with actual citations."
    status: completed
  - id: number-chunks-in-context
    content: "Improve context formatting to number chunks and add citation instructions. Update context building (lines 815-823) to prefix each chunk with [Chunk N] and add citation guidance at end. This makes it easier for LLM to reference specific chunks. Expected impact: Improves Groundedness and Attribution Hit Rate by making citations easier and more consistent."
    status: completed
  - id: add-citation-validation
    content: "Add post-generation validation to check if answer contains citations. After LLM response (after line 874), validate that answer mentions at least one filename from provided chunks. Log warning if no citations found despite having context chunks. Expected impact: Improves observability for Groundedness issues, helps identify cases where citations are missing."
    status: pending
  - id: improve-folder-selection-logging
    content: "Enhance folder selection logging to diagnose scope miss issues. After folder selection (around lines 434-436), add Info-level logging showing selected folders count and list, or explicit message when no folders selected (searching all folders). Expected impact: Improves Scope Miss Rate diagnosis and helps tune folder selection logic."
    status: pending
  - id: lower-llm-temperature
    content: "Reduce LLM temperature from 0.7 to 0.3 in ChatWithMessages call (line 866). Lower temperature produces more focused, citation-aware responses with less hallucination. Expected impact: Improves Groundedness (reduces hallucinations), Correctness (more focused answers), and Attribution Hit Rate (more consistent citation behavior)."
    status: pending
  - id: enhance-score-threshold-logging
    content: "Upgrade vector score threshold logging from Debug to Info level (lines 594-600). Include threshold value and rel_path in log message to help diagnose why relevant chunks are filtered out. Expected impact: Improves observability for Recall@K and Precision@K issues, helps tune score thresholds."
    status: pending
---

# RAG System Improvements Based on Evaluation Results

## Evaluation Results Analysis

Evaluation run `20251219_050759` (10 test cases) reveals several critical performance issues:

### Current Metric Levels

**Retrieval Metrics:**

- **Recall@K**: Often 0.0 - system fails to retrieve gold support chunks
- **MRR (Mean Reciprocal Rank)**: Often 0.0 - no correct chunks in top K
- **Precision@K**: Often 0.0 - top K chunks don't match gold supports
- **Scope Miss Rate**: High - folder selection excludes all gold supports
- **Attribution Hit Rate**: Low - final references don't match gold supports

**Answer Quality Metrics:**

- **Groundedness**: 0-3 (out of 5) - many unsupported claims, missing citations
- **Correctness**: 2-3 (out of 5) - partially correct but incomplete answers

**Operational Metrics:**

- **Error Rate**: 0.00% - system is stable
- **Latency**: p50 ~4.8s, p95 ~11s - acceptable performance

### Root Causes Identified

1. **Weak Citation Requirements**: System prompt only says "cite when possible" rather than requiring citations
2. **Reference Mismatch**: All retrieved chunks are returned as references, not just those actually cited by the LLM
3. **Poor Citation Format**: Context doesn't provide clear citation format, making it hard for LLM to cite properly
4. **High Temperature**: 0.7 temperature leads to less focused, less citation-aware responses
5. **Limited Observability**: Insufficient logging to diagnose retrieval and citation issues

## Implementation Plan

All changes target [internal/rag/engine.go](internal/rag/engine.go) and are code-only improvements that don't require model changes.