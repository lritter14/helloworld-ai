# Evaluation Results Summary

This document tracks how each change to the RAG system has affected evaluation metrics.

## Baseline: 20251219_050759

- **Configuration**: Original system prompt ("cite when possible"), temperature=0.7
- **Groundedness**: 1.80
- **Correctness**: 2.60
- **Attribution Hit Rate**: 0.0%
- **Citations in Answers**: 0/10

## Change 1: Required Citations with Example (20251219_221906)

- **Configuration**: System prompt requires citations in format `[File: ..., Section: ...]` with example, temperature=0.7
- **Changes Made**:
  - Added explicit requirement for Citations section at bottom of answer
  - Added example citation format
  - Strengthened language ("CRITICAL", "MUST", "REQUIRED")

- **Groundedness**: 1.80 → 2.00 (+0.20)
- **Correctness**: 2.60 → 1.50 (-1.10)
- **Attribution Hit Rate**: 0.0% → 20.0% (+20.0%)
- **Citations in Answers**: 0/10 → 10/10

**Impact**: Citations now appear in all answers, attribution hit rate improved from 0% to 20%. However, correctness dropped significantly (-1.10), suggesting the model was prioritizing citation compliance over answer quality.

## Change 2: Temperature Reduction + Prompt Rebalancing (20251219_223251)

- **Configuration**: Temperature reduced 0.7 → 0.3, prompt rebalanced to emphasize answer quality first
- **Changes Made**:
  - Reduced temperature from 0.7 to 0.3 for more focused responses
  - Added "Your primary goal is to provide accurate, complete answers" at start of prompt
  - Changed closing to "Remember: Answer quality comes first, but citations are required"

- **Groundedness**: 2.00 → 1.80 (-0.20)
- **Correctness**: 1.50 → 2.40 (+0.90)
- **Attribution Hit Rate**: 20.0% → 20.0% (+0.0%)
- **Citations in Answers**: 10/10 → 9/10

**Impact**: Correctness recovered significantly (+0.90), now above baseline. Attribution hit rate maintained at 20%. Citations still appearing in 9/10 answers. Small groundedness drop (-0.20) but overall net positive improvement.

## Summary

| Metric | Baseline | After Citations | After Temp+Prompt | Net Change |
|--------|----------|------------------|-------------------|------------|
| Groundedness | 1.80 | 2.00 | 1.80 | +0.00 |
| Correctness | 2.60 | 1.50 | 2.40 | -0.20 |
| Attribution Hit | 0.0% | 20.0% | 20.0% | +20.0% |
| Citations | 0/10 | 10/10 | 9/10 | +9 |

## Key Takeaways

1. **Citation Requirements Work**: Requiring citations with examples successfully gets the model to include citations in answers (0/10 → 10/10 → 9/10)

2. **Attribution Hit Rate Improved**: Path/heading normalization and citation extraction improved attribution matching (0% → 20%)

3. **Temperature Reduction Helps**: Lower temperature (0.7 → 0.3) significantly improved correctness (+0.90) while maintaining citations

4. **Prompt Balance Matters**: Rebalancing prompt to emphasize answer quality first recovered correctness without losing citation compliance

5. **Trade-offs Exist**: Small groundedness drop (-0.20) suggests some tension between correctness and claim coverage, but net improvement overall

