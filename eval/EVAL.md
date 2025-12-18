# Core Evaluation Metrics

This document defines the core metrics tracked in every evaluation run. These metrics provide objective, repeatable measurements that enable meaningful comparisons across runs and over time.

## Retrieval Metrics

Retrieval metrics measure whether the system successfully found the relevant content needed to answer the question. These metrics are objective and don't drift over time.

### 1. Retrieval Recall@K

**Definition**: Binary metric - did we retrieve at least one chunk that matches the gold supports?

**Calculation**:

- **Recall_any@K**: At least one retrieved chunk matches any gold_support anchor (1 if yes, 0 if no)
- **Recall_all@K**: For multi-hop questions, did we retrieve *all required* supports? (only for tests marked `multi_hop=true` with `required_support_groups`)

**Match Definition** (deterministic normalization):

- Normalize `heading_path`: strip extra spaces, consistent delimiter (` > `), same depth rules
- A retrieved chunk matches a gold support if:
  - Same `rel_path` (exact match), and
  - Retrieved `heading_path` **starts with** gold `heading_path` (prefix match) - handles cases where chunking depth changes
- If `snippets` are provided in gold_supports, optionally require snippet hit (chunk text contains snippet)

**Rationale**: This prevents accidental "misses" when chunking depth or heading formatting changes. Anchor-based matching (rel_path + heading_path) is resilient to chunking strategy changes.

**Multi-hop Handling**:

- For `category: "multi_hop"` with `required_support_groups`, compute both `Recall_any@K` and `Recall_all@K`
- `Recall_all@K` prevents multi-hop questions from looking "green" when only half the needed evidence was retrieved

**Recency/Conflict Cases**:

- For `category: "recency/conflict"`, define what "correct" means in labeling:
  - `recency_conflict_rule: "cite_newer"` - must cite the newer note
  - `recency_conflict_rule: "acknowledge_both"` - must acknowledge conflict and cite both
  - `recency_conflict_rule: "cite_both"` - must cite both notes

### 2. MRR (Mean Reciprocal Rank)

**Definition**: How high was the first correct chunk ranked?

**Calculation**: 1 / rank of first matching chunk (0 if no match found)

**Example**:

- If first matching chunk is at rank 1: MRR = 1/1 = 1.0
- If first matching chunk is at rank 3: MRR = 1/3 = 0.33
- If no matching chunk found: MRR = 0

**Rationale**: Measures retrieval quality beyond binary recall - a system that ranks correct chunks higher is better, even if recall is the same.

### 3. Precision@K (Optional)

**Definition**: Fraction of top K chunks that match any gold_support anchor.

**Calculation**: Number of top K chunks that match any gold_support / K

**Rationale**: Tells you if you dragged in junk that can hurt groundedness + cost/latency. Lightweight metric that complements Recall@K.

**Multi-hop Handling**: For multi-hop, allow multiple anchors (count as match if matches any required support).

### 4. Scope Miss Rate

**Definition**: Fraction of cases where folder selection excluded all gold supports.

**Calculation**: Only calculated when `folder_mode=on` or `folder_mode=on_with_fallback`. Miss if folder selection excludes all gold supports (strict definition).

**Rationale**: Detects when folder selection silently kills recall. Critical for systems with folder-based scoping.

### 5. Attribution Hit Rate

**Definition**: For answerable questions, did the final cited references include at least one matching gold_support?

**Calculation**: Binary (1 if yes, 0 if no). Only computed for questions where `answerable=true`. Checks if any reference in the answer's `references` field matches a gold_support (using same match criteria as Recall@K).

**Rationale**: Catches cases where retrieval found the right content but generation ignored it or cited irrelevant chunks. Measures end-to-end attribution quality.

## Answer Quality Metrics

Answer quality metrics measure whether the generated answer is correct and well-supported. These metrics use LLM-as-judge with controlled configuration to minimize drift.

### 6. Groundedness (0-5)

**Definition**: Are all claims in the answer supported by the provided context?

**Judge Input**: Answer text + retrieved context (top K chunk texts)

**Judge Output**: Score (0-5) + structured JSON with `unsupported_claims` and `supported_claims` lists

**Scoring Scale**:

- **5**: All claims directly supported by context AND all major claims have citations
- **4**: Most claims supported with citations, minor unsupported details
- **3**: Some claims supported, some unsupported, or missing citations
- **2**: Major claims unsupported or missing citations
- **1**: Answer contradicts context
- **0**: Answer has no relation to context

**Important**: Score of 5 requires citations for all major claims (citation coverage is part of groundedness).

**Judge Configuration** (Critical for preventing drift):

- **Fixed Judge Model**: Pick a single fixed judge model per "season" (e.g., Qwen2.5-14B, or specific cloud model)
- **Immutable Version**: Judge model must be pinned to an immutable version or local model build hash
- **Judge Temperature**: Always use `temperature=0` for deterministic scoring
- **Prompt Version**: Store exact judge prompt version in config.json

**Rationale**: Groundedness detects hallucination (claims not in context). Structured JSON output (unsupported_claims) makes debugging dramatically easier.

### 7. Correctness (0-5)

**Definition**: Does the answer correctly address the question? (considers context + question)

**Judge Input**: Question + answer text + retrieved context

**Judge Output**: Score (0-5) + reasoning

**Scoring Scale**:

