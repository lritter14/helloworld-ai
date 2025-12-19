#!/usr/bin/env python3
"""
Full evaluation pipeline entry point.

Runs all evaluation scripts in the correct order:
1. run_eval.py - Execute evaluation suite against Go API
2. score_retrieval.py - Compute retrieval metrics
3. judge_answers.py - Judge answer quality (optional, if judge model provided)
4. score_abstention.py - Compute abstention metrics

Usage:
    # Full evaluation with judges
    python eval/scripts/run_full_eval.py \
        --eval-set eval/eval_set.jsonl \
        --judge-model qwen2.5-14b

    # Retrieval-only (fast, no judge cost)
    python eval/scripts/run_full_eval.py \
        --eval-set eval/eval_set.jsonl \
        --retrieval-only

    # Custom configuration
    python eval/scripts/run_full_eval.py \
        --eval-set eval/eval_set.jsonl \
        --k 10 \
        --judge-model qwen2.5-14b \
        --judge-base-url http://localhost:8081
"""

import argparse
import json
import os
import subprocess
import sys
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Dict, List, Optional

try:
    import requests
except ImportError:
    print("Error: 'requests' library is required. Install with: pip install requests")
    sys.exit(1)

# Import storage module for loading config and metrics
sys.path.insert(0, str(Path(__file__).parent))
from storage import load_config, load_metrics


def check_api_health(api_url: str, timeout: int = 10) -> tuple[bool, str]:
    """
    Check if the API is healthy and accessible.

    Args:
        api_url: Base URL of the API
        timeout: Request timeout in seconds

    Returns:
        Tuple of (success: bool, message: str)
    """
    try:
        url = f"{api_url.rstrip('/')}/api/health"
        response = requests.get(url, timeout=timeout)
        response.raise_for_status()
        
        health_data = response.json()
        status = health_data.get("status", "unknown")
        
        if status == "healthy":
            return True, "API is healthy"
        elif status == "degraded":
            issues = health_data.get("issues", [])
            return False, f"API is degraded: {', '.join(issues)}"
        else:
            issues = health_data.get("issues", [])
            return False, f"API is unhealthy: {', '.join(issues)}"
    except requests.exceptions.ConnectionError:
        return False, f"Cannot connect to API at {api_url}. Is the server running?"
    except requests.exceptions.Timeout:
        return False, f"API health check timed out after {timeout} seconds"
    except requests.exceptions.RequestException as e:
        return False, f"API health check failed: {e}"
    except Exception as e:
        return False, f"Unexpected error checking API health: {e}"


def check_judge_llm_connectivity(
    judge_model: str,
    judge_base_url: Optional[str] = None,
    judge_api_key: Optional[str] = None,
    timeout: int = 10,
) -> tuple[bool, str]:
    """
    Check if the judge LLM is accessible and the model is available.

    Args:
        judge_model: Judge model identifier (e.g., "qwen2.5-14b", "openai:gpt-4")
        judge_base_url: Base URL for local LLM (required for local models)
        judge_api_key: API key for cloud models
        timeout: Request timeout in seconds

    Returns:
        Tuple of (success: bool, message: str)
    """
    # Determine model type
    if judge_model.startswith("openai:"):
        return check_openai_model(judge_model, judge_api_key, timeout)
    elif judge_model.startswith("anthropic:"):
        return check_anthropic_model(judge_model, judge_api_key, timeout)
    else:
        # Local model
        if not judge_base_url:
            judge_base_url = os.getenv("LLM_BASE_URL", "http://localhost:8081")
        return check_local_model(judge_model, judge_base_url, timeout)


