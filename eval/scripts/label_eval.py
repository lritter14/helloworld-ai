#!/usr/bin/env python3
"""
Labeling workflow script for marking gold_supports in eval_set.jsonl.

This script helps create ground truth for retrieval metrics by marking which
content supports the answer. Uses anchor-based labeling (rel_path + heading_path)
that is resilient to chunking changes.

Usage:
    python eval/scripts/label_eval.py --eval-set eval/eval_set.jsonl --api-url http://localhost:9000
"""

import argparse
import json
import sys
from pathlib import Path
from typing import Any, Dict, List, Optional, Set

try:
    import requests
except ImportError:
    print("Error: 'requests' library is required. Install with: pip install requests")
    sys.exit(1)


def load_eval_set(eval_set_path: Path) -> List[Dict[str, Any]]:
    """Load test cases from JSONL file."""
    test_cases = []
    with open(eval_set_path, "r", encoding="utf-8") as f:
        for line_num, line in enumerate(f, 1):
            line = line.strip()
            if not line:
                continue
            try:
                test_case = json.loads(line)
                test_cases.append(test_case)
            except json.JSONDecodeError as e:
                print(f"Warning: Failed to parse line {line_num} in {eval_set_path}: {e}")
    return test_cases


def save_eval_set(eval_set_path: Path, test_cases: List[Dict[str, Any]]):
    """Save test cases to JSONL file."""
    # Create backup
    backup_path = eval_set_path.with_suffix(".jsonl.bak")
    if eval_set_path.exists():
        import shutil
        shutil.copy2(eval_set_path, backup_path)
        print(f"Created backup: {backup_path}")

    # Write updated test cases
    with open(eval_set_path, "w", encoding="utf-8") as f:
        for test_case in test_cases:
            f.write(json.dumps(test_case, ensure_ascii=False) + "\n")
    print(f"Saved {len(test_cases)} test cases to {eval_set_path}")


def call_api(api_url: str, question: str, vaults: List[str], folders: List[str], k: int = 20) -> Dict[str, Any]:
    """Call the ask API with debug mode enabled."""
    url = f"{api_url.rstrip('/')}/api/v1/ask?debug=true"
    payload = {
        "question": question,
        "k": k,
    }
    if vaults:
        payload["vaults"] = vaults
    if folders:
        payload["folders"] = folders

    try:
        response = requests.post(url, json=payload, timeout=60)
        response.raise_for_status()
        result = response.json()
        
        # Debug: Log if debug info is missing
        if "debug" not in result:
            print(f"Debug: API call successful but no 'debug' field in response")
            print(f"Debug: Response status: {response.status_code}")
            print(f"Debug: Response keys: {list(result.keys())}")
        
        return result
    except requests.exceptions.RequestException as e:
        print(f"Error calling API: {e}")
        print(f"Debug: URL was: {url}")
        print(f"Debug: Payload was: {json.dumps(payload, indent=2)}")
        if hasattr(e, "response") and e.response is not None:
            print(f"Debug: Response status: {e.response.status_code}")
            try:
                error_detail = e.response.json()
                print(f"Error details: {error_detail}")
            except Exception:
                print(f"Response text: {e.response.text}")
        raise


def normalize_heading_path(heading_path: str) -> str:
    """Normalize heading path for consistent matching."""
    # Strip extra spaces, normalize delimiter
    normalized = " > ".join(part.strip() for part in heading_path.split(">"))
    return normalized.strip()


def display_chunks(chunks: List[Dict[str, Any]], selected_indices: Set[int]):
    """Display retrieved chunks with selection status."""
    print("\n" + "=" * 80)
    print("RETRIEVED CHUNKS")
    print("=" * 80)
    for idx, chunk in enumerate(chunks):
        selected = "✓" if idx in selected_indices else " "
        print(f"\n[{selected}] {idx + 1}. Rank {chunk['rank']} | Score: {chunk['score_final']:.3f}")
        print(f"   File: {chunk['rel_path']}")
        print(f"   Heading: {chunk['heading_path']}")
        # Truncate text for display
        text = chunk.get("text", "")
        if len(text) > 200:
            text = text[:200] + "..."
        print(f"   Text: {text}")
        print("-" * 80)


