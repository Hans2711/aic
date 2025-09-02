# aic

AI-assisted git commit message generator with recursive suggestion combining and first-class OpenAI + Claude integration.

## Key Features

- Recursive combine loop: select multiple drafts, press Enter to have the AI synthesize improved alternatives, then iterate until you’re happy. This can be repeated recursively to converge on a great message.
- OpenAI and Claude: plug in either provider by setting `AIC_PROVIDER=openai|claude` with the matching API key. If `AIC_PROVIDER` is unset, the provider is auto-detected from your API keys (both set -> uses OpenAI).
- Smart defaults: OpenAI model `gpt-4o-mini`; Claude model `claude-3-sonnet-20240229`. Override with `AIC_MODEL`.
- Interactive UX: single-key choice (1–9,0), arrow navigation, Space to multi-select, and consistent symbols (✓ success, ✗ error, ➤ prompt, ℹ info).
- CI-friendly: non-interactive mode (`AIC_NON_INTERACTIVE=1`) with optional auto-commit (`AIC_AUTO_COMMIT=1`).
- Large diff support: when the diff is huge, the tool generates a compact, structured summary and includes a clearly marked, truncated raw diff with cutoff notes. If summarization fails, it gracefully falls back to simple truncation.
- Local/mock mode: `AIC_MOCK=1` returns deterministic mock suggestions with no API calls.
- Quality guardrails: requests the model to output plain, one-line conventional commit messages without numbering.

## Install / Build

Quick build:

```bash
bash scripts/build.sh
```

Binary output:

```
dist/aic
```

### Multi-platform builds

The build script produces platform-specific binaries and checksums:

```
dist/ubuntu/aic        # linux/amd64
dist/ubuntu-arm64/aic  # linux/arm64
dist/mac/aic           # macOS arm64 (Apple Silicon)
dist/mac-intel/aic     # macOS amd64 (Intel)
dist/checksums.txt     # SHA256 sums
```

Build everything:

```bash
./scripts/build.sh
```

### Installation (symlink preferred)

Create a stable symlink so rebuilds update in place:

```bash
bash scripts/build.sh
sudo bash scripts/install.sh   # creates /usr/local/bin/aic -> dist/<platform>/aic
aic --version                  # prints: aic 1.0.0
```

If `/usr/local/bin` is unavailable, it falls back to `~/.local/bin/aic`. Ensure it’s on `PATH`:

```bash
export PATH="$HOME/.local/bin:$PATH"
hash -r  # clear shell command cache if needed
```

### Verify checksums

```bash
./scripts/verify.sh           # validates dist/checksums.txt
```

Manual spot-check:

```bash
sha256sum dist/ubuntu/aic | grep $(cut -d' ' -f1 dist/checksums.txt)
```

### macOS note

Gatekeeper may require unquarantining the binary once:

```bash
xattr -d com.apple.quarantine /usr/local/bin/aic 2>/dev/null || true
```

### Homebrew (Tap)

You can install via a personal Homebrew tap. A ready-to-use formula is included at `Formula/aic.rb`.

Publish your tap (one-time):

```bash
# 1) Create a new repo on GitHub named: homebrew-tap
#    e.g. https://github.com/Hans2711/homebrew-tap

# 2) In that repo, add this file path and copy the formula from this project:
#    Formula/aic.rb

# 3) Commit and push the formula to your tap repo
```

Tag a release and point the formula to it (recommended):

```bash
# Tag your current commit and push the tag (example: v1.0.0)
git tag v1.0.0
git push origin v1.0.0

# Compute the SHA256 of the release tarball and update Formula/aic.rb
curl -L https://github.com/Hans2711/aic/archive/refs/tags/v1.0.0.tar.gz \
  | shasum -a 256
# Replace the url/sha256 in Formula/aic.rb with the tag URL and printed checksum
```

Install from your tap:

```bash
brew tap Hans2711/tap
brew install aic
aic --version
```

