# aic

AI‑assisted git commit message generator with an iterative “combine” workflow and first‑class OpenAI, Claude, and Gemini support.

## Quick Start

```bash
# choose a provider (auto‑detected if set)
export OPENAI_API_KEY=sk-...   # or: export CLAUDE_API_KEY=sk-... or: export GEMINI_API_KEY=sk-...

# generate commit messages for staged changes
aic

# optional: guide the model
aic -s "Focus on auth refactor and migrations"

# CI mode (no prompts) and auto‑commit
AIC_NON_INTERACTIVE=1 AIC_AUTO_COMMIT=1 aic
```

## Highlights

- Recursive combine: multi‑select suggestions, press Enter to synthesize better options, repeat to refine.
- Multiple providers: `openai`, `claude`, or `gemini` (auto‑detect; priority openai > claude > gemini).
- Sensible defaults: OpenAI `gpt-4o-mini`, Claude `claude-3-sonnet-20240229`, Gemini `gemini-1.5-flash` (override with `AIC_MODEL`).
- Friendly TUI: 1–9/0 to choose, arrows to navigate, Space to multi‑select.
- CI‑ready: non‑interactive mode and optional auto‑commit.
- Large diffs: structured summary plus clearly truncated raw diff with cutoff notes.
- Mock mode: `AIC_MOCK=1` for deterministic, offline suggestions.

<details>
<summary><strong>Install</strong></summary>

Clone, verify, and install (no build required):

```bash
git clone https://github.com/Hans2711/aic.git
cd aic
./scripts/verify.sh
sudo bash scripts/install.sh   # /usr/local/bin/aic -> dist/<platform>/aic
aic --version
```

If `/usr/local/bin` is unavailable, the installer falls back to `~/.local/bin/aic`. Ensure it’s on `PATH`:

```bash
export PATH="$HOME/.local/bin:$PATH"; hash -r
```

macOS Gatekeeper (if needed):

```bash
xattr -d com.apple.quarantine /usr/local/bin/aic 2>/dev/null || true
```

</details>

<details>
<summary><strong>Windows</strong></summary>

Option 1: Download a prebuilt binary

- Grab `aic_windows_amd64.zip` (or `aic_windows_arm64.zip`) from the latest GitHub Release.
- Unzip and place `aic.exe` somewhere on your `PATH` (e.g., `C:\Users\<you>\bin`).
- Ensure Git for Windows is installed and available in `PATH`.

Option 2: Build from source with Go

```powershell
git clone https://github.com/Hans2711/aic.git
cd aic
go build -o aic.exe ./cmd/aic
```

Verify:

```powershell
PS> .\aic.exe --version
```

Notes:

- The interactive UI falls back gracefully on Windows if advanced TTY features aren’t available.
- Clipboard copy uses `clip` when present (bundled with modern Windows).

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

- `AIC_PROVIDER`: `openai` | `claude` | `gemini` (auto‑detect from API keys; priority openai > claude > gemini).
- `OPENAI_API_KEY` / `CLAUDE_API_KEY` / `GEMINI_API_KEY`: required for chosen provider.
- `AIC_MODEL`: override default model (OpenAI: `gpt-4o-mini`; Claude: `claude-3-sonnet-20240229`; Gemini: `gemini-1.5-flash`).

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
<summary><strong>Gemini Notes</strong></summary>

- Models: commonly available options include `gemini-1.5-flash` (default) and `gemini-1.5-pro`. Newer `gemini-2.5-*` models may require allowlisted access.
- Auto‑detect: setting `GEMINI_API_KEY` is enough; or force with `AIC_PROVIDER=gemini`.
- Output budget: if Gemini returns empty content with `finishReason=MAX_TOKENS`, `aic` automatically retries with a larger output token budget. You can also set `AIC_SUGGESTIONS=1` or choose a smaller model (e.g., `gemini-1.5-flash`).

Example:

```bash
export GEMINI_API_KEY=sk-...
AIC_PROVIDER=gemini AIC_MODEL=gemini-1.5-flash aic
```

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

Gemini API:

```bash
export GEMINI_API_KEY=sk-...
./scripts/test_gemini_models.sh
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