def select_chunks_interactive(chunks: List[Dict[str, Any]]) -> Set[int]:
    """Interactive chunk selection."""
    selected_indices: Set[int] = set()

    while True:
        display_chunks(chunks, selected_indices)
        print("\nCommands:")
        print("  <number>     - Toggle selection of chunk (1-{})".format(len(chunks)))
        print("  all          - Select all chunks")
        print("  none         - Deselect all chunks")
        print("  done         - Finish selection")
        print("  quit         - Quit without saving")

        command = input("\n> ").strip().lower()

        if command == "done":
            break
        if command == "quit":
            return None  # Signal to quit
        if command == "all":
            selected_indices = set(range(len(chunks)))
            continue
        if command == "none":
            selected_indices = set()
            continue

        try:
            idx = int(command) - 1
            if 0 <= idx < len(chunks):
                if idx in selected_indices:
                    selected_indices.remove(idx)
                else:
                    selected_indices.add(idx)
            else:
                print(f"Invalid chunk number. Please enter 1-{len(chunks)}")
        except ValueError:
            print("Invalid command. Please enter a number, 'all', 'none', 'done', or 'quit'")

    return selected_indices


def extract_snippets(chunks: List[Dict[str, Any]], selected_indices: Set[int]) -> List[str]:
    """Extract optional snippets from selected chunks."""
    snippets = []
    for idx in selected_indices:
        text = chunks[idx].get("text", "")
        # Extract first sentence or first 100 chars as snippet
        if text:
            # Try to get first sentence
            sentences = text.split(". ")
            if sentences:
                snippet = sentences[0].strip()
                if len(snippet) > 100:
                    snippet = snippet[:100] + "..."
                if snippet:
                    snippets.append(snippet)
    return snippets


def create_gold_supports(chunks: List[Dict[str, Any]], selected_indices: Set[int]) -> List[Dict[str, Any]]:
    """Create gold_supports from selected chunks."""
    gold_supports = []

    # Group by (rel_path, heading_path) to avoid duplicates
    seen = set()
    for idx in sorted(selected_indices):
        chunk = chunks[idx]
        rel_path = chunk["rel_path"]
        heading_path = normalize_heading_path(chunk["heading_path"])
        key = (rel_path, heading_path)

        if key not in seen:
            seen.add(key)
            gold_support = {
                "rel_path": rel_path,
                "heading_path": heading_path,
            }
            # Extract snippet from this chunk
            text = chunk.get("text", "")
            if text:
                # Try to get first sentence or first 100 chars
                sentences = text.split(". ")
                if sentences:
                    snippet = sentences[0].strip()
                    if len(snippet) > 100:
                        snippet = snippet[:100] + "..."
                    if snippet:
                        gold_support["snippets"] = [snippet]

            gold_supports.append(gold_support)

    return gold_supports