def load_local_model(model_name: str, base_url: str, timeout: int = 10) -> tuple[bool, str]:
    """
    Load a model into the llama.cpp server cache.
    
    Note: This function should only be called if the model is confirmed to NOT be in cache.

    Args:
        model_name: Model name to load (should match the ID from /models endpoint)
        base_url: Base URL of the llama.cpp server
        timeout: Request timeout in seconds

    Returns:
        Tuple of (success: bool, message: str)
    """
    try:
        load_url = f"{base_url.rstrip('/')}/models/load"
        
        # Try with empty extra_args first (standard format)
        payload = {"model": model_name, "extra_args": []}
        
        response = requests.post(load_url, json=payload, timeout=timeout)
        
        # If we get a 400 error, check if it's because model is already loaded
        if response.status_code == 400:
            try:
                error_data = response.json()
                error_msg = error_data.get("error", response.text)
                # Handle case where error might be a dict (nested error object)
                if isinstance(error_msg, dict):
                    error_msg = error_msg.get("message", str(error_msg))
                # Ensure error_msg is a string
                error_msg = str(error_msg)
                # Check if error indicates model is already loaded
                # If so, this is actually a success - the model is ready to use
                if "already" in error_msg.lower() or "loaded" in error_msg.lower():
                    return True, f"Model is already loaded (detected from load response)"
                # Try without extra_args as fallback
                payload_no_args = {"model": model_name}
                retry_response = requests.post(load_url, json=payload_no_args, timeout=timeout)
                if retry_response.status_code == 200:
                    load_data = retry_response.json()
                    if load_data.get("success", False):
                        # Continue with polling below
                        pass
                    else:
                        return False, f"Bad request (400): {error_msg}. Retry also failed: {load_data.get('error', 'Unknown')}"
                else:
                    return False, f"Bad request (400): {error_msg}. Model name might be incorrect or server requires different format."
            except (ValueError, KeyError, requests.exceptions.RequestException):
                return False, f"Bad request (400): {response.text[:200]}. Model name might be incorrect."
        
        if response.status_code != 200:
            response.raise_for_status()
        
        load_data = response.json()
        if not load_data.get("success", False):
            error = load_data.get("error", "Unknown error")
            return False, f"Failed to load model: {error}"
        
        # Model loading is asynchronous, so we need to poll
        # Wait up to 30 seconds, checking every second
        models_url = f"{base_url.rstrip('/')}/models"
        max_wait = 30
        for attempt in range(max_wait):
            time.sleep(1)
            status_response = requests.get(models_url, timeout=5)
            if status_response.status_code == 200:
                models_data = status_response.json()
                models_list = models_data.get("data", [])
                
                for model in models_list:
                    if model.get("id") == model_name:
                        if model.get("in_cache", False):
                            return True, f"Model '{model_name}' loaded successfully"
                        # Check if loading failed
                        status = model.get("status", {})
                        if status.get("failed", False):
                            return False, f"Model '{model_name}' failed to load"
        
        return False, f"Model '{model_name}' did not load within {max_wait} seconds"
        
    except requests.exceptions.RequestException as e:
        return False, f"Failed to load model: {e}"
    except Exception as e:
        return False, f"Unexpected error loading model: {e}"


def check_local_model(model_name: str, base_url: str, timeout: int = 10) -> tuple[bool, str]:
    """
    Check if a local LLM model is available. If not loaded, attempts to load it.

    Args:
        model_name: Model name to check
        base_url: Base URL of the llama.cpp server
        timeout: Request timeout in seconds

    Returns:
        Tuple of (success: bool, message: str)
    """
    try:
        # First, check if server is reachable by listing models
        models_url = f"{base_url.rstrip('/')}/models"
        response = requests.get(models_url, timeout=timeout)
        response.raise_for_status()
        
        models_data = response.json()
        models_list = models_data.get("data", [])
        
        # Check if our model is in the list and if it's already loaded
        model_found = False
        model_in_cache = False
        
        for model in models_list:
            if model.get("id") == model_name:
                model_found = True
                model_in_cache = model.get("in_cache", False)
                break
        
        if not model_found:
            return False, f"Model '{model_name}' not found in server. Available models: {[m.get('id') for m in models_list]}"
        
        # Only attempt to load if model is confirmed to NOT be in cache
        # This prevents unnecessary load requests and 400 errors for already-loaded models
        if not model_in_cache:
            print(f"  Model '{model_name}' is not loaded. Attempting to load...")
            load_ok, load_msg = load_local_model(model_name, base_url, timeout)
            load_msg_str = str(load_msg).lower()
            
            if load_ok:
                # Check if this is "already loaded" success vs "just loaded" success
                if "already loaded" in load_msg_str:
                    # Model was already loaded - no need to wait, just confirm it's in cache
                    print(f"  {load_msg}")
                    model_in_cache = True
                else:
                    # Model was just loaded - wait for it to stabilize
                    print(f"  {load_msg}. Waiting 10 seconds for model to stabilize...")
                    time.sleep(10)
                    # Re-check after loading
                    response = requests.get(models_url, timeout=timeout)
                    response.raise_for_status()
                    models_data = response.json()
                    models_list = models_data.get("data", [])
                    for model in models_list:
                        if model.get("id") == model_name:
                            model_in_cache = model.get("in_cache", False)
                            break
                    if not model_in_cache:
                        return False, f"Model '{model_name}' was loaded but is not in cache after waiting"
            else:
                # Load failed - check if it's because model is already loaded
                if "already" in load_msg_str or "loaded" in load_msg_str:
                    # Model is already loaded - treat as success
                    print(f"  Model '{model_name}' is already loaded (server confirmed)")
                    model_in_cache = True
                else:
                    # Actual failure
                    return False, load_msg
        else:
            # Model is already loaded, skip load request
            pass
        
        # Try a simple test call to verify the model works
        test_url = f"{base_url.rstrip('/')}/v1/chat/completions"
        test_payload = {
            "model": model_name,
            "messages": [{"role": "user", "content": "test"}],
            "max_tokens": 5,
        }
        
        test_response = requests.post(test_url, json=test_payload, timeout=timeout)
        if test_response.status_code == 200:
            return True, f"Model '{model_name}' is available and responding"
        else:
            return False, f"Model '{model_name}' is listed but not responding (status {test_response.status_code})"
            
    except requests.exceptions.ConnectionError:
        return False, f"Cannot connect to LLM server at {base_url}. Is the server running?"
    except requests.exceptions.Timeout:
        return False, f"LLM connectivity check timed out after {timeout} seconds"
    except requests.exceptions.RequestException as e:
        return False, f"LLM connectivity check failed: {e}"
    except Exception as e:
        return False, f"Unexpected error checking LLM connectivity: {e}"


