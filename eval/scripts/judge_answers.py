#!/usr/bin/env python3
"""
Answer quality judges for evaluation framework.

Implements separate groundedness and correctness judges with fixed model
(immutable version), temperature=0, structured JSON output. Includes judge
caching and optional reliability spot-check.

Usage:
    # Judge all results in a run
    python eval/scripts/judge_answers.py --run-id <run_id> --judge-model qwen2.5-14b

    # Judge with cloud model
    python eval/scripts/judge_answers.py --run-id <run_id> --judge-model openai:gpt-4

    # Judge with reliability spot-check
    python eval/scripts/judge_answers.py --run-id <run_id> --judge-model qwen2.5-14b --spot-check
"""

import argparse
import hashlib
import json
import os
import random
import sys
import time
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

try:
    import requests
except ImportError:
    print("Error: 'requests' library is required. Install with: pip install requests")
    sys.exit(1)

try:
    import openai
except ImportError:
    openai = None

try:
    import anthropic
except ImportError:
    anthropic = None

# Import storage module
sys.path.insert(0, str(Path(__file__).parent))
from storage import (
    CorrectnessScore,
    CostTracking,
    GroundednessScore,
    JudgeInput,
    load_config,
    load_results,
    TestResult,
)


# Judge prompt versions
GROUNDEDNESS_PROMPT_V1 = """Evaluate whether all claims in the answer are supported by the retrieved context.

Answer: {answer}

Retrieved Context:
{context_chunks}

IMPORTANT:
- Treat anything not present in context as unsupported, even if it's "common knowledge"
- Penalize "confident tone" on unsupported claims
- Score of 5 requires citations for all major claims (citation coverage is part of groundedness)

Rate groundedness (0-5):
- 5: All claims directly supported by context AND all major claims have citations
- 4: Most claims supported with citations, minor unsupported details
- 3: Some claims supported, some unsupported, or missing citations
- 2: Major claims unsupported or missing citations
- 1: Answer contradicts context
- 0: Answer has no relation to context

Return JSON only (no other text):
{{
  "score": 0-5,
  "reasoning": "...",
  "unsupported_claims": ["claim 1", "claim 2"],
  "supported_claims": ["claim 3", "claim 4"]
}}"""

CORRECTNESS_PROMPT_V1 = """Evaluate whether the answer correctly addresses the question.

Question: {question}

Answer: {answer}

Retrieved Context:
{context_chunks}

Rate correctness (0-5):
- 5: Answer is fully correct and complete
- 4: Answer is mostly correct with minor issues
- 3: Answer is partially correct
- 2: Answer has significant errors
- 1: Answer is mostly incorrect
- 0: Answer is completely wrong

Return JSON only (no other text):
{{
  "score": 0-5,
  "reasoning": "..."
}}"""


