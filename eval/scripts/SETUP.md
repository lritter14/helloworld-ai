# Python Development Setup

This guide explains how to set up a Python virtual environment for the evaluation framework scripts.

## Quick Setup

```bash
# From project root
cd /Users/loganritter/projects/helloworld-ai

# Create virtual environment
python3 -m venv venv

# Activate virtual environment
source venv/bin/activate  # On macOS/Linux
# or
venv\Scripts\activate  # On Windows

# Install dependencies
pip install -r requirements.txt
```

## Step-by-Step Instructions

### 1. Create Virtual Environment

From the project root directory:

```bash
python3 -m venv venv
```

This creates a `venv/` directory containing an isolated Python environment.

### 2. Activate Virtual Environment

**On macOS/Linux:**
```bash
source venv/bin/activate
```

**On Windows:**
```bash
venv\Scripts\activate
```

When activated, your terminal prompt will show `(venv)` at the beginning.

### 3. Install Dependencies

```bash
pip install -r requirements.txt
```

This installs:
- `requests` - For making HTTP requests to the API
- `pytest` - For running tests

### 4. Verify Installation

```bash
# Check that requests is installed
python -c "import requests; print(requests.__version__)"

# Run tests
pytest eval/scripts/
```

### 5. Deactivate (when done)

```bash
deactivate
```

## Usage

Once the virtual environment is activated, you can run the evaluation scripts:

```bash
# Label test cases
python eval/scripts/label_eval.py --eval-set eval/eval_set.jsonl

# Run tests
pytest eval/scripts/
```

## Troubleshooting

**Issue: `python3` command not found**
- On macOS, ensure Xcode Command Line Tools are installed: `xcode-select --install`
- On Linux, install Python 3: `sudo apt-get install python3 python3-venv`

**Issue: `pip` command not found**
- Use `python3 -m pip` instead of `pip`
- Or install pip: `python3 -m ensurepip --upgrade`

**Issue: Permission errors**
- Make sure you're not using `sudo` with the virtual environment
- Virtual environments should be created and used without root privileges

## IDE Integration

### VS Code

1. Open the project in VS Code
2. Press `Cmd+Shift+P` (macOS) or `Ctrl+Shift+P` (Windows/Linux)
3. Type "Python: Select Interpreter"
4. Choose the interpreter from `./venv/bin/python` (or `.\venv\Scripts\python.exe` on Windows)

### PyCharm

1. Open the project in PyCharm
2. Go to Settings → Project → Python Interpreter
3. Click the gear icon → Add
4. Select "Existing environment"
5. Choose `venv/bin/python` (or `venv\Scripts\python.exe` on Windows)