Notes:

- The included formula currently points at a specific commit and sets `version "1.0.0"`.
- For Homebrew/homebrew-core submission, prefer a tagged release URL and run: `brew audit --new --strict Formula/aic.rb`.
- The formula builds from source via Go Modules; Homebrew will fetch dependencies automatically.

## Usage

```bash
aic [-s "extra instruction"] [--version] [--no-color]
```

Minimum setup (choose a provider):

```bash
export OPENAI_API_KEY=sk-...          # for OpenAI (auto-detected if set)
# or
export CLAUDE_API_KEY=sk-...          # for Claude
aic
```

Add extra guidance to the model:

```bash
aic -s "Focus on auth refactor and database migrations"
```

Disable ANSI colors:

```bash
aic --no-color
# or
export AIC_NO_COLOR=1; aic
```

Show version label:

```bash
aic --version  # prints: aic 1.0.0
```

### Interactive selection and recursive combine

- Use 1–9 (and 0 for the 10th) to pick a suggestion.
- Use ↑/↓ to navigate; Space toggles selection for multi-select.
- With 2+ selections, press Enter to “Combine” and get a fresh set of refined suggestions. Repeat as needed to iterate toward the best message.

## Configuration (Environment Options)

Provider and model:

- `AIC_PROVIDER`: choose `openai` or `claude`. If unset, the tool auto-detects the provider from available API keys (`OPENAI_API_KEY`/`CLAUDE_API_KEY`). If both are set, OpenAI is used.
- `OPENAI_API_KEY`: required when `AIC_PROVIDER=openai`.
- `CLAUDE_API_KEY`: required when `AIC_PROVIDER=claude`.
- `AIC_MODEL`: override model; defaults depend on provider:
  - OpenAI default: `gpt-4o-mini`
  - Claude default: `claude-3-sonnet-20240229`

Generation behavior:

- `AIC_SUGGESTIONS`: number of suggestions (1–10, default: 5).
- `AIC_MOCK`: set to `1` for local mock suggestions (no API calls).

Run modes and output:

- `AIC_NON_INTERACTIVE`: set to `1` to print and select the first suggestion without prompts (CI mode).
- `AIC_AUTO_COMMIT`: with non-interactive mode, also run `git commit -m ...`.
- `AIC_NO_COLOR` or `--no-color`: disable ANSI colors.
- `-s "..."`: provide extra instruction appended to the system prompt.
- `--version` / `-v`: print version label and exit.

Debugging:

- `AIC_DEBUG`: set to `1` to print verbose raw response details on errors.
- `AIC_DEBUG_SUMMARY`: set to `1` to print diff summary debug details when large diff summarization triggers.

Notes on large diffs:

- When the staged diff is very large, the tool calls the provider’s default model (ignores `AIC_MODEL`) to generate a compact “Diff Summary” and then appends a clearly marked truncated raw diff (~16k chars) with cutoff notes. If summarization fails, it falls back to simple truncation.

## Testing Models

Mock mode (fast, offline):

```bash
AIC_MOCK=1 ./scripts/test_openai_models.sh
```

Real OpenAI API (consumes tokens):

```bash
export OPENAI_API_KEY=sk-...
./scripts/test_openai_models.sh
```

Customize OpenAI models:

```bash
MODELS="gpt-4o-mini gpt-4o" ./scripts/test_openai_models.sh
```

Test Claude models:

```bash
export CLAUDE_API_KEY=sk-...
./scripts/test_claude_models.sh
```

## Large Diff Summarization Test

Validate summarization behavior with a synthetic large diff (~50KB):

Mock (no API calls):

```bash
bash scripts/test_large_diff.sh
```

Real API (consumes tokens):

```bash
export OPENAI_API_KEY=sk-...
REAL=1 bash scripts/test_large_diff.sh
```

Enable debug output for the summary step:

```bash
export AIC_DEBUG_SUMMARY=1
```
