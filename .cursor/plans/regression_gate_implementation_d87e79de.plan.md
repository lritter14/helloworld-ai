---
name: Regression Gate Implementation
overview: Implement regression gate in run_full_eval.py to fail evaluation runs when key metrics drop below configurable thresholds compared to a baseline run, preventing silent regressions from being committed.
todos:
  - id: add_cli_args
    content: "Add CLI arguments to run_full_eval.py: --baseline-run-id, --regression-threshold-recall, --regression-threshold-scope-miss, --regression-threshold-groundedness, --allow-regressions"
    status: pending
  - id: create_check_function
    content: Create check_regressions() function that loads baseline and current metrics, compares key metrics against thresholds, and returns list of detected regressions
    status: pending
    dependencies:
      - add_cli_args
  - id: integrate_gate
    content: "Integrate regression gate into main() function: call check_regressions() at end of pipeline, exit with non-zero code if regressions detected"
    status: pending
    dependencies:
      - create_check_function
  - id: add_invariant_check
    content: Add invariant checking (eval set commit hash, judge model/version/temperature) to ensure baseline and current runs are comparable
    status: pending
    dependencies:
      - create_check_function
  - id: error_messaging
    content: Format clear error messages showing baseline vs current values, deltas, and thresholds for each regression
    status: pending
    dependencies:
      - integrate_gate
  - id: edge_case_handling
    content: "Handle edge cases: no baseline run, missing metrics, retrieval-only mode, invariant mismatches"
    status: pending
    dependencies:
      - integrate_gate
---

# Regression Gate Implementation

## Overview

Add a regression gate to `run_full_eval.py` that compares the current evaluation run against a baseline run and fails (exits with non-zero code) if key metrics regress beyond configurable thresholds. This prevents silent regressions from being committed and forces explicit acknowledgment of trade-offs.

## Implementation Details

### 1. CLI Arguments

Add the following arguments to `run_full_eval.py`:

- `--baseline-run-id <run_id>`: Specify which run to use as baseline for comparison (defaults to previous run if not specified)
- `--regression-threshold-recall <float>`: Threshold for Recall@K regression (default: 0.05, i.e., 5% absolute drop)
- `--regression-threshold-scope-miss <float>`: Threshold for scope miss rate increase (default: 0.10, i.e., 10% absolute increase)
- `--regression-threshold-groundedness <float>`: Threshold for groundedness drop (default: 0.5 points)
- `--allow-regressions`: Disable regression gate (for exploratory runs)

**File**: `eval/scripts/run_full_eval.py`

### 2. Regression Checking Function

Create a new function `check_regressions()` that:

1. Loads baseline run metrics (from `--baseline-run-id` or previous run)
2. Loads current run metrics
3. Compares key metrics:

   - `recall_at_k_avg`: Current < Baseline - threshold → regression
   - `scope_miss_rate`: Current > Baseline + threshold → regression
   - `groundedness_avg`: Current < Baseline - threshold → regression

4. Returns list of detected regressions with details
5. Handles edge cases:

   - No baseline run available → skip check (warn but don't fail)
   - Missing metrics in baseline or current → skip that metric (warn)
   - Metrics not computed yet (e.g., groundedness in retrieval-only mode) → skip

**File**: `eval/scripts/run_full_eval.py`

### 3. Integration into Main Pipeline

Call `check_regressions()` at the end of `main()` function, after all metrics have been computed and written:

1. Only check if `--allow-regressions` is not set
2. Only check if baseline run exists and has metrics
3. If regressions detected:

   - Print clear error message with regression details
   - Show baseline vs current values and deltas
   - Exit with non-zero code (e.g., `sys.exit(1)`)

4. If no regressions or check skipped:

   - Print success message (or skip silently if no baseline)
   - Continue normal exit

**File**: `eval/scripts/run_full_eval.py`

### 4. Error Messages

Format regression errors clearly:

```
❌ REGRESSION DETECTED

The following metrics have regressed beyond thresholds:

1. Recall@K: 0.85 → 0.78 (delta: -0.07, threshold: 0.05)
   Baseline: 0.85 (run: 20251218_063617)
   Current:  0.78 (run: 20251219_120000)

2. Scope Miss Rate: 0.05 → 0.18 (delta: +0.13, threshold: 0.10)
   Baseline: 0.05 (run: 20251218_063617)
   Current:  0.18 (run: 20251219_120000)

Use --allow-regressions to proceed anyway, or fix the regressions.
```

**File**: `eval/scripts/run_full_eval.py`

### 5. Invariant Checking

Reuse invariant checking logic from `compare_runs.py` to ensure baseline and current runs are comparable:

- Same eval set commit hash
- Same judge model + prompt version (if judges were run)
- Same judge temperature (if judges were run)

If invariants don't match, warn but don't fail (comparison may still be meaningful for some metrics).

**File**: `eval/scripts/run_full_eval.py` (import or reuse logic from `compare_runs.py`)

## Implementation Steps

1. **Add CLI arguments** to `run_full_eval.py` argument parser
2. **Create `check_regressions()` function** with comparison logic
3. **Import `load_metrics` and `load_config`** from storage module (already imported)
4. **Call `check_regressions()`** at end of `main()` function
5. **Add error handling** for missing baseline runs or metrics
6. **Test with sample runs** to verify thresholds work correctly

## Files to Modify

- `eval/scripts/run_full_eval.py`: Add regression gate implementation

## Dependencies

- Uses existing `load_metrics()` and `load_config()` from `storage.py`
- Reuses invariant checking logic from `compare_runs.py` (can import or duplicate)

## Testing Considerations

- Test with baseline run that has all metrics
- Test with baseline run missing some metrics (e.g., no judges)
- Test with no baseline run available
- Test with regressions detected (should fail)
- Test with no regressions (should pass)
- Test with `--allow-regressions` flag (should skip check)
- Test with custom thresholds
- Test with `--baseline-run-id` pointing to specific run

## Edge Cases

- **No baseline run**: Warn but don't fail (first run or baseline deleted)
- **Missing metrics**: Skip that metric check, warn
- **Retrieval-only mode**: Only check retrieval metrics (recall, scope_miss_rate)
- **Invariant mismatch**: Warn but proceed (user may want to compare anyway)
- **Baseline run incomplete**: Check only metrics that exist in both runs