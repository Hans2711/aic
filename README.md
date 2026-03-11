# aic

AI-assisted git CLI for commit messages and hosted code review requests. It summarizes your staged diff to propose commit subjects, and it can draft and open a GitHub pull request or GitLab merge request for the current branch.

## Status

The TypeScript CLI is functional for generating commit messages plus GitHub/GitLab review requests. `aic analyze` and `aic update` are scaffolding commands and not yet implemented.

## Quick start

1. Install dependencies (Bun ≥ 1.1 required):
   ```bash
   bun install
   ```
2. Export an API key. Providers auto-detect in the order OpenAI → Claude → Gemini (falls back to OpenAI if none are set):
   ```bash
   export OPENAI_API_KEY=sk-...
   # or
   export CLAUDE_API_KEY=sk-...
   # or
   export GEMINI_API_KEY=sk-...
   ```
3. Run the CLI against your staged changes:
   ```bash
   bun run src/cli.ts
   ```
   Use `AIC_NON_INTERACTIVE=1` to print the first suggestion (CI mode), or add `AIC_AUTO_COMMIT=1` to commit automatically.
4. To create a review request for the current branch, make sure the matching host CLI is installed and authenticated, then run:
   ```bash
   bun run src/cli.ts mr
   ```
   `aic` detects the repo host from the Git remote and uses `gh` for GitHub repos or `glab` for GitLab repos.

## What the CLI does

- Reads the staged diff (or the worktree diff when `AIC_DAEMON=1`) and filters noisy paths such as `dist/`, `node_modules/`, and other default prefixes (override with `AIC_IGNORE_PREFIXES`).
- Summarizes the diff, extracts file lists and highlights, and feeds that context to the selected model.
- Generates multiple suggestions (default 5; CI default 1) and supports a combine step with optional separate provider/model (`AIC_COMBINE_PROVIDER`, `AIC_COMBINE_MODEL`).
- Interactive mode shows staged files, lets you pick or combine suggestions, then offers to commit and push; non-interactive mode just prints (and optionally commits).
- Supports OpenAI, Claude, Gemini, or a custom OpenAI-compatible server; will pick a small or large model automatically based on diff size unless `AIC_MODEL` is set.
- `AIC_MOCK=1` returns deterministic offline suggestions for quick testing.
- `aic mr` detects the repo host and default branch, summarizes commits that are on the current branch but not on that default branch, generates a title and Markdown description, previews them, and creates the review request with `gh pr create` or `glab mr create`.

## Review requests

- `aic mr` creates a GitHub pull request or GitLab merge request for the current branch.
- It targets the remote default branch by default; use `aic mr --target-branch <branch>` to override it.
- `aic mr --draft` creates the review request as a draft.
- The generated description includes `Summary`, `Testing`, and `Commit Breakdown` sections, with one breakdown item per commit in the branch.
- `gh` must be available in `PATH` and authenticated for GitHub repos; `glab` must be available in `PATH` and authenticated for GitLab repos.

## Key environment variables

Core behavior:

- `AIC_MODEL`, `AIC_MODEL_SMALL`, `AIC_MODEL_LARGE`: override model choices (otherwise auto small/large per provider).
- `AIC_SUGGESTIONS`: number of initial suggestions (1–10; default 5, CI default 1).
- `AIC_NON_INTERACTIVE`: 1 to skip prompts and print the first suggestion.
- `AIC_AUTO_COMMIT`: with non-interactive mode, also run `git commit -m ...`.
- `AIC_NO_COLOR` / `--no-color`: disable colored output.
- `AIC_DAEMON` (or legacy `AIC_DEAMON`): use worktree diff and suppress staged-file banner.
- `AIC_IGNORE_PREFIXES`: comma-separated prefixes to drop from the diff (default includes `dist/`, `node_modules/`, `build/`, `out/`, `coverage/`, `target/`, `.next/`, `.turbo/`).
- `AIC_SUMMARIZE_CONCURRENCY`: number of concurrent API calls for diff chunk summarization (1-20; default: 10). Increase for faster summarization of large diffs, decrease if hitting API rate limits.
- `AIC_SUGGESTION_CONCURRENCY`: number of concurrent API calls for suggestion generation (1-20; default: 10). Controls how many suggestions are generated in parallel.

Provider selection:

- `AIC_PROVIDER`: force `openai` | `claude` | `gemini` | `custom` (auto-detect otherwise).
- `AIC_COMBINE_PROVIDER`, `AIC_COMBINE_MODEL`, `AIC_COMBINE_SUGGESTIONS`: overrides for the combine step.
- `OPENAI_API_KEY`, `CLAUDE_API_KEY`, `GEMINI_API_KEY`: API keys for the corresponding providers.
- Custom server: `CUSTOM_BASE_URL` (default `http://127.0.0.1:1234`), `CUSTOM_CHAT_COMPLETIONS_PATH` (default `/v1/chat/completions`), `CUSTOM_API_KEY` (optional).

## Mocking and debugging

- `AIC_MOCK=1 bun run src/cli.ts` produces deterministic suggestions without calling any API.
- `AIC_DEBUG=1` prints basic debug information with colored, categorized output and timestamps.
- `AIC_DEBUG=2` prints verbose debug information, including full content, system prompts, and detailed metrics.

The debug output features:

- Color-coded log levels (info, warnings, errors, success, metrics)
- Categorized logging (API, TOKEN, MODEL, GIT, WORKER, CONTENT, SYSTEM)
- Elapsed timestamps showing time since process start
- Beautiful formatting with icons and structured output

## Development

- The `dist/` folder contains the built CLI (`dist/aic`, `dist/cli.js`), but the recommended entrypoint during development is `bun run src/cli.ts`.
- A version banner is printed with `aic --version`.
- Future work: implement the `analyze` and `update` subcommands.
