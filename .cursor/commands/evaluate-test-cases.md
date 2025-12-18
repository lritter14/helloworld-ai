# Evaluation Set Validation and Correction Prompt

You are tasked with validating and correcting the `eval_set.jsonl` file, which contains test cases for evaluating a RAG (Retrieval-Augmented Generation) system. This file contains one JSON object per line, where each object represents a single test case.

**Your task**: Validate all test cases and **update any invalid test cases** to fix all validation issues. If all test cases are valid, no action is required. Leave valid test cases unchanged.

## File Structure

The file is in JSONL format (JSON Lines), where each line is a complete JSON object. Each test case has the following structure:

```json
{
  "id": "test_001",
  "question": "What are the key tips for LeetCode interviews in Golang?",
  "answerable": true,
  "expected_key_facts": ["no built in string sort", "single element in string is a byte", "custom sorting"],
  "gold_supports": [
    {
      "rel_path": "Software/LeetCode Tips.md",
      "heading_path": "# Golang Tips & Oddities",
      "snippets": ["no built in string sort", "single element in a string is a byte"]
    }
  ],
  "required_support_groups": null,
  "recency_conflict_rule": null,
  "tags": ["personal", "code"],
  "vaults": ["personal"],
  "folders": ["Software"],
  "category": "factual",
  "difficulty": "easy"
}
```

## Field Definitions and Requirements

### Required Fields

1. **`id`** (string, required)
   - Unique identifier for the test case
   - Format: `test_XXX` where XXX is a zero-padded number
   - Must be unique across all test cases
   - Validation: Check for duplicates

2. **`question`** (string, required)
   - The question to be evaluated
   - Must be a non-empty string
   - Should be clear and unambiguous
   - Validation: Check that question is meaningful and answerable given the context

3. **`answerable`** (boolean, required)
   - Whether the question can be answered from the knowledge base
   - `true`: The answer exists in the notes
   - `false`: The answer does not exist (system should abstain)
   - Validation: Must match the actual answerability based on gold_supports

4. **`expected_key_facts`** (array of strings, required)
   - List of key facts that should appear in the answer
   - Can be empty array `[]` if not applicable
   - Used for reference during evaluation, not for automated scoring
   - Validation: Check that facts are relevant to the question

5. **`gold_supports`** (array of objects, required)
   - Ground truth supporting documents that contain the answer
   - Format: Array of objects with `rel_path`, `heading_path`, and optional `snippets`
   - Must be empty array `[]` if `answerable: false`
   - Must be non-empty if `answerable: true`
   - Validation: **CRITICAL** - Verify each gold_support actually exists and contains relevant content

6. **`tags`** (array of strings, required)
   - Flexible tags for filtering and categorization
   - Common tags: `["personal"]`, `["work"]`, `["code"]`, `["interview"]`, `["recipes"]`
   - Validation: Check that tags are consistent and meaningful

7. **`vaults`** (array of strings, required)
   - Which vault(s) to search for this question
   - Valid values: `["personal"]`, `["work"]`, or both
   - Can be empty array `[]` to search all vaults
   - Validation: Check that vault names match actual vault structure

8. **`folders`** (array of strings, required)
   - Which folder(s) within the vault to search
   - Can be empty array `[]` to search all folders
   - Folder paths are relative to vault root
   - Validation: Check that folders exist in the specified vault(s)

9. **`category`** (string, required)
   - Type of question being tested
   - Valid values: `"factual"`, `"multi_hop"`, `"recency/conflict"`, `"general"`, `"adversarial"`
   - Validation: Check that category matches question type and other fields

10. **`difficulty`** (string, required)
    - Difficulty level of the question
    - Valid values: `"easy"`, `"medium"`, `"hard"`
    - Validation: Check that difficulty is appropriate for the question complexity

### Optional Fields

1. **`required_support_groups`** (array of arrays of integers, optional)
   - For multi-hop questions, specifies which gold_supports must be retrieved together
   - Format: `[[0, 1], [2]]` means: (support 0 AND support 1) OR (support 2)
   - Indices refer to positions in the `gold_supports` array
   - Must be `null` for non-multi-hop questions
   - Required for `category: "multi_hop"`
   - Validation: Check that indices are valid and groups make sense

2. **`recency_conflict_rule`** (string, optional)
    - For recency/conflict questions, specifies expected behavior
    - Valid values: `"cite_newer"`, `"acknowledge_both"`, `"cite_both"`
    - Must be `null` for non-recen cy/conflict questions
    - Required for `category: "recency/conflict"`
    - Validation: Check that rule matches the question type

