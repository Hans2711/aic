# aic

AI‑assisted git commit message generator with an iterative “combine” workflow and first‑class OpenAI + Claude support.

## Quick Start

```bash
# choose a provider (auto‑detected if set)
export OPENAI_API_KEY=sk-...   # or: export CLAUDE_API_KEY=sk-...

# generate commit messages for staged changes
aic

# optional: guide the model
aic -s "Focus on auth refactor and migrations"

# CI mode (no prompts) and auto‑commit
AIC_NON_INTERACTIVE=1 AIC_AUTO_COMMIT=1 aic
```

## Highlights

- Recursive combine: multi‑select suggestions, press Enter to synthesize better options, repeat to refine.
- Dual providers: `openai` or `claude` (auto‑detect; both set → OpenAI).
- Sensible defaults: OpenAI `gpt-4o-mini`, Claude `claude-3-sonnet-20240229` (override with `AIC_MODEL`).
- Friendly TUI: 1–9/0 to choose, arrows to navigate, Space to multi‑select.
- CI‑ready: non‑interactive mode and optional auto‑commit.
- Large diffs: structured summary plus clearly truncated raw diff with cutoff notes.
- Mock mode: `AIC_MOCK=1` for deterministic, offline suggestions.

<details>
<summary><strong>Install</strong></summary>

Build and install a symlinked binary:

```bash
bash scripts/build.sh
sudo bash scripts/install.sh   # /usr/local/bin/aic -> dist/<platform>/aic
aic --version
```

If `/usr/local/bin` is unavailable, the installer falls back to `~/.local/bin/aic`. Ensure it’s on `PATH`:

```bash
export PATH="$HOME/.local/bin:$PATH"; hash -r
```

Verify checksums and see produced artifacts:

```bash
./scripts/verify.sh
# outputs like:
# dist/ubuntu/aic, dist/ubuntu-arm64/aic, dist/mac/aic, dist/mac-intel/aic, dist/checksums.txt
```

macOS Gatekeeper (if needed):

```bash
xattr -d com.apple.quarantine /usr/local/bin/aic 2>/dev/null || true
```

</details>

<details>
<summary><strong>Usage</strong></summary>

```bash
aic [-s "extra instruction"] [--version] [--no-color]
```

Interactive controls:

- 1–9/0 choose, ↑/↓ navigate, Space multi‑select, Enter combine.

Disable ANSI colors:

```bash
aic --no-color
# or
export AIC_NO_COLOR=1; aic
```

</details>

<details>
<summary><strong>Configuration</strong></summary>

Providers and models:

- `AIC_PROVIDER`: `openai` | `claude` (auto‑detect from API keys; both set → OpenAI).
- `OPENAI_API_KEY` / `CLAUDE_API_KEY`: required for chosen provider.
- `AIC_MODEL`: override default model (OpenAI: `gpt-4o-mini`; Claude: `claude-3-sonnet-20240229`).

Generation & UX:

- `AIC_SUGGESTIONS`: number of suggestions (1–10, default 5; non-interactive default: 1).
- `AIC_NO_COLOR`: disable colors (same as `--no-color`).
- `-s "..."`: extra instruction appended to the prompt.

Run modes:

- `AIC_NON_INTERACTIVE=1`: pick first suggestion and print (CI).
- `AIC_AUTO_COMMIT=1`: with non‑interactive, also run `git commit -m ...`.
- `AIC_MOCK=1`: offline, deterministic suggestions (no API calls).

Debug:

- `AIC_DEBUG=1`: verbose error details.
- `AIC_DEBUG_SUMMARY=1`: debug output during large‑diff summarization.

Large diffs:

- For very large staged diffs, the tool generates a compact “Diff Summary” (using the provider’s default model) and appends a clearly truncated raw diff (~16k chars) with cutoff notes. If summarization fails, it falls back to simple truncation.

</details>

<details>
<summary><strong>Testing</strong></summary>

Mock (fast, offline):

```bash
AIC_MOCK=1 ./scripts/test_openai_models.sh
```

OpenAI API (consumes tokens):

```bash
export OPENAI_API_KEY=sk-...
./scripts/test_openai_models.sh
MODELS="gpt-4o-mini gpt-4o" ./scripts/test_openai_models.sh
```

Claude API:

```bash
export CLAUDE_API_KEY=sk-...
./scripts/test_claude_models.sh
```

Large diff summarization (~50KB synthetic diff):

```bash
bash scripts/test_large_diff.sh                 # mock
export OPENAI_API_KEY=sk-...; REAL=1 bash scripts/test_large_diff.sh
```

Enable extra debug during summary:

```bash
export AIC_DEBUG_SUMMARY=1
```

</details>