- **5**: Answer is fully correct and complete
- **4**: Answer is mostly correct with minor issues
- **3**: Answer is partially correct
- **2**: Answer has significant errors
- **1**: Answer is mostly incorrect
- **0**: Answer is completely wrong

**Judge Configuration**: Same as Groundedness (fixed model, temperature=0, prompt version tracking).

**Rationale**: Correctness detects wrong interpretation (even if grounded). Separating groundedness from correctness helps identify different failure modes.

## Abstention Metrics

Abstention metrics measure whether the system knows when not to answer. Critical for real RAG systems that must handle unanswerable questions gracefully.

### 8. Abstention Accuracy

**Definition**: When `answerable=false`, did the model refuse/say it can't find support?

**Calculation**: Binary (1 if abstained, 0 if answered). Only computed for questions where `answerable=false`.

**Detection**:

- **Option 1 (Preferred)**: Use explicit `abstained: bool` field in Go API response
- **Option 2 (Fallback)**: Use a tiny judge prompt for abstention classification (consistent classification, better than regex pattern matching)

**Rationale**: Prevents "Recall@K is always 0" from being misinterpreted as "retrieval is broken" when the correct behavior is to abstain.

### 9. Hallucination Rate on Unanswerable

**Definition**: When `answerable=false`, did it confidently answer anyway?

**Calculation**: Binary (1 if answered confidently, 0 if abstained). Inverse of abstention accuracy.

**Rationale**: High rate indicates the system is hallucinating on unanswerable questions. Critical safety metric for production systems.

## Metric Stability and Drift Control

### Retrieval Metrics (Stable)

Retrieval metrics (Recall@K, MRR, Precision@K, Scope Miss Rate, Attribution Hit Rate) don't drift over time because they:

- Use deterministic anchor-based matching (rel_path + heading_path)
- Don't depend on LLM judges
- Are computed from debug API response (retrieved chunks with ranks)

### LLM-Judge Metrics (Controlled Drift)

LLM-judge metrics (Groundedness, Correctness) have controlled drift through:

- **Fixed judge model** (immutable version or local model build hash)
- **Temperature = 0** (deterministic scoring)
- **Fixed prompt version** (stored in config for reproducibility)
- **Judge input storage** (full judge input payload saved in results for re-judging later)

**Rationale**: Even with temperature=0, model updates (cloud) can change behavior. Pinning judge model version ensures meaningful comparisons across runs.

### Judge Reliability Spot-Check (Optional)

Even with temperature=0, judges can be inconsistent or drift over time. Add a reliability check:

- Re-judge a small random subset (e.g., n=10-20 questions, ~5-10% of eval set) using:
  - **Option A**: Second judge model (different model, same prompt)
  - **Option B**: Same judge with slightly different prompt version
- Compute disagreement rate (score difference > 1 point or different binary classification)
- Report disagreement rate in metrics.json as `judge_reliability: {"disagreement_rate": 0.15, "spot_check_n": 20}`

**Rationale**: Early warning that judge is becoming a bottleneck or drifting. High disagreement (>20%) suggests judge instability.

## Aggregation and Reporting

### Aggregate Metrics

All metrics are aggregated across the evaluation set:

- **Average**: Mean value across all test cases
- **By Tag**: Metrics segmented by work/personal, category, difficulty
- **By Category**: Separate metrics for factual, multi_hop, recency/conflict, etc.
- **Answerable vs Unanswerable**: Separate metrics for answerable and unanswerable questions

### Breakdown Metrics

Metrics are broken down by:

- **Tags**: work/personal, code/health, etc.
- **Category**: factual, reasoning, multi_hop, recency/conflict, general, adversarial
- **Difficulty**: easy, medium, hard
- **Vault**: personal, work

## Performance Metrics

In addition to quality metrics, the following performance metrics are tracked:

- **Latency**: p50/p95 per test, total suite time
- **Latency Breakdown**: Separate timings for folder selection, retrieval, generation, judge
- **Cost**: Judge tokens and estimated cost per run (even if approximate)

## Indexing Coverage Metrics

These metrics make eval sensitive to indexing changes:

- **No Chunks Generated Rate**: Fraction of docs that produced 0 chunks (should trend toward ~0)
- **Chunks Skipped Due to Context**: Fraction of chunks skipped because they exceeded context limit (should trend toward 0)
- **Coverage Stats**:
  - Docs processed, docs with 0 chunks
  - Chunks attempted / embedded / skipped (and why)
  - Distribution of tokens per chunk (min/max/mean/p95)
- **Index Version Info**: chunker_version, embedding_model, index_version

## Operational Metrics

Production reality metrics:

- **Error Rate**: % of cases where API failed / timed out / returned empty
- **Timeout Rate**: % of cases that timed out
- **Empty Response Rate**: % of cases that returned empty answer
- **Coverage by Doc Type** (optional): markdown vs pdf vs code, since chunking failures often cluster by type

## Summary

These metrics provide objective, repeatable measurements that enable:

- **Data-Driven Decisions**: Quantify impact of model/hardware changes
- **Regression Detection**: Catch quality degradation early
- **Optimization Guidance**: Identify which components need improvement
- **Historical Tracking**: See improvement trends over time
- **Configuration Comparison**: Compare different setups objectively

Retrieval metrics are stable and don't drift. LLM-judge metrics have controlled drift through fixed judge model, prompt version, and temperature=0, enabling meaningful comparisons across runs.

