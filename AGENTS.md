# AGENTS.md

This document helps AI assistants work effectively in this repository. It covers how to build, test (unit and integration), package, and the conventions to follow when making changes.

## TL;DR

- Build: `bash scripts/build.sh` → binaries in `dist/<platform>/aic`
- Unit tests: `go test ./...` (no network required)
- Integration tests (providers): run scripts in `scripts/` with the right API keys
- Mock mode: set `AIC_MOCK=1` to avoid external API calls
- Entry points: `cmd/aic/main.go`, version in `internal/version/version.go`
- Keep changes minimal and focused; do not touch unrelated files

## Repo Overview

- `cmd/aic/main.go`: CLI entrypoint.
- `internal/…`: packages for CLI, providers, config, commit generation, etc.
- `internal/version/version.go`: static version string used by the binary.
- `scripts/…`: build, package, hook installers, and integration test helpers.
- `dist/…`: build outputs per platform (created by scripts).
- `README.md`: user‑facing docs with usage, install, and testing notes.

## Build

- Requirement: Recent Go (see `go.mod` → `go 1.23.0`, toolchain `go1.24.6`).
- Primary build script: `scripts/build.sh`
  - Produces static binaries (CGO disabled) for Linux, macOS (arm64/amd64), and Windows.
  - Outputs to:
    - `dist/ubuntu/aic`
    - `dist/ubuntu-arm64/aic`
    - `dist/mac/aic`
    - `dist/mac-intel/aic`
    - `dist/windows/aic.exe`
    - `dist/windows-arm64/aic.exe`
  - Writes `dist/checksums.txt`; verify with `bash scripts/verify.sh`.

## Packaging

- AppImage (Linux): `bash scripts/package_appimage.sh`
  - Outputs: `dist/aic_linux_amd64.AppImage`, `dist/aic_linux_arm64.AppImage`.
- Zip/Tar/DEB helpers:
  - `scripts/package_zip.sh`
  - `scripts/package_deb.sh`

## Testing

- Unit tests (fast, offline):
  - Run all: `go test ./...`
  - Tests in `internal/**` and `cmd/aic` avoid real network calls.

- Integration scripts (exercise real providers; require API keys):
  - OpenAI: `scripts/test_openai_models.sh`
    - Usage: `export OPENAI_API_KEY=sk-...; ./scripts/test_openai_models.sh`
    - Restrict models: `MODELS="gpt-4o-mini gpt-4o" ./scripts/test_openai_models.sh`
  - Claude: `scripts/test_claude_models.sh`
    - Usage: `export CLAUDE_API_KEY=sk-...; ./scripts/test_claude_models.sh`
  - Gemini: `scripts/test_gemini_models.sh`
    - Usage: `export GEMINI_API_KEY=sk-...; ./scripts/test_gemini_models.sh`
  - Custom (OpenAI‑compatible): `scripts/test_custom_local.sh`
    - Example: `AIC_PROVIDER=custom CUSTOM_BASE_URL=http://127.0.0.1:1234 ./scripts/test_custom_local.sh`

- Large diff summarization test:
  - Mock (offline): `bash scripts/test_large_diff.sh`
  - Real (OpenAI): `export OPENAI_API_KEY=sk-...; REAL=1 bash scripts/test_large_diff.sh`

- Mock mode for quick sanity checks (no network):
  - `AIC_MOCK=1 ./scripts/test_openai_models.sh`
  - Or run the binary with `AIC_MOCK=1`.

## Running the CLI

- Basic: `aic` (generate commit messages for staged changes).
- Guide output: `aic -s "Focus on auth refactor"`.
- Non‑interactive/CI: `AIC_NON_INTERACTIVE=1 aic`
- Auto‑commit in CI: `AIC_NON_INTERACTIVE=1 AIC_AUTO_COMMIT=1 aic`
- Disable color: `aic --no-color` or `AIC_NO_COLOR=1 aic`
- Git hook install: `bash scripts/install_git_hook.sh`
  - Then `git commit -m aic` to trigger generation.

## Providers and Env Vars

- Provider selection (auto‑detect by keys; priority OpenAI > Claude > Gemini):
  - `AIC_PROVIDER`: `openai` | `claude` | `gemini` | `custom`
  - `OPENAI_API_KEY`, `CLAUDE_API_KEY`, `GEMINI_API_KEY`
  - Custom server: `AIC_PROVIDER=custom` and `CUSTOM_BASE_URL`, plus optional `CUSTOM_API_KEY`
  - Choose models: `AIC_MODEL`, combine‑step override: `AIC_COMBINE_PROVIDER`, `AIC_COMBINE_MODEL`

## Code Conventions for AI Changes

- Keep edits surgical and focused on the asked task.
- Do not reformat or rename unrelated files.
- Prefer smallest viable change; avoid introducing new deps unless required.
- If touching behavior:
  - Add/adjust unit tests near the changed code.
  - Run `go test ./...` to validate locally.
- When reading/searching:
  - Use fast grep: `rg` (ripgrep) and read files in chunks (≤250 lines per read).
- Secrets & network:
  - Never print or commit API keys.
  - Prefer mock/in‑repo tests; use integration scripts only when necessary.

## Common File Paths

- CLI entry: `cmd/aic/main.go`
- Version: `internal/version/version.go`
- Commit logic: `internal/commit/…`
- Providers: `internal/provider/…`, OpenAI client: `internal/openai/…`
- Config/env: `internal/config/…`
- Git hook (template): `scripts/git-hooks/prepare-commit-msg`

## Release Bump (manual)

- Update version string in `internal/version/version.go` and rebuild.

## When in Doubt

- Skim `README.md` and `scripts/` for examples before changing code.
- Ask for confirmation if a change might impact packaging or release outputs.