class JudgeClient:
    """Base class for judge clients (local LLM, OpenAI, Anthropic)."""

    def __init__(
        self,
        model: str,
        base_url: Optional[str] = None,
        api_key: Optional[str] = None,
        temperature: float = 0.0,
    ):
        """
        Initialize judge client.

        Args:
            model: Model identifier (e.g., "qwen2.5-14b", "openai:gpt-4", "anthropic:claude-3-5-sonnet-20241022")
            base_url: Base URL for local LLM (required for local models)
            api_key: API key (required for cloud models)
            temperature: Temperature for generation (default: 0.0 for deterministic)
        """
        self.model = model
        self.base_url = base_url
        self.api_key = api_key
        self.temperature = temperature
        self.model_type = self._parse_model_type(model)

    def _parse_model_type(self, model: str) -> str:
        """Parse model type from model identifier."""
        if model.startswith("openai:"):
            return "openai"
        elif model.startswith("anthropic:"):
            return "anthropic"
        else:
            return "local"

    def chat(self, messages: List[Dict[str, str]], max_tokens: Optional[int] = None) -> str:
        """
        Send chat completion request to judge model.

        Args:
            messages: List of messages with "role" and "content" keys
            max_tokens: Optional max tokens for response

        Returns:
            Response text from model
        """
        if self.model_type == "openai":
            return self._chat_openai(messages, max_tokens)
        elif self.model_type == "anthropic":
            return self._chat_anthropic(messages, max_tokens)
        else:
            return self._chat_local(messages, max_tokens)

    def _chat_local(self, messages: List[Dict[str, str]], max_tokens: Optional[int] = None) -> str:
        """Call local LLM via llama.cpp server."""
        if not self.base_url:
            raise ValueError("base_url is required for local LLM")

        url = f"{self.base_url.rstrip('/')}/v1/chat/completions"

        payload = {
            "model": self.model,
            "messages": messages,
        }

        # Only include temperature if it's not 0 (some APIs may reject 0 or prefer it omitted)
        if self.temperature > 0:
            payload["temperature"] = self.temperature

        if max_tokens:
            payload["max_tokens"] = max_tokens

        headers = {"Content-Type": "application/json"}
        # API key is optional for local llama.cpp server
        if self.api_key:
            headers["Authorization"] = f"Bearer {self.api_key}"

        try:
            response = requests.post(url, json=payload, headers=headers, timeout=120)
            
            if response.status_code != 200:
                # Try to get error details from response
                try:
                    error_body = response.text
                    # Try to parse as JSON for better error message
                    try:
                        error_json = response.json()
                        if "error" in error_json:
                            error_msg = error_json["error"]
                            if isinstance(error_msg, dict) and "message" in error_msg:
                                error_body = error_msg["message"]
                            elif isinstance(error_msg, str):
                                error_body = error_msg
                    except (ValueError, KeyError):
                        pass
                    
                    if len(error_body) > 500:
                        error_body = error_body[:500] + "..."
                    raise Exception(
                        f"Local LLM request failed with status {response.status_code}: {error_body}"
                    )
                except Exception as e:
                    if isinstance(e, Exception) and "Local LLM request failed" in str(e):
                        raise
                    raise Exception(
                        f"Local LLM request failed with status {response.status_code}: {response.reason}"
                    )
            
            result = response.json()

            if "choices" not in result or len(result["choices"]) == 0:
                raise ValueError("No choices in response")

            return result["choices"][0]["message"]["content"]
        except requests.exceptions.RequestException as e:
            raise Exception(f"Local LLM request failed: {e}")

    def _chat_openai(self, messages: List[Dict[str, str]], max_tokens: Optional[int] = None) -> str:
        """Call OpenAI API."""
        if not openai:
            raise ImportError("openai library is required. Install with: pip install openai")

        if not self.api_key:
            self.api_key = os.getenv("OPENAI_API_KEY")
            if not self.api_key:
                raise ValueError("OPENAI_API_KEY environment variable or api_key parameter required")

        # Extract model name (remove "openai:" prefix)
        model_name = self.model.replace("openai:", "")

        client = openai.OpenAI(api_key=self.api_key)

        try:
            response = client.chat.completions.create(
                model=model_name,
                messages=messages,
                temperature=self.temperature,
                max_tokens=max_tokens,
            )

            if not response.choices or len(response.choices) == 0:
                raise ValueError("No choices in response")

            return response.choices[0].message.content
        except Exception as e:
            raise Exception(f"OpenAI API request failed: {e}")

    def _chat_anthropic(self, messages: List[Dict[str, str]], max_tokens: Optional[int] = None) -> str:
        """Call Anthropic API."""
        if not anthropic:
            raise ImportError("anthropic library is required. Install with: pip install anthropic")

        if not self.api_key:
            self.api_key = os.getenv("ANTHROPIC_API_KEY")
            if not self.api_key:
                raise ValueError("ANTHROPIC_API_KEY environment variable or api_key parameter required")

        # Extract model name (remove "anthropic:" prefix)
        model_name = self.model.replace("anthropic:", "")

        # Convert messages format (Anthropic uses different format)
        # Anthropic expects system message separately
        system_message = None
        conversation_messages = []

        for msg in messages:
            if msg["role"] == "system":
                system_message = msg["content"]
            else:
                conversation_messages.append(msg)

        client = anthropic.Anthropic(api_key=self.api_key)

        try:
            kwargs = {
                "model": model_name,
                "messages": conversation_messages,
                "temperature": self.temperature,
            }

            if system_message:
                kwargs["system"] = system_message

            if max_tokens:
                kwargs["max_tokens"] = max_tokens
            else:
                kwargs["max_tokens"] = 4096  # Default for Anthropic

            response = client.messages.create(**kwargs)

            if not response.content or len(response.content) == 0:
                raise ValueError("No content in response")

            # Anthropic returns list of content blocks
            return response.content[0].text
        except Exception as e:
            raise Exception(f"Anthropic API request failed: {e}")


class JudgeCache:
    """Cache for judge calls to avoid redundant API calls."""

    def __init__(self, cache_file: Path):
        """
        Initialize judge cache.

        Args:
            cache_file: Path to cache file (JSONL format)
        """
        self.cache_file = cache_file
        self.cache: Dict[str, Dict[str, Any]] = {}
        self._load_cache()

    def _load_cache(self):
        """Load cache from file."""
        if not self.cache_file.exists():
            return

        try:
            with open(self.cache_file, "r", encoding="utf-8") as f:
                for line in f:
                    if line.strip():
                        entry = json.loads(line)
                        cache_key = entry.get("cache_key")
                        if cache_key:
                            self.cache[cache_key] = entry
        except Exception as e:
            print(f"Warning: Failed to load judge cache: {e}", file=sys.stderr)

    def _save_entry(self, entry: Dict[str, Any]):
        """Save a single cache entry to file."""
        self.cache_file.parent.mkdir(parents=True, exist_ok=True)
        with open(self.cache_file, "a", encoding="utf-8") as f:
            f.write(json.dumps(entry, ensure_ascii=False) + "\n")

    def get(self, cache_key: str) -> Optional[Dict[str, Any]]:
        """Get cached result."""
        return self.cache.get(cache_key)

    def put(self, cache_key: str, judge_type: str, result: Dict[str, Any], tokens: int = 0, cost_usd: float = 0.0):
        """Store result in cache."""
        entry = {
            "cache_key": cache_key,
            "judge_type": judge_type,
            "result": result,
            "tokens": tokens,
            "cost_usd": cost_usd,
            "timestamp": time.time(),
        }
        self.cache[cache_key] = entry
        self._save_entry(entry)


def compute_context_hash(context_chunks: List[Dict[str, Any]]) -> str:
    """Compute hash of context chunks for caching."""
    # Create a stable representation of context
    context_str = json.dumps(
        [
            {
                "chunk_id": chunk.get("chunk_id", ""),
                "text": chunk.get("text", "")[:500],  # Truncate for hash
            }
            for chunk in context_chunks
        ],
        sort_keys=True,
    )
    return hashlib.sha256(context_str.encode()).hexdigest()[:16]


def compute_cache_key(
    question: str,
    answer: str,
    context_hash: str,
    judge_model: str,
    prompt_version: str,
    judge_type: str,
) -> str:
    """Compute cache key for judge call."""
    key_str = f"{judge_type}|{judge_model}|{prompt_version}|{question}|{answer}|{context_hash}"
    return hashlib.sha256(key_str.encode()).hexdigest()[:32]


def extract_json_from_response(response: str) -> Dict[str, Any]:
    """
    Extract JSON from judge response (may have markdown code blocks or extra text).

    Args:
        response: Raw response from judge model

    Returns:
        Parsed JSON dictionary
    """
    # Try to find JSON in markdown code blocks
    import re

    # Look for JSON in ```json ... ``` blocks
    json_match = re.search(r"```(?:json)?\s*(\{.*?\})\s*```", response, re.DOTALL)
    if json_match:
        try:
            return json.loads(json_match.group(1))
        except json.JSONDecodeError:
            pass

    # Look for JSON object directly
    json_match = re.search(r"\{.*\}", response, re.DOTALL)
    if json_match:
        try:
            return json.loads(json_match.group(0))
        except json.JSONDecodeError:
            pass

    # If all else fails, try parsing the whole response
    try:
        return json.loads(response)
    except json.JSONDecodeError:
        raise ValueError(f"Failed to extract JSON from response: {response[:500]}")