def check_openai_model(model_name: str, api_key: Optional[str], timeout: int = 10) -> tuple[bool, str]:
    """
    Check if an OpenAI model is accessible.

    Args:
        model_name: Model identifier (e.g., "openai:gpt-4")
        api_key: API key (or will check OPENAI_API_KEY env var)
        timeout: Request timeout in seconds

    Returns:
        Tuple of (success: bool, message: str)
    """
    if not api_key:
        api_key = os.getenv("OPENAI_API_KEY")
    
    if not api_key:
        return False, "OPENAI_API_KEY environment variable or --judge-api-key required for OpenAI models"
    
    try:
        # Try importing openai library
        try:
            import openai
        except ImportError:
            return False, "openai library is required. Install with: pip install openai"
        
        # Extract actual model name (remove "openai:" prefix)
        actual_model = model_name.replace("openai:", "")
        
        # Make a simple test call
        client = openai.OpenAI(api_key=api_key, timeout=timeout)
        response = client.chat.completions.create(
            model=actual_model,
            messages=[{"role": "user", "content": "test"}],
            max_tokens=5,
        )
        
        if response.choices and len(response.choices) > 0:
            return True, f"OpenAI model '{actual_model}' is accessible"
        else:
            return False, f"OpenAI model '{actual_model}' returned empty response"
            
    except Exception as e:
        error_msg = str(e)
        if "authentication" in error_msg.lower() or "invalid" in error_msg.lower() or "api key" in error_msg.lower():
            return False, "OpenAI API key is invalid or expired"
        elif "not found" in error_msg.lower() or "does not exist" in error_msg.lower():
            return False, f"OpenAI model '{actual_model}' not found or not accessible"
        else:
            return False, f"Error checking OpenAI model: {e}"


def check_anthropic_model(model_name: str, api_key: Optional[str], timeout: int = 10) -> tuple[bool, str]:
    """
    Check if an Anthropic model is accessible.

    Args:
        model_name: Model identifier (e.g., "anthropic:claude-3-5-sonnet-20241022")
        api_key: API key (or will check ANTHROPIC_API_KEY env var)
        timeout: Request timeout in seconds

    Returns:
        Tuple of (success: bool, message: str)
    """
    if not api_key:
        api_key = os.getenv("ANTHROPIC_API_KEY")
    
    if not api_key:
        return False, "ANTHROPIC_API_KEY environment variable or --judge-api-key required for Anthropic models"
    
    try:
        # Try importing anthropic library
        try:
            import anthropic
        except ImportError:
            return False, "anthropic library is required. Install with: pip install anthropic"
        
        # Extract actual model name (remove "anthropic:" prefix)
        actual_model = model_name.replace("anthropic:", "")
        
        # Make a simple test call
        client = anthropic.Anthropic(api_key=api_key, timeout=timeout)
        response = client.messages.create(
            model=actual_model,
            max_tokens=5,
            messages=[{"role": "user", "content": "test"}],
        )
        
        if response.content:
            return True, f"Anthropic model '{actual_model}' is accessible"
        else:
            return False, f"Anthropic model '{actual_model}' returned empty response"
            
    except Exception as e:
        error_msg = str(e)
        if "authentication" in error_msg.lower() or "invalid" in error_msg.lower() or "api key" in error_msg.lower():
            return False, "Anthropic API key is invalid or expired"
        elif "not found" in error_msg.lower() or "does not exist" in error_msg.lower():
            return False, f"Anthropic model '{actual_model}' not found or not accessible"
        else:
            return False, f"Error checking Anthropic model: {e}"


def run_connectivity_tests(args: argparse.Namespace) -> bool:
    """
    Run all connectivity tests before starting the evaluation pipeline.

    Args:
        args: Parsed command-line arguments

    Returns:
        True if all tests pass, False otherwise
    """
    print("\n" + "=" * 70)
    print("Connectivity Tests")
    print("=" * 70)
    print()
    
    all_passed = True
    
    # Test 1: API health check
    print("Testing API connectivity...")
    api_ok, api_msg = check_api_health(args.api_url)
    if api_ok:
        print(f"  ✓ {api_msg}")
    else:
        print(f"  ✗ {api_msg}")
        all_passed = False
    print()
    
    # Test 2: Judge LLM connectivity (if needed)
    if not args.retrieval_only and not args.skip_judges and args.judge_model:
        print("Testing judge LLM connectivity...")
        judge_ok, judge_msg = check_judge_llm_connectivity(
            args.judge_model,
            args.judge_base_url,
            args.judge_api_key,
        )
        if judge_ok:
            print(f"  ✓ {judge_msg}")
        else:
            print(f"  ✗ {judge_msg}")
            all_passed = False
        print()
    
    if all_passed:
        print("All connectivity tests passed!")
        print()
        return True
    else:
        print("=" * 70)
        print("ERROR: Some connectivity tests failed.")
        print("Please fix the issues above before running the evaluation.")
        print("=" * 70)
        print()
        return False