## Gold Support Structure

Each object in `gold_supports` must have:

- **`rel_path`** (string, required): Relative path to the note file from vault root
  - Example: `"Software/LeetCode Tips.md"`
  - Validation: **VERIFY FILE EXISTS** - Check that this file actually exists in the specified vault(s)

- **`heading_path`** (string, required): Heading hierarchy path within the document
  - Format: `"# Heading"` or `"# Heading > ## Subheading"`
  - Uses ` > ` as delimiter between heading levels
  - Validation: **VERIFY HEADING EXISTS** - Check that this heading path exists in the file

- **`snippets`** (array of strings, optional): Exact phrases or quotes that should appear
  - Used for validation and reference
  - Can be empty array or omitted
  - Validation: **VERIFY SNIPPETS EXIST** - Check that snippets appear in the specified heading section

## Validation Checklist

For each test case, perform the following validations:

### 1. Basic Structure Validation

- [ ] All required fields are present
- [ ] Field types are correct (strings, booleans, arrays as expected)
- [ ] No extra unexpected fields
- [ ] `id` is unique across all test cases
- [ ] `id` follows the format `test_XXX`

### 2. Answerability Validation

- [ ] If `answerable: true`, then `gold_supports` must be non-empty
- [ ] If `answerable: false`, then `gold_supports` must be empty array `[]`
- [ ] If `answerable: false`, then `expected_key_facts` should be empty (or very minimal)
- [ ] The question actually matches the answerability flag (verify by checking if answer exists in notes)

### 3. Gold Supports Validation (CRITICAL)

For each `gold_support` in the array:

- [ ] **File Existence**: The `rel_path` file exists in at least one of the specified vaults
- [ ] **Heading Existence**: The `heading_path` exists in the file (check heading hierarchy)
- [ ] **Snippet Verification**: If `snippets` are provided, verify they appear in the specified heading section
- [ ] **Content Relevance**: The heading section actually contains information relevant to answering the question
- [ ] **Path Format**: `rel_path` uses forward slashes and matches actual file structure
- [ ] **Heading Format**: `heading_path` uses ` > ` as delimiter and matches actual heading structure

### 4. Expected Key Facts Validation

- [ ] Facts are relevant to the question
- [ ] Facts can reasonably be extracted from the gold_supports
- [ ] Facts are specific enough to be useful (not too vague)
- [ ] For unanswerable questions, `expected_key_facts` should be empty

### 5. Category-Specific Validation

#### For `category: "factual"`

- [ ] Question asks for a straightforward fact
- [ ] `required_support_groups` is `null`
- [ ] `recency_conflict_rule` is `null`
- [ ] Answer should be directly extractable from gold_supports

#### For `category: "multi_hop"`

- [ ] Question requires information from multiple sources or reasoning steps
- [ ] `required_support_groups` is NOT `null` and is a valid array
- [ ] `recency_conflict_rule` is `null`
- [ ] All indices in `required_support_groups` are valid (within bounds of `gold_supports` array)
- [ ] Multiple gold_supports are provided (at least 2)
- [ ] The question actually requires multiple pieces of information

#### For `category: "recency/conflict"`

- [ ] Question involves conflicting information or recency considerations
- [ ] `recency_conflict_rule` is NOT `null` and is a valid value
- [ ] `required_support_groups` is typically `null` (unless also multi-hop)
- [ ] Multiple gold_supports are provided (at least 2)
- [ ] The rule matches the question type (e.g., "cite_newer" for questions about recent information)

#### For `category: "general"`

- [ ] Question is unanswerable from the knowledge base
- [ ] `answerable: false`
- [ ] `gold_supports` is empty array `[]`
- [ ] `expected_key_facts` is empty array `[]`

### 6. Vault and Folder Validation

- [ ] `vaults` array contains valid vault names (typically `"personal"` or `"work"`)
- [ ] If `folders` is specified, verify folders exist in the specified vault(s)
- [ ] If `folders` is empty, question should be answerable from any folder
- [ ] Gold support files are actually in the specified vault(s) and folder(s)

### 7. Tags Validation

- [ ] Tags are consistent (e.g., work-related questions have `"work"` tag)
- [ ] Tags match the vault (personal vault → `"personal"` tag, work vault → `"work"` tag)
- [ ] Tags are meaningful and useful for filtering

### 8. Difficulty Validation