def judge_groundedness(
    answer: str,
    context_chunks: List[Dict[str, Any]],
    judge_client: JudgeClient,
    prompt_version: str,
    cache: Optional[JudgeCache] = None,
    cache_key: Optional[str] = None,
) -> Tuple[GroundednessScore, CostTracking]:
    """
    Judge groundedness of an answer.

    Args:
        answer: Answer text to judge
        context_chunks: List of retrieved chunks (with "text" field)
        judge_client: Judge client instance
        prompt_version: Prompt version identifier
        cache: Optional judge cache
        cache_key: Optional cache key (if already computed)

    Returns:
        Tuple of (GroundednessScore, CostTracking)
    """
    # Check cache first
    if cache and cache_key:
        cached = cache.get(cache_key)
        if cached:
            result = cached["result"]
            return (
                GroundednessScore(
                    score=result.get("score", 0.0),
                    reasoning=result.get("reasoning", ""),
                    unsupported_claims=result.get("unsupported_claims", []),
                    supported_claims=result.get("supported_claims", []),
                ),
                CostTracking(
                    judge_tokens=cached.get("tokens", 0),
                    judge_cost_usd=cached.get("cost_usd", 0.0),
                ),
            )

    # Format context chunks
    context_text = "\n\n".join(
        [
            f"[Chunk {i+1}]\n{chunk.get('text', '')}"
            for i, chunk in enumerate(context_chunks)
        ]
    )

    # Format prompt
    prompt = GROUNDEDNESS_PROMPT_V1.format(answer=answer, context_chunks=context_text)

    messages = [
        {"role": "system", "content": "You are an expert evaluator. Return only valid JSON."},
        {"role": "user", "content": prompt},
    ]

    # Call judge
    start_time = time.time()
    try:
        response_text = judge_client.chat(messages, max_tokens=2000)
        elapsed_ms = (time.time() - start_time) * 1000

        # Extract JSON from response
        result = extract_json_from_response(response_text)

        # Validate score
        score = float(result.get("score", 0.0))
        if not (0.0 <= score <= 5.0):
            raise ValueError(f"Invalid score: {score} (must be 0-5)")

        groundedness = GroundednessScore(
            score=score,
            reasoning=result.get("reasoning", ""),
            unsupported_claims=result.get("unsupported_claims", []),
            supported_claims=result.get("supported_claims", []),
        )

        # Estimate tokens (rough approximation: 1 token ≈ 4 chars)
        estimated_tokens = len(prompt) // 4 + len(response_text) // 4

        # Estimate cost (very rough - actual cost depends on model)
        cost_usd = 0.0  # Will be updated if using cloud models with pricing info

        cost = CostTracking(judge_tokens=estimated_tokens, judge_cost_usd=cost_usd)

        # Cache result
        if cache and cache_key:
            cache.put(
                cache_key,
                "groundedness",
                {
                    "score": groundedness.score,
                    "reasoning": groundedness.reasoning,
                    "unsupported_claims": groundedness.unsupported_claims,
                    "supported_claims": groundedness.supported_claims,
                },
                tokens=estimated_tokens,
                cost_usd=cost_usd,
            )

        return groundedness, cost

    except Exception as e:
        raise Exception(f"Groundedness judge failed: {e}")


def judge_correctness(
    question: str,
    answer: str,
    context_chunks: List[Dict[str, Any]],
    judge_client: JudgeClient,
    prompt_version: str,
    cache: Optional[JudgeCache] = None,
    cache_key: Optional[str] = None,
) -> Tuple[CorrectnessScore, CostTracking]:
    """
    Judge correctness of an answer.

    Args:
        question: Original question
        answer: Answer text to judge
        context_chunks: List of retrieved chunks (with "text" field)
        judge_client: Judge client instance
        prompt_version: Prompt version identifier
        cache: Optional judge cache
        cache_key: Optional cache key (if already computed)

    Returns:
        Tuple of (CorrectnessScore, CostTracking)
    """
    # Check cache first
    if cache and cache_key:
        cached = cache.get(cache_key)
        if cached:
            result = cached["result"]
            return (
                CorrectnessScore(
                    score=result.get("score", 0.0),
                    reasoning=result.get("reasoning", ""),
                ),
                CostTracking(
                    judge_tokens=cached.get("tokens", 0),
                    judge_cost_usd=cached.get("cost_usd", 0.0),
                ),
            )

    # Format context chunks
    context_text = "\n\n".join(
        [
            f"[Chunk {i+1}]\n{chunk.get('text', '')}"
            for i, chunk in enumerate(context_chunks)
        ]
    )

    # Format prompt
    prompt = CORRECTNESS_PROMPT_V1.format(
        question=question, answer=answer, context_chunks=context_text
    )

    messages = [
        {"role": "system", "content": "You are an expert evaluator. Return only valid JSON."},
        {"role": "user", "content": prompt},
    ]

    # Call judge
    start_time = time.time()
    try:
        response_text = judge_client.chat(messages, max_tokens=2000)
        elapsed_ms = (time.time() - start_time) * 1000

        # Extract JSON from response
        result = extract_json_from_response(response_text)

        # Validate score
        score = float(result.get("score", 0.0))
        if not (0.0 <= score <= 5.0):
            raise ValueError(f"Invalid score: {score} (must be 0-5)")

        correctness = CorrectnessScore(
            score=score,
            reasoning=result.get("reasoning", ""),
        )

        # Estimate tokens (rough approximation: 1 token ≈ 4 chars)
        estimated_tokens = len(prompt) // 4 + len(response_text) // 4

        # Estimate cost (very rough - actual cost depends on model)
        cost_usd = 0.0  # Will be updated if using cloud models with pricing info

        cost = CostTracking(judge_tokens=estimated_tokens, judge_cost_usd=cost_usd)

        # Cache result
        if cache and cache_key:
            cache.put(
                cache_key,
                "correctness",
                {
                    "score": correctness.score,
                    "reasoning": correctness.reasoning,
                },
                tokens=estimated_tokens,
                cost_usd=cost_usd,
            )

        return correctness, cost

    except Exception as e:
        raise Exception(f"Correctness judge failed: {e}")


