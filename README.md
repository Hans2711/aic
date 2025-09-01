# aic

AI-assisted git commit message generator.

## Features

- Generates N (configurable) commit message suggestions from OpenAI Chat Completions
- Default model: `gpt-4o-mini` (override with `AIC_MODEL`)
- Default suggestions: 5 (override with `AIC_SUGGESTIONS` 1–15)
- Optional extra instruction via `-s "extra context"`
- Interactive selection with clear defaults & consistent symbols (✓ success, ✗ error, ➤ prompt, ℹ info)
- Non-interactive / CI mode (`AIC_NON_INTERACTIVE=1`) with optional auto commit (`AIC_AUTO_COMMIT=1`)
- Mock mode (`AIC_MOCK=1`) for offline/local testing (no API calls)
- `--no-color` flag (or `AIC_NO_COLOR=1`) to disable ANSI colors
- `--version` / `-v` flag to print version

## Install / Build

```bash
./scripts/build.sh
```

Binary output: `dist/aic`

Optionally place it in your PATH:

```bash
sudo mv dist/aic /usr/local/bin/
### Multi-platform Builds

The build script now produces platform-specific binaries:

```
dist/ubuntu/aic        # linux/amd64
dist/ubuntu-arm64/aic  # linux/arm64
dist/mac/aic           # macOS arm64 (Apple Silicon)
dist/mac-intel/aic     # macOS amd64 (Intel)
dist/checksums.txt     # SHA256 sums
```

Build all targets with version injection:

```bash
VERSION=0.1.1 ./scripts/build.sh
```

### Installation (Preferred Symlink)

Creates a versioned copy under `/opt/aic/<version>/<platform>/aic` and a symlink `/usr/local/bin/aic`.

```bash
./scripts/build.sh
sudo ./scripts/install.sh
aic --version
```

If you lack permissions, it installs to `~/.local/bin/aic` (ensure that is in your PATH).

### Checksums

After building, verify integrity:

```bash
./scripts/verify.sh
```

Or manually:

```bash
sha256sum dist/ubuntu/aic
grep ubuntu/aic dist/checksums.txt
```

### macOS Note

If macOS Gatekeeper blocks the binary, you may need:

```bash
xattr -d com.apple.quarantine /usr/local/bin/aic 2>/dev/null || true
```

```

## Usage

```bash
aic [-s "extra instruction"] [--version] [--no-color]
```

Minimum:

```bash
export OPENAI_API_KEY=sk-...  # required
aic
```

Provide extra guidance to the model:

```bash
aic -s "Focus on auth refactor and database migrations"
```

Disable color (scripted environments):

```bash
aic --no-color
# or
export AIC_NO_COLOR=1; aic
```

Show version:

```bash
aic --version
```

### Environment / Flags Matrix

| Name / Flag          | Purpose |
|----------------------|---------|
| `OPENAI_API_KEY`     | (required) API key |
| `AIC_MODEL`          | (optional) Model (default: gpt-4o-mini) |
| `AIC_SUGGESTIONS`    | (optional) Suggestions count 1–15 (default: 5) |
| `AIC_PROVIDER`       | (optional) Provider (default: openai) |
| `AIC_DEBUG`          | (optional) `1` for verbose raw response debug |
| `AIC_MOCK`           | (optional) `1` for mock suggestions (no API call) |
| `AIC_NON_INTERACTIVE`| (optional) `1` auto-select first suggestion & skip prompt |
| `AIC_AUTO_COMMIT`    | (optional) With NON_INTERACTIVE=1 also run `git commit` |
| `--no-color` / `AIC_NO_COLOR` | Disable ANSI colors |
| `--version` / `-v`   | Show version and exit |

### Example Help Snippet

```
Usage:
	aic [-s "extra instruction"] [--version] [--no-color]

Arguments & Environment:
	OPENAI_API_KEY        (required) OpenAI API key
	AIC_MODEL             (optional) Model [default: gpt-4o-mini]
	...
	--version / -v        Show version and exit
	--no-color            Disable colored output (alias: AIC_NO_COLOR=1)

Example:
	aic -s "Refactor auth logic"
```

## Testing Models

Use mock mode (fast, offline):

```bash
AIC_MOCK=1 ./scripts/test_models.sh
```

Real API (consumes tokens):

```bash
export OPENAI_API_KEY=sk-...
./scripts/test_models.sh
```

Customize models:

```bash
MODELS="gpt-4o-mini gpt-4o" ./scripts/test_models.sh
```

## Limitations / Notes

- Only OpenAI provider supported currently.
- Diff truncated at ~16k chars for safety.
- Suggestions limited to 15 for usability.
- Version is embedded at build (override with: `go build -ldflags "-X github.com/diesi/aic/internal/version.Version=0.1.1"`).

## Roadmap Ideas

- Support Anthropic / local providers via provider interface
- Optional streaming output
- Rich diff summarization step pre-prompt