def label_test_case(
    test_case: Dict[str, Any],
    api_url: str,
    skip_if_labeled: bool = False,
) -> Optional[Dict[str, Any]]:
    """Label a single test case."""
    test_id = test_case.get("id", "unknown")
    question = test_case.get("question", "")
    vaults = test_case.get("vaults", [])
    folders = test_case.get("folders", [])

    print("\n" + "=" * 80)
    print(f"TEST CASE: {test_id}")
    print("=" * 80)
    print(f"Question: {question}")
    print(f"Vaults: {vaults if vaults else 'all'}")
    print(f"Folders: {folders if folders else 'all'}")

    # Check if already labeled
    if skip_if_labeled and test_case.get("gold_supports"):
        print("\nAlready labeled. Current gold_supports:")
        gold_supports = test_case.get("gold_supports", [])
        if gold_supports:
            for idx, support in enumerate(gold_supports, 1):
                print(f"  {idx}. {support.get('rel_path', 'N/A')}")
                print(f"     Heading: {support.get('heading_path', 'N/A')}")
                if support.get("snippets"):
                    snippets = support["snippets"]
                    if isinstance(snippets, list):
                        print(f"     Snippets: {', '.join(snippets[:3])}")  # Show first 3 snippets
                    else:
                        print(f"     Snippets: {snippets}")
        else:
            print("  (empty)")
        print("\nSkipping...")
        response = input("Re-label? (y/n): ").strip().lower()
        if response != "y":
            return test_case

    # Call API
    print("\nCalling API...")
    try:
        api_response = call_api(api_url, question, vaults, folders, k=20)
    except Exception as e:
        print(f"Failed to call API: {e}")
        response = input("Continue anyway? (y/n): ").strip().lower()
        if response != "y":
            return None  # Signal to skip
        # Use empty chunks
        api_response = {"debug": {"retrieved_chunks": []}}

    # Debug: Print response structure for troubleshooting
    if "debug" not in api_response:
        print("\nDebug: API response keys:", list(api_response.keys()))
        if "error" in api_response:
            print(f"Debug: API returned error: {api_response.get('error')}")
        # Print a sample of the response (first 500 chars) for debugging
        import json
        response_str = json.dumps(api_response, indent=2)
        if len(response_str) > 500:
            print(f"Debug: Response preview:\n{response_str[:500]}...")
        else:
            print(f"Debug: Full response:\n{response_str}")

    # Extract chunks from debug response
    debug_info = api_response.get("debug")
    if not debug_info:
        print("Warning: No debug information in response. API may not support debug mode.")
        print("Make sure you're using the correct API URL and that debug mode is enabled.")
        chunks = []
    else:
        chunks = debug_info.get("retrieved_chunks", [])
        
        # Show folder selection info if available (helpful context even when no chunks)
        folder_selection = debug_info.get("folder_selection")
        if folder_selection:
            selected_folders = folder_selection.get("selected_folders", [])
            if selected_folders:
                print(f"\nFolder selection: {len(selected_folders)} folder(s) searched")
                for folder in selected_folders[:5]:  # Show first 5
                    print(f"  - {folder}")
                if len(selected_folders) > 5:
                    print(f"  ... and {len(selected_folders) - 5} more")

    if not chunks:
        print("\nNo chunks retrieved. This question may be unanswerable.")
        if debug_info and debug_info.get("folder_selection"):
            print("(Note: Folder selection was performed, but no chunks were found or could be fetched)")
        response = input("Mark as unanswerable? (y/n): ").strip().lower()
        if response == "y":
            test_case["answerable"] = False
            test_case["gold_supports"] = []
            return test_case
        else:
            # User wants to continue, maybe they'll add manual gold_supports
            test_case["gold_supports"] = []
            return test_case

    # Select chunks
    print(f"\nRetrieved {len(chunks)} chunks. Select which ones contain the answer:")
    selected_indices = select_chunks_interactive(chunks)

    if selected_indices is None:
        # User quit
        return None

    # Create gold_supports
    if selected_indices:
        gold_supports = create_gold_supports(chunks, selected_indices)
        test_case["gold_supports"] = gold_supports
        test_case["answerable"] = True
        print(f"\nCreated {len(gold_supports)} gold support(s)")
    else:
        # No chunks selected
        print("\nNo chunks selected.")
        response = input("Mark as unanswerable? (y/n): ").strip().lower()
        test_case["answerable"] = response == "y"
        test_case["gold_supports"] = []

    return test_case


def main():
    parser = argparse.ArgumentParser(
        description="Labeling workflow for marking gold_supports in eval_set.jsonl"
    )
    parser.add_argument(
        "--eval-set",
        type=Path,
        default=Path("eval/eval_set.jsonl"),
        help="Path to eval_set.jsonl file (default: eval/eval_set.jsonl)",
    )
    parser.add_argument(
        "--api-url",
        type=str,
        default="http://localhost:9000",
        help="Base URL of the API (default: http://localhost:9000)",
    )
    parser.add_argument(
        "--test-id",
        type=str,
        help="Label only a specific test case by ID",
    )
    parser.add_argument(
        "--skip-labeled",
        action="store_true",
        help="Skip test cases that already have gold_supports",
    )

    args = parser.parse_args()

    # Load test cases
    if not args.eval_set.exists():
        print(f"Error: Eval set file not found: {args.eval_set}")
        sys.exit(1)

    test_cases = load_eval_set(args.eval_set)
    print(f"Loaded {len(test_cases)} test cases from {args.eval_set}")

    # Filter to specific test ID if provided
    if args.test_id:
        test_cases = [tc for tc in test_cases if tc.get("id") == args.test_id]
        if not test_cases:
            print(f"Error: Test case with ID '{args.test_id}' not found")
            sys.exit(1)
        print(f"Filtered to test case: {args.test_id}")

    # Label test cases
    updated_test_cases = []
    skipped = False

    for i, test_case in enumerate(test_cases):
        if skipped:
            # User quit, save what we have so far
            updated_test_cases.append(test_case)
            continue

        updated = label_test_case(
            test_case,
            args.api_url,
            skip_if_labeled=args.skip_labeled,
        )

        if updated is None:
            # User quit
            skipped = True
            # Keep remaining test cases as-is
            updated_test_cases.extend(test_cases[i:])
            break

        updated_test_cases.append(updated)

    # Save updated test cases
    if updated_test_cases:
        save_eval_set(args.eval_set, updated_test_cases)
        print("\nLabeling complete!")
    else:
        print("\nNo changes to save.")


if __name__ == "__main__":
    main()