- [ ] `difficulty` matches question complexity:
  - `"easy"`: Simple factual lookup, single source
  - `"medium"`: Requires understanding context, may need multiple facts
  - `"hard"`: Complex reasoning, multi-hop, or requires synthesis

### 9. Consistency Checks

- [ ] Question, expected_key_facts, and gold_supports are all aligned
- [ ] The question can actually be answered using the provided gold_supports
- [ ] Tags, vaults, and folders are consistent with each other
- [ ] Category matches the question type and field configuration

## Validation and Correction Process

1. **Load the JSONL file**: Parse each line as a JSON object
2. **For each test case**:
   - Perform basic structure validation
   - Load the actual note files referenced in `gold_supports`
   - Verify file existence and heading paths
   - Check snippet presence in the specified sections
   - Validate answerability matches reality
   - Check category-specific requirements
   - Verify consistency across all fields
   - **If invalid**: Fix all issues by updating the test case fields appropriately
   - **If valid**: Leave the test case unchanged
3. **Update the file**: Write the corrected test cases back to `eval_set.jsonl`, preserving the JSONL format (one JSON object per line)
4. **Report summary**: Provide a summary of:
   - Total test cases processed
   - Number of valid test cases (unchanged)
   - Number of invalid test cases (fixed)
   - List of fixes applied for each corrected test case

## Common Issues to Watch For

1. **File Path Mismatches**: `rel_path` doesn't match actual file location
2. **Heading Path Errors**: `heading_path` doesn't exist or is incorrectly formatted
3. **Snippet Not Found**: Snippets don't appear in the specified heading section
4. **Answerability Mismatch**: `answerable: true` but no valid gold_supports, or vice versa
5. **Multi-hop Configuration**: `category: "multi_hop"` but `required_support_groups` is null
6. **Invalid Indices**: `required_support_groups` contains indices out of bounds
7. **Category Mismatch**: Category doesn't match question type or field configuration
8. **Vault/Folder Mismatch**: Gold support files aren't in the specified vaults/folders
9. **Inconsistent Tags**: Tags don't match vault or question type
10. **Difficulty Mismatch**: Difficulty level doesn't match question complexity

## Correction Guidelines

When fixing invalid test cases, follow these guidelines:

1. **File Path Corrections**: If `rel_path` is incorrect, find the correct path in the vault and update it
2. **Heading Path Corrections**: If `heading_path` doesn't exist, find the correct heading path in the file and update it
3. **Snippet Corrections**: If snippets don't exist, either remove them or find correct snippets from the heading section
4. **Answerability Corrections**:
   - If `answerable: true` but no valid gold_supports exist, set `answerable: false` and clear `gold_supports` and `expected_key_facts`
   - If `answerable: false` but valid gold_supports exist, set `answerable: true` and populate `gold_supports`
5. **Category Corrections**: Update category to match the question type and field configuration
6. **Multi-hop Corrections**: If `category: "multi_hop"` but `required_support_groups` is null, add appropriate `required_support_groups` based on which supports must be retrieved together
7. **Field Consistency**: Ensure all fields are consistent with each other (tags match vaults, folders exist, etc.)

## Output Format

After processing all test cases, provide a summary report:

```text
Validation Summary:
- Total test cases: 50
- Valid test cases: 45 (unchanged)
- Invalid test cases: 5 (fixed)

Fixes Applied:

Test test_002:
  - Fixed: Updated rel_path from "Recipes/Maple Pork.md" to "Recipes/Maple Pork.md" (verified file exists)
  - Fixed: Updated heading_path from "# Ingredients" to "# Ingredients" (verified heading exists)

Test test_019:
  - Fixed: Added required_support_groups: [[0, 1]] (category is "multi_hop")

Test test_025:
  - Fixed: Updated category from "factual" to "multi_hop" (question requires multiple sources)
  - Fixed: Added required_support_groups: [[0, 1]]

All test cases are now valid.
```

**Important**:

- Only update test cases that have validation issues
- Leave valid test cases completely unchanged
- If all test cases are valid, simply report: "All test cases are valid. No changes required."

## Notes

- Be thorough: Check every field and every gold_support
- Be precise: Verify file paths, headings, and snippets exactly
- Be contextual: Consider the question when validating expected_key_facts and gold_supports
- Fix issues directly: Don't just report problems—update the test cases to fix them
- Preserve valid data: Only change fields that need correction; leave correct data unchanged
- Maintain format: Ensure the output remains valid JSONL format (one JSON object per line)
- Be conservative: When in doubt about a fix, prefer minimal changes that address the specific validation issue
