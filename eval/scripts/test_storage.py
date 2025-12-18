"""
Simple test script to verify storage module functionality.
"""

import sys
import tempfile
from pathlib import Path

# Add scripts directory to path for imports (needed for pytest)
scripts_dir = Path(__file__).parent
if str(scripts_dir) not in sys.path:
    sys.path.insert(0, str(scripts_dir))

# Import after path setup
from storage import (  # noqa: E402
    ResultsWriter,
    TestResult,
    RetrievedChunk,
    RunConfig,
    IndexingCoverage,
    LatencyBreakdown,
    RetrievalMetrics,
    GroundednessScore,
    CorrectnessScore,
    AbstentionResult,
    JudgeInput,
    CostTracking,
    load_results,
    load_metrics,
    load_config,
)


def test_storage_basic():
    """Test basic storage functionality."""
    with tempfile.TemporaryDirectory() as tmpdir:
        writer = ResultsWriter(output_dir=tmpdir, run_id="test_run_001")

        # Create a test result
        config = RunConfig(
            k=5,
            rerank_weights={"vector": 0.7, "lexical": 0.3},
            folder_mode="on_with_fallback",
            llm_model="llama3.2",
            embedding_model="granite-278m",
            judge_model="qwen2.5-14b",
            judge_prompt_version="v1.0",
            judge_temperature=0.0,
            dataset_version="abc123",
            index_build_version="chunker_v1.2+embedding_granite-278m",
            retriever_version="k5+rerank_70_30",
            answerer_version="prompt_v2.0+llm_llama3.2",
        )

        retrieved_chunks = [
            RetrievedChunk(
                chunk_id="chunk_abc123",
                rel_path="projects/main.md",
                heading_path="# Overview",
                rank=1,
                score_vector=0.95,
                score_lexical=0.80,
                score_final=0.90,
                text="This is a long chunk of text that should be truncated when store_full_text is False. " * 10,
                token_count=245,
            ),
            RetrievedChunk(
                chunk_id="chunk_def456",
                rel_path="docs/config.md",
                heading_path="# Setup > ## Embeddings",
                rank=2,
                score_vector=0.85,
                score_lexical=0.75,
                score_final=0.82,
                text="Another chunk of text here.",
                token_count=150,
            ),
        ]

        indexing_coverage = IndexingCoverage(
            docs_processed=1500,
            docs_with_0_chunks=5,
            chunks_attempted=8500,
            chunks_embedded=8450,
            chunks_skipped=50,
            chunks_skipped_reasons={"context_limit_exceeded": 45, "too_small": 5},
            chunk_token_stats={"min": 10, "max": 512, "mean": 245, "p95": 480},
            chunker_version="v1.2",
            index_version="2024-01-15_abc123",
        )

        latency = LatencyBreakdown(
            total_ms=1234.0,
            folder_selection_ms=50.0,
            retrieval_ms=200.0,
            generation_ms=900.0,
            judge_ms=84.0,
        )

        retrieval_metrics = RetrievalMetrics(
            recall_at_k=1.0,
            mrr=0.5,
            scope_miss=False,
            attribution_hit=True,
        )

        groundedness = GroundednessScore(
            score=4.5,
            reasoning="All claims are well-supported by the context.",
            unsupported_claims=[],
            supported_claims=["claim 1", "claim 2"],
        )

        correctness = CorrectnessScore(
            score=4.0,
            reasoning="Answer correctly addresses the question with minor issues.",
        )

        abstention = AbstentionResult(abstained=False, hallucinated=False)

        judge_input = JudgeInput(
            question="What is the main topic of the project?",
            answer="The project is about RAG systems using llama.cpp.",
            context_chunk_ids=["chunk_abc123", "chunk_def456"],
            context_chunks_truncated=["First 200 chars of chunk 1...", "First 200 chars of chunk 2..."],
        )

        cost = CostTracking(judge_tokens=500, judge_cost_usd=0.001)

        result = TestResult(
            test_case_id="test_001",
            question="What is the main topic of the project?",
            answer="The project is about RAG systems using llama.cpp for local LLMs.",
            references=[
                {"chunk_id": "chunk_abc123", "rel_path": "projects/main.md", "heading_path": "# Overview"}
            ],
            retrieved_chunks=retrieved_chunks,
            config=config,
            indexing_coverage=indexing_coverage,
            latency=latency,
            retrieval_metrics=retrieval_metrics,
            groundedness=groundedness,
            correctness=correctness,
            abstention=abstention,
            judge_input=judge_input,
            cost=cost,
        )

        # Test truncated text (default)
        writer.write_result(result, store_full_text=False)

        # Verify truncation
        results = load_results(str(writer.run_dir))
        assert len(results) == 1
        assert len(results[0]["retrieved_chunks"][0]["text"]) <= 200 + 3  # 200 chars + "..."

        # Test full text
        writer2 = ResultsWriter(output_dir=tmpdir, run_id="test_run_002")
        writer2.write_result(result, store_full_text=True)

        results2 = load_results(str(writer2.run_dir))
        assert len(results2) == 1
        # Full text should be much longer
        assert len(results2[0]["retrieved_chunks"][0]["text"]) > 200

        # Test config writing
        writer.write_config(config, eval_set_commit_hash="abc123def456")

        config_loaded = load_config(str(writer.run_dir))
        assert config_loaded["k"] == 5
        assert config_loaded["eval_set_commit_hash"] == "abc123def456"

        # Test metrics writing
        metrics = {
            "recall_at_k_avg": 0.85,
            "mrr_avg": 0.72,
            "scope_miss_rate": 0.05,
            "attribution_hit_rate": 0.88,
            "groundedness_avg": 4.2,
            "correctness_avg": 4.0,
            "abstention_accuracy": 0.90,
            "hallucination_rate_unanswerable": 0.10,
            "latency": {
                "p50_ms": 1200,
                "p95_ms": 2500,
                "total_ms": 60000,
            },
            "cost": {
                "judge_total_usd": 0.05,
                "judge_total_tokens": 25000,
            },
        }

        writer.write_metrics(
            metrics,
            config_hash="config_hash_123",
            eval_set_commit_hash="abc123def456",
        )

        metrics_loaded = load_metrics(str(writer.run_dir))
        assert metrics_loaded["run_id"] == "test_run_001"
        assert metrics_loaded["aggregate_metrics"]["recall_at_k_avg"] == 0.85
        assert metrics_loaded["config_hash"] == "config_hash_123"

        print("✓ All storage tests passed!")


if __name__ == "__main__":
    test_storage_basic()