def run_command(cmd: List[str], description: str) -> bool:
    """
    Run a command and return True if successful, False otherwise.

    Args:
        cmd: Command to run as list of strings
        description: Description of what the command does

    Returns:
        True if command succeeded, False otherwise
    """
    print(f"\n{'='*70}")
    print(f"Step: {description}")
    print(f"{'='*70}")
    print(f"Running: {' '.join(cmd)}")
    print()

    try:
        result = subprocess.run(cmd, check=True)
        print(f"\n✓ {description} completed successfully")
        return True
    except subprocess.CalledProcessError as e:
        print(f"\n✗ {description} failed with exit code {e.returncode}", file=sys.stderr)
        return False
    except KeyboardInterrupt:
        print(f"\n✗ {description} interrupted by user", file=sys.stderr)
        return False


def get_last_run_info(results_dir: Path) -> Optional[Dict]:
    """
    Get information about the last run for comparison.

    Args:
        results_dir: Results directory path

    Returns:
        Dictionary with last run info (run_id, config, metrics) or None
    """
    if not results_dir.exists():
        return None

    # Get all run directories, sorted by modification time (newest first)
    run_dirs = sorted(
        [d for d in results_dir.iterdir() if d.is_dir()],
        key=lambda x: x.stat().st_mtime,
        reverse=True,
    )

    if len(run_dirs) < 2:  # Need at least 2 runs (current + previous)
        return None

    # Get the second most recent run (previous run)
    last_run_dir = run_dirs[1]
    last_run_id = last_run_dir.name

    try:
        config = load_config(str(last_run_dir))
        metrics = load_metrics(str(last_run_dir))
        return {
            "run_id": last_run_id,
            "config": config,
            "metrics": metrics,
        }
    except Exception:
        return None


def prompt_for_description(last_run_info: Optional[Dict]) -> str:
    """
    Prompt user for qualitative description of this run.

    Args:
        last_run_info: Information about the last run (if available)

    Returns:
        User's description string
    """
    print("\n" + "=" * 70)
    print("Run Description")
    print("=" * 70)

    if last_run_info:
        print(f"\nLast run: {last_run_info['run_id']}")
        if last_run_info.get("config"):
            config = last_run_info["config"]
            print(f"  K: {config.get('k', 'N/A')}")
            print(f"  Folder mode: {config.get('folder_mode', 'N/A')}")
            if config.get("judge_model"):
                print(f"  Judge model: {config.get('judge_model', 'N/A')}")
        print()

    print(
        "Please provide a qualitative description of what is being tested in this run"
    )
    if last_run_info:
        print("compared to the last run. This will be included in the summary.")
    else:
        print("This will be included in the summary.")
    print("\nExamples:")
    print("  - 'Testing increased K value from 5 to 10 to improve recall'")
    print("  - 'Comparing new embedding model granite-278m vs previous model'")
    print("  - 'Testing folder selection fallback strategy improvements'")
    print("  - 'Baseline run with default settings'")
    print()

    description = input("Description: ").strip()

    if not description:
        description = "No description provided"

    return description