def judge_reliability_spot_check(
    results: List[Dict[str, Any]],
    judge_client: JudgeClient,
    prompt_version: str,
    spot_check_n: int = 20,
    second_judge_model: Optional[str] = None,
    second_judge_base_url: Optional[str] = None,
    second_judge_api_key: Optional[str] = None,
) -> Dict[str, Any]:
    """
    Perform reliability spot-check by re-judging a random subset.

    Args:
        results: List of test results
        judge_client: Primary judge client
        prompt_version: Prompt version identifier
        spot_check_n: Number of results to spot-check
        second_judge_model: Optional second judge model for comparison
        second_judge_base_url: Optional base URL for second judge (if local)
        second_judge_api_key: Optional API key for second judge

    Returns:
        Dictionary with disagreement rate and spot-check results
    """
    if len(results) < spot_check_n:
        spot_check_n = len(results)

    # Select random subset
    selected = random.sample(results, spot_check_n)

    disagreements = 0
    total_checked = 0

    # Create second judge client if provided
    second_judge = None
    if second_judge_model:
        second_judge = JudgeClient(
            model=second_judge_model,
            base_url=second_judge_base_url,
            api_key=second_judge_api_key,
            temperature=0.0,
        )

    for result in selected:
        question = result.get("question", "")
        answer = result.get("answer", "")
        retrieved_chunks = result.get("retrieved_chunks", [])

        if not answer or not retrieved_chunks:
            continue

        # Get original scores
        groundedness = result.get("groundedness")
        correctness = result.get("correctness")

        if not groundedness and not correctness:
            continue

        # Re-judge with second judge (or same judge with slightly different prompt)
        try:
            if second_judge:
                # Use second judge model
                re_groundedness, _ = judge_groundedness(
                    answer, retrieved_chunks, second_judge, prompt_version
                )
                re_correctness, _ = judge_correctness(
                    question, answer, retrieved_chunks, second_judge, prompt_version
                )
            else:
                # Use same judge (will test consistency)
                re_groundedness, _ = judge_groundedness(
                    answer, retrieved_chunks, judge_client, prompt_version
                )
                re_correctness, _ = judge_correctness(
                    question, answer, retrieved_chunks, judge_client, prompt_version
                )

            # Check for disagreement
            if groundedness:
                original_score = groundedness.get("score", 0.0)
                re_score = re_groundedness.score
                if abs(original_score - re_score) > 1.0:
                    disagreements += 1

            if correctness:
                original_score = correctness.get("score", 0.0)
                re_score = re_correctness.score
                if abs(original_score - re_score) > 1.0:
                    disagreements += 1

            total_checked += 1

        except Exception as e:
            print(f"Warning: Spot-check failed for result: {e}", file=sys.stderr)
            continue

    disagreement_rate = disagreements / total_checked if total_checked > 0 else 0.0

    return {
        "disagreement_rate": disagreement_rate,
        "spot_check_n": total_checked,
        "disagreements": disagreements,
    }