def create_summary_md(
    run_dir: Path,
    run_id: str,
    description: str,
    args: argparse.Namespace,
    last_run_info: Optional[Dict],
) -> None:
    """
    Create a summary.md file with run details and metrics.

    Args:
        run_dir: Run directory path
        run_id: Run ID
        description: User's qualitative description
        args: Command-line arguments
        last_run_info: Information about the last run (if available)
    """
    # Load config and metrics for this run
    config = load_config(str(run_dir))
    metrics = load_metrics(str(run_dir))

    # Build summary content
    summary_lines = [
        "# Evaluation Run Summary",
        "",
        f"**Run ID**: `{run_id}`",
        f"**Timestamp**: {datetime.now(timezone.utc).strftime('%Y-%m-%d %H:%M:%S UTC')}",
        "",
        "## Run Description",
        "",
        description,
        "",
        "## Configuration",
        "",
    ]

    # Add configuration details
    if config:
        summary_lines.extend([
            "### RAG Parameters",
            "",
            f"- **K**: {config.get('k', 'N/A')}",
            f"- **Rerank Weights**: Vector={config.get('rerank_weights', {}).get('vector', 'N/A')}, Lexical={config.get('rerank_weights', {}).get('lexical', 'N/A')}",
            f"- **Folder Mode**: {config.get('folder_mode', 'N/A')}",
            "",
            "### Models",
            "",
        ])

        if config.get("llm_model"):
            summary_lines.append(f"- **LLM Model**: {config.get('llm_model')}")
        else:
            summary_lines.append("- **LLM Model**: Not specified")

        if config.get("embedding_model"):
            summary_lines.append(f"- **Embedding Model**: {config.get('embedding_model')}")
        else:
            summary_lines.append("- **Embedding Model**: Not specified")

        if config.get("judge_model"):
            summary_lines.extend([
                f"- **Judge Model**: {config.get('judge_model')}",
                f"- **Judge Prompt Version**: {config.get('judge_prompt_version', 'N/A')}",
                f"- **Judge Temperature**: {config.get('judge_temperature', 'N/A')}",
            ])
        else:
            summary_lines.append("- **Judge Model**: Not used (retrieval-only mode)")

        summary_lines.extend([
            "",
            "### Dataset",
            "",
            f"- **Eval Set**: {args.eval_set}",
        ])

        if config.get("dataset_version"):
            summary_lines.append(f"- **Eval Set Commit Hash**: `{config.get('dataset_version')}`")

        summary_lines.append("")

    # Add metrics breakdown
    summary_lines.extend([
        "## Metrics Overview",
        "",
        "The evaluation framework tracks the following metrics:",
        "",
        "### Retrieval Metrics",
        "",
        "These metrics measure whether the system successfully found the relevant content:",
        "",
        "- **Recall@K**: Did we retrieve at least one chunk that matches the gold supports? (Binary: 0 or 1)",
        "- **MRR (Mean Reciprocal Rank)**: How high was the first correct chunk ranked? (0-1, where 1.0 = first rank)",
        "- **Precision@K**: Fraction of top K chunks that match any gold_support anchor (0-1)",
        "- **Scope Miss Rate**: Fraction of cases where folder selection excluded all gold supports (0-1)",
        "- **Attribution Hit Rate**: Did the final cited references include at least one matching gold_support? (Binary: 0 or 1)",
        "",
        "### Answer Quality Metrics",
        "",
        "These metrics measure whether the generated answer is correct and well-supported:",
        "",
        "- **Groundedness (0-5)**: Are all claims in the answer supported by the provided context?",
        "  - Score of 5 requires citations for all major claims",
        "- **Correctness (0-5)**: Does the answer correctly address the question?",
        "",
        "### Abstention Metrics",
        "",
        "These metrics measure whether the system knows when not to answer:",
        "",
        "- **Abstention Accuracy**: When answerable=false, did the model refuse? (Binary: 0 or 1)",
        "- **Hallucination Rate on Unanswerable**: When answerable=false, did it confidently answer anyway? (Binary: 0 or 1)",
        "",
    ])

    # Add actual metrics if available
    if metrics and metrics.get("aggregate_metrics"):
        agg_metrics = metrics["aggregate_metrics"]
        summary_lines.extend([
            "## Results Summary",
            "",
        ])

        # Retrieval metrics
        if "recall_at_k_avg" in agg_metrics:
            summary_lines.extend([
                "### Retrieval Metrics",
                "",
                f"- **Average Recall@K**: {agg_metrics.get('recall_at_k_avg', 'N/A'):.3f}",
            ])

        if "mrr_avg" in agg_metrics:
            summary_lines.append(f"- **Average MRR**: {agg_metrics.get('mrr_avg', 'N/A'):.3f}")

        if "precision_at_k_avg" in agg_metrics:
            summary_lines.append(f"- **Average Precision@K**: {agg_metrics.get('precision_at_k_avg', 'N/A'):.3f}")

        if "scope_miss_rate" in agg_metrics:
            summary_lines.append(f"- **Scope Miss Rate**: {agg_metrics.get('scope_miss_rate', 'N/A'):.3f}")

        if "attribution_hit_rate" in agg_metrics:
            summary_lines.append(f"- **Attribution Hit Rate**: {agg_metrics.get('attribution_hit_rate', 'N/A'):.3f}")

        summary_lines.append("")

        # Answer quality metrics
        if "groundedness_avg" in agg_metrics or "correctness_avg" in agg_metrics:
            summary_lines.extend([
                "### Answer Quality Metrics",
                "",
            ])

            if "groundedness_avg" in agg_metrics:
                summary_lines.append(f"- **Average Groundedness**: {agg_metrics.get('groundedness_avg', 'N/A'):.2f}/5.0")

            if "correctness_avg" in agg_metrics:
                summary_lines.append(f"- **Average Correctness**: {agg_metrics.get('correctness_avg', 'N/A'):.2f}/5.0")

            summary_lines.append("")

        # Abstention metrics
        if "abstention_accuracy" in agg_metrics:
            summary_lines.extend([
                "### Abstention Metrics",
                "",
                f"- **Abstention Accuracy**: {agg_metrics.get('abstention_accuracy', 'N/A'):.3f}",
            ])

            if "hallucination_rate_unanswerable" in agg_metrics:
                summary_lines.append(f"- **Hallucination Rate on Unanswerable**: {agg_metrics.get('hallucination_rate_unanswerable', 'N/A'):.3f}")

            summary_lines.append("")

        # Operational metrics
        if "operational_metrics" in agg_metrics:
            op_metrics = agg_metrics["operational_metrics"]
            summary_lines.extend([
                "### Operational Metrics",
                "",
                f"- **Error Rate**: {op_metrics.get('error_rate', 'N/A'):.2%}",
                f"- **Timeout Rate**: {op_metrics.get('timeout_rate', 'N/A'):.2%}",
                f"- **Empty Response Rate**: {op_metrics.get('empty_response_rate', 'N/A'):.2%}",
                "",
            ])

        # Latency
        if "latency" in agg_metrics:
            latency = agg_metrics["latency"]
            summary_lines.extend([
                "### Performance",
                "",
                f"- **Latency p50**: {latency.get('p50_ms', 'N/A'):.0f}ms",
                f"- **Latency p95**: {latency.get('p95_ms', 'N/A'):.0f}ms",
                "",
            ])

    # Add comparison to last run if available
    if last_run_info:
        summary_lines.extend([
            "## Comparison to Previous Run",
            "",
            f"**Previous Run ID**: `{last_run_info['run_id']}`",
            "",
        ])

        if last_run_info.get("config"):
            prev_config = last_run_info["config"]
            summary_lines.append("### Configuration Changes")
            summary_lines.append("")

            if config and prev_config:
                if config.get("k") != prev_config.get("k"):
                    summary_lines.append(f"- **K**: {prev_config.get('k', 'N/A')} → {config.get('k', 'N/A')}")

                if config.get("folder_mode") != prev_config.get("folder_mode"):
                    summary_lines.append(f"- **Folder Mode**: {prev_config.get('folder_mode', 'N/A')} → {config.get('folder_mode', 'N/A')}")

                if config.get("judge_model") != prev_config.get("judge_model"):
                    summary_lines.append(f"- **Judge Model**: {prev_config.get('judge_model', 'N/A')} → {config.get('judge_model', 'N/A')}")

                if config.get("embedding_model") != prev_config.get("embedding_model"):
                    summary_lines.append(f"- **Embedding Model**: {prev_config.get('embedding_model', 'N/A')} → {config.get('embedding_model', 'N/A')}")

            summary_lines.append("")

        if last_run_info.get("metrics") and metrics:
            prev_metrics = last_run_info["metrics"].get("aggregate_metrics", {})
            curr_metrics = metrics.get("aggregate_metrics", {})
            summary_lines.append("### Metric Changes")
            summary_lines.append("")

            if "recall_at_k_avg" in curr_metrics and "recall_at_k_avg" in prev_metrics:
                delta = curr_metrics["recall_at_k_avg"] - prev_metrics["recall_at_k_avg"]
                summary_lines.append(f"- **Recall@K**: {prev_metrics['recall_at_k_avg']:.3f} → {curr_metrics['recall_at_k_avg']:.3f} ({delta:+.3f})")

            if "mrr_avg" in curr_metrics and "mrr_avg" in prev_metrics:
                delta = curr_metrics["mrr_avg"] - prev_metrics["mrr_avg"]
                summary_lines.append(f"- **MRR**: {prev_metrics['mrr_avg']:.3f} → {curr_metrics['mrr_avg']:.3f} ({delta:+.3f})")

            if "groundedness_avg" in curr_metrics and "groundedness_avg" in prev_metrics:
                delta = curr_metrics["groundedness_avg"] - prev_metrics["groundedness_avg"]
                summary_lines.append(f"- **Groundedness**: {prev_metrics['groundedness_avg']:.2f} → {curr_metrics['groundedness_avg']:.2f} ({delta:+.2f})")

            summary_lines.append("")

    # Add file references
    summary_lines.extend([
        "## Files",
        "",
        "Detailed results are available in:",
        "",
        f"- `results.jsonl`: Individual test results with full detail",
        f"- `metrics.json`: Aggregated metrics in JSON format",
        f"- `config.json`: Run configuration snapshot",
        "",
    ])

    # Write summary file
    summary_file = run_dir / "summary.md"
    with open(summary_file, "w", encoding="utf-8") as f:
        f.write("\n".join(summary_lines))

    print(f"\nSummary written to: {summary_file}")


def main():
    parser = argparse.ArgumentParser(
        description="Run full evaluation pipeline",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Full evaluation with judges
  python eval/scripts/run_full_eval.py --eval-set eval/eval_set.jsonl --judge-model qwen2.5-14b

  # Retrieval-only (fast, no judge cost)
  python eval/scripts/run_full_eval.py --eval-set eval/eval_set.jsonl --retrieval-only

  # Limit to 10 test cases for faster iteration
  python eval/scripts/run_full_eval.py --eval-set eval/eval_set.jsonl --limit 10 --judge-model qwen2.5-14b

  # Custom configuration
  python eval/scripts/run_full_eval.py \\
      --eval-set eval/eval_set.jsonl \\
      --k 10 \\
      --judge-model qwen2.5-14b \\
      --judge-base-url http://localhost:8081
        """,
    )

    # Common arguments
    parser.add_argument(
        "--eval-set",
        type=Path,
        default=Path("eval/eval_set.jsonl"),
        help="Path to eval_set.jsonl file (default: eval/eval_set.jsonl)",
    )
    parser.add_argument(
        "--results-dir",
        type=Path,
        default=Path("eval/results"),
        help="Results directory (default: eval/results)",
    )

    # run_eval.py arguments
    parser.add_argument(
        "--api-url",
        type=str,
        default="http://localhost:9000",
        help="Base URL of the API (default: http://localhost:9000)",
    )
    parser.add_argument(
        "--k",
        type=int,
        default=5,
        help="Number of chunks to retrieve (default: 5)",
    )
    parser.add_argument(
        "--rerank-vector-weight",
        type=float,
        default=0.7,
        help="Vector weight for reranking (default: 0.7)",
    )
    parser.add_argument(
        "--rerank-lexical-weight",
        type=float,
        default=0.3,
        help="Lexical weight for reranking (default: 0.3)",
    )
    parser.add_argument(
        "--folder-mode",
        type=str,
        choices=["off", "on", "on_with_fallback"],
        default="on_with_fallback",
        help="Folder selection mode (default: on_with_fallback)",
    )
    parser.add_argument(
        "--retrieval-only",
        action="store_true",
        help="Run retrieval metrics only (skip judge calls, faster iteration)",
    )
    parser.add_argument(
        "--store-full-text",
        action="store_true",
        help="Store full chunk text (default: truncate to 200 chars)",
    )
    parser.add_argument(
        "--timeout",
        type=int,
        default=120,
        help="Request timeout in seconds (default: 120)",
    )
    parser.add_argument(
        "--llm-model",
        type=str,
        help="LLM model name (for config tracking)",
    )
    parser.add_argument(
        "--embedding-model",
        type=str,
        help="Embedding model name (for config tracking)",
    )
    parser.add_argument(
        "--limit",
        type=int,
        help="Limit number of test cases to process (for faster iteration)",
    )

    # judge_answers.py arguments
    parser.add_argument(
        "--judge-model",
        type=str,
        help="Judge model (e.g., qwen2.5-14b, openai:gpt-4). Required unless --retrieval-only.",
    )
    parser.add_argument(
        "--judge-base-url",
        type=str,
        help="Base URL for local LLM (default: http://localhost:8081)",
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
        "--skip-judges",
        action="store_true",
        help="Skip judge step even if judge-model is provided",
    )

    # score_retrieval.py and score_abstention.py arguments
    parser.add_argument(
        "--skip-retrieval-metrics",
        action="store_true",
        help="Skip retrieval metrics computation",
    )
    parser.add_argument(
        "--skip-abstention-metrics",
        action="store_true",
        help="Skip abstention metrics computation",
    )
    parser.add_argument(
        "--skip-description-prompt",
        action="store_true",
        help="Skip the interactive description prompt (use 'No description provided' as default)",
    )
    parser.add_argument(
        "--description",
        type=str,
        help="Provide run description directly (skips interactive prompt)",
    )

    args = parser.parse_args()

    # Validate arguments
    if not args.eval_set.exists():
        print(f"Error: Eval set file not found: {args.eval_set}", file=sys.stderr)
        sys.exit(1)

    # Run connectivity tests before starting evaluation
    if not run_connectivity_tests(args):
        print("Connectivity tests failed. Exiting.", file=sys.stderr)
        sys.exit(1)

    # Get last run info for comparison context
    last_run_info = get_last_run_info(args.results_dir)

    # Get run description
    if args.description:
        description = args.description
    elif args.skip_description_prompt:
        description = "No description provided"
    else:
        description = prompt_for_description(last_run_info)

    # Determine if we should run judges
    run_judges = not args.retrieval_only and not args.skip_judges and args.judge_model

    if not args.retrieval_only and not args.skip_judges and not args.judge_model:
        print(
            "Warning: No judge model provided. Running in retrieval-only mode.",
            file=sys.stderr,
        )
        print(
            "  Use --judge-model to enable answer quality judging, or --retrieval-only to skip.",
            file=sys.stderr,
        )

    # Get script directory
    script_dir = Path(__file__).parent

    # Step 1: Run evaluation suite
    run_eval_cmd = [
        sys.executable,
        str(script_dir / "run_eval.py"),
        "--eval-set",
        str(args.eval_set),
        "--api-url",
        args.api_url,
        "--k",
        str(args.k),
        "--rerank-vector-weight",
        str(args.rerank_vector_weight),
        "--rerank-lexical-weight",
        str(args.rerank_lexical_weight),
        "--folder-mode",
        args.folder_mode,
        "--output-dir",
        str(args.results_dir),
        "--timeout",
        str(args.timeout),
    ]

    if args.retrieval_only:
        run_eval_cmd.append("--retrieval-only")
    else:
        if args.judge_model:
            run_eval_cmd.extend(["--judge-model", args.judge_model])
        if args.judge_temperature:
            run_eval_cmd.extend(["--judge-temperature", str(args.judge_temperature)])
        if args.judge_prompt_version:
            run_eval_cmd.extend(["--judge-prompt-version", args.judge_prompt_version])

    if args.store_full_text:
        run_eval_cmd.append("--store-full-text")

    if args.llm_model:
        run_eval_cmd.extend(["--llm-model", args.llm_model])
    if args.embedding_model:
        run_eval_cmd.extend(["--embedding-model", args.embedding_model])
    if args.limit:
        run_eval_cmd.extend(["--limit", str(args.limit)])

    # Extract run_id from run_eval output (we'll need to parse it)
    # For now, we'll use a timestamp-based approach or read from the latest run
    # Actually, run_eval creates a timestamp-based run_id, so we need to capture it
    # Let's modify the approach: run_eval will output the run_id, or we can find the latest run

    if not run_command(run_eval_cmd, "Running evaluation suite"):
        print("\nEvaluation suite failed. Stopping pipeline.", file=sys.stderr)
        sys.exit(1)

    # Find the latest run_id (most recent directory in results/)
    results_dir = args.results_dir
    if not results_dir.exists():
        print(f"Error: Results directory not found: {results_dir}", file=sys.stderr)
        sys.exit(1)

    # Get all run directories, sorted by modification time (newest first)
    run_dirs = sorted(
        [d for d in results_dir.iterdir() if d.is_dir()],
        key=lambda x: x.stat().st_mtime,
        reverse=True,
    )

    if not run_dirs:
        print("Error: No run directories found in results/", file=sys.stderr)
        sys.exit(1)

    run_id = run_dirs[0].name
    print(f"\nUsing run_id: {run_id}")

    # Step 2: Compute retrieval metrics
    if not args.skip_retrieval_metrics:
        score_retrieval_cmd = [
            sys.executable,
            str(script_dir / "score_retrieval.py"),
            "--run-id",
            run_id,
            "--results-dir",
            str(args.results_dir),
            "--eval-set",
            str(args.eval_set),
        ]

        if not run_command(score_retrieval_cmd, "Computing retrieval metrics"):
            print("\nRetrieval metrics computation failed. Continuing anyway...", file=sys.stderr)

    # Step 3: Judge answers (optional)
    if run_judges:
        judge_cmd = [
            sys.executable,
            str(script_dir / "judge_answers.py"),
            "--run-id",
            run_id,
            "--results-dir",
            str(args.results_dir),
            "--judge-model",
            args.judge_model,
            "--judge-prompt-version",
            args.judge_prompt_version,
            "--judge-temperature",
            str(args.judge_temperature),
            "--cache-dir",
            str(args.cache_dir),
        ]

        if args.judge_base_url:
            judge_cmd.extend(["--judge-base-url", args.judge_base_url])
        if args.judge_api_key:
            judge_cmd.extend(["--judge-api-key", args.judge_api_key])

        if not run_command(judge_cmd, "Judging answer quality"):
            print("\nAnswer judging failed. Continuing anyway...", file=sys.stderr)

    # Step 4: Compute abstention metrics
    if not args.skip_abstention_metrics:
        score_abstention_cmd = [
            sys.executable,
            str(script_dir / "score_abstention.py"),
            "--run-id",
            run_id,
            "--results-dir",
            str(args.results_dir),
            "--eval-set",
            str(args.eval_set),
        ]

        if not run_command(score_abstention_cmd, "Computing abstention metrics"):
            print("\nAbstention metrics computation failed. Continuing anyway...", file=sys.stderr)

    # Create summary.md
    run_dir = args.results_dir / run_id
    create_summary_md(run_dir, run_id, description, args, last_run_info)

    # Summary
    print(f"\n{'='*70}")
    print("Evaluation Pipeline Complete")
    print(f"{'='*70}")
    print(f"Run ID: {run_id}")
    print(f"Results directory: {run_dir}")
    print(f"\nResults files:")
    print(f"  - results.jsonl: Individual test results")
    print(f"  - metrics.json: Aggregated metrics")
    print(f"  - config.json: Run configuration")
    print(f"  - summary.md: Run summary with description and metrics")
    print()


if __name__ == "__main__":
    main()