def update_results_with_judges(
    run_dir: Path,
    judge_client: JudgeClient,
    prompt_version: str,
    cache: Optional[JudgeCache] = None,
    output_path: Optional[Path] = None,
) -> None:
    """
    Load results, judge answers, and update results file.

    Args:
        run_dir: Path to run directory containing results.jsonl
        judge_client: Judge client instance
        prompt_version: Prompt version identifier
        cache: Optional judge cache
        output_path: Optional path to write updated results (default: overwrite results.jsonl)
    """
    # Load results
    results = load_results(str(run_dir))

    if not results:
        print(f"Warning: No results found in {run_dir}", file=sys.stderr)
        return

    # Update each result with judge scores
    updated_results = []
    total_tokens = 0
    total_cost = 0.0

    for i, result in enumerate(results, 1):
        test_case_id = result.get("test_case_id", f"test_{i}")
        question = result.get("question", "")
        answer = result.get("answer", "")
        retrieved_chunks = result.get("retrieved_chunks", [])

        print(f"[{i}/{len(results)}] Judging {test_case_id}...", end=" ", flush=True)

        if not answer:
            print("SKIP (no answer)")
            updated_results.append(result)
            continue

        if not retrieved_chunks:
            print("SKIP (no context)")
            updated_results.append(result)
            continue

        try:
            # Compute context hash and cache keys
            context_hash = compute_context_hash(retrieved_chunks)
            groundedness_key = compute_cache_key(
                question, answer, context_hash, judge_client.model, prompt_version, "groundedness"
            )
            correctness_key = compute_cache_key(
                question, answer, context_hash, judge_client.model, prompt_version, "correctness"
            )

            # Judge groundedness
            groundedness, groundedness_cost = judge_groundedness(
                answer, retrieved_chunks, judge_client, prompt_version, cache, groundedness_key
            )

            # Judge correctness
            correctness, correctness_cost = judge_correctness(
                question, answer, retrieved_chunks, judge_client, prompt_version, cache, correctness_key
            )

            # Create judge input for reproducibility
            judge_input = JudgeInput(
                question=question,
                answer=answer,
                context_chunk_ids=[chunk.get("chunk_id", "") for chunk in retrieved_chunks],
                context_chunks_truncated=[
                    chunk.get("text", "")[:200] + "..." if len(chunk.get("text", "")) > 200 else chunk.get("text", "")
                    for chunk in retrieved_chunks
                ],
            )

            # Update result
            result["groundedness"] = groundedness.to_dict()
            result["correctness"] = correctness.to_dict()
            result["judge_input"] = judge_input.to_dict()

            # Update cost tracking
            total_tokens += groundedness_cost.judge_tokens + correctness_cost.judge_tokens
            total_cost += groundedness_cost.judge_cost_usd + correctness_cost.judge_cost_usd

            cost = CostTracking(
                judge_tokens=groundedness_cost.judge_tokens + correctness_cost.judge_tokens,
                judge_cost_usd=groundedness_cost.judge_cost_usd + correctness_cost.judge_cost_usd,
            )
            result["cost"] = cost.to_dict()

            print(f"OK (groundedness={groundedness.score:.1f}, correctness={correctness.score:.1f})")
            updated_results.append(result)

        except Exception as e:
            print(f"ERROR: {e}")
            # Keep original result without judge scores
            updated_results.append(result)
            continue

    # Write updated results
    output_file = output_path or (run_dir / "results.jsonl")
    with open(output_file, "w", encoding="utf-8") as f:
        for result in updated_results:
            f.write(json.dumps(result, ensure_ascii=False) + "\n")

    print(f"\nUpdated {len(updated_results)} results with judge scores")
    print(f"Total tokens: {total_tokens}, Total cost: ${total_cost:.4f}")
    print(f"Results written to: {output_file}")


def aggregate_judge_metrics(run_dir: Path) -> Dict[str, Any]:
    """
    Aggregate judge metrics across all results.

    Args:
        run_dir: Path to run directory containing results.jsonl

    Returns:
        Dictionary with aggregated metrics
    """
    results = load_results(str(run_dir))

    if not results:
        return {}

    groundedness_scores = []
    correctness_scores = []
    total_tokens = 0
    total_cost = 0.0

    for result in results:
        groundedness = result.get("groundedness")
        correctness = result.get("correctness")
        cost = result.get("cost")

        if groundedness:
            groundedness_scores.append(groundedness.get("score", 0.0))

        if correctness:
            correctness_scores.append(correctness.get("score", 0.0))

        if cost:
            total_tokens += cost.get("judge_tokens", 0)
            total_cost += cost.get("judge_cost_usd", 0.0)

    aggregate = {}

    if groundedness_scores:
        aggregate["groundedness_avg"] = sum(groundedness_scores) / len(groundedness_scores)

    if correctness_scores:
        aggregate["correctness_avg"] = sum(correctness_scores) / len(correctness_scores)

    aggregate["cost"] = {
        "judge_total_usd": total_cost,
        "judge_total_tokens": total_tokens,
    }

    return aggregate


def main():
    parser = argparse.ArgumentParser(
        description="Judge answer quality (groundedness and correctness)",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument(
        "--run-id",
        type=str,
        required=True,
        help="Run ID (directory name in results/)",
    )
    parser.add_argument(
        "--results-dir",
        type=Path,
        default=Path("eval/results"),
        help="Results directory (default: eval/results)",
    )
    parser.add_argument(
        "--judge-model",
        type=str,
        required=True,
        help="Judge model (e.g., 'qwen2.5-14b', 'openai:gpt-4', 'anthropic:claude-3-5-sonnet-20241022')",
    )
    parser.add_argument(
        "--judge-base-url",
        type=str,
        help="Base URL for local LLM (required for local models, default: http://localhost:8080)",
    )
    parser.add_argument(
        "--judge-api-key",
        type=str,
        help="API key for cloud models (or use OPENAI_API_KEY/ANTHROPIC_API_KEY env vars)",
    )
    parser.add_argument(
        "--judge-temperature",
        type=float,
        default=0.0,
        help="Judge temperature (default: 0.0 for deterministic)",
    )
    parser.add_argument(
        "--judge-prompt-version",
        type=str,
        default="v1.0",
        help="Judge prompt version (default: v1.0)",
    )
    parser.add_argument(
        "--cache-dir",
        type=Path,
        default=Path("eval/cache"),
        help="Cache directory (default: eval/cache)",
    )
    parser.add_argument(
        "--spot-check",
        action="store_true",
        help="Perform reliability spot-check (re-judge random subset)",
    )
    parser.add_argument(
        "--spot-check-n",
        type=int,
        default=20,
        help="Number of results to spot-check (default: 20)",
    )
    parser.add_argument(
        "--second-judge-model",
        type=str,
        help="Second judge model for spot-check (optional, uses same judge if not provided)",
    )
    parser.add_argument(
        "--second-judge-base-url",
        type=str,
        help="Base URL for second judge (if local)",
    )
    parser.add_argument(
        "--second-judge-api-key",
        type=str,
        help="API key for second judge (if cloud)",
    )
    parser.add_argument(
        "--aggregate-only",
        action="store_true",
        help="Only compute aggregate metrics, don't update results.jsonl",
    )

    args = parser.parse_args()

    # Validate paths
    run_dir = args.results_dir / args.run_id
    if not run_dir.exists():
        print(f"Error: Run directory not found: {run_dir}", file=sys.stderr)
        sys.exit(1)

    # Determine base URL for local models
    if not args.judge_model.startswith(("openai:", "anthropic:")):
        if not args.judge_base_url:
            args.judge_base_url = os.getenv("LLM_BASE_URL", "http://localhost:8081")

    # Create judge client
    judge_client = JudgeClient(
        model=args.judge_model,
        base_url=args.judge_base_url,
        api_key=args.judge_api_key,
        temperature=args.judge_temperature,
    )

    # Create cache
    cache_file = args.cache_dir / "judge_cache.jsonl"
    cache = JudgeCache(cache_file)

    # Update results with judges (unless aggregate-only)
    if not args.aggregate_only:
        update_results_with_judges(run_dir, judge_client, args.judge_prompt_version, cache)

    # Perform spot-check if requested
    if args.spot_check:
        print("\nPerforming reliability spot-check...")
        results = load_results(str(run_dir))
        reliability = judge_reliability_spot_check(
            results,
            judge_client,
            args.judge_prompt_version,
            spot_check_n=args.spot_check_n,
            second_judge_model=args.second_judge_model,
            second_judge_base_url=args.second_judge_base_url,
            second_judge_api_key=args.second_judge_api_key,
        )
        print(f"Disagreement rate: {reliability['disagreement_rate']:.2%}")
        print(f"Spot-checked: {reliability['spot_check_n']} results")

    # Compute aggregate metrics
    aggregate = aggregate_judge_metrics(run_dir)
    if aggregate:
        print("\nAggregate Judge Metrics:")
        print(json.dumps(aggregate, indent=2, ensure_ascii=False))


if __name__ == "__main__":
    main()

