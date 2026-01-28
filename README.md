# aic

AI-assisted git commit message generator (TypeScript, Bun) that summarizes your diff, proposes multiple subjects, and lets you combine them into a stronger message.

## Status

The TypeScript CLI is functional for generating commit messages. `aic analyze` and `aic update` are scaffolding commands and not yet implemented.

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

## What the CLI does

- Reads the staged diff (or the worktree diff when `AIC_DAEMON=1`) and filters noisy paths such as `dist/`, `node_modules/`, and other default prefixes (override with `AIC_IGNORE_PREFIXES`).
- Summarizes the diff, extracts file lists and highlights, and feeds that context to the selected model.
- Generates multiple suggestions (default 5; CI default 1) and supports a combine step with optional separate provider/model (`AIC_COMBINE_PROVIDER`, `AIC_COMBINE_MODEL`).
- Interactive mode shows staged files, lets you pick or combine suggestions, then offers to commit and push; non-interactive mode just prints (and optionally commits).
- Supports OpenAI, Claude, Gemini, or a custom OpenAI-compatible server; will pick a small or large model automatically based on diff size unless `AIC_MODEL` is set.
- `AIC_MOCK=1` returns deterministic offline suggestions for quick testing.

## Key environment variables

Core behavior:

- `AIC_MODEL`, `AIC_MODEL_SMALL`, `AIC_MODEL_LARGE`: override model choices (otherwise auto small/large per provider).
- `AIC_SUGGESTIONS`: number of initial suggestions (1–10; default 5, CI default 1).
- `AIC_NON_INTERACTIVE`: 1 to skip prompts and print the first suggestion.
- `AIC_AUTO_COMMIT`: with non-interactive mode, also run `git commit -m ...`.
- `AIC_NO_COLOR` / `--no-color`: disable colored output.
- `AIC_DAEMON` (or legacy `AIC_DEAMON`): use worktree diff and suppress staged-file banner.
- `AIC_IGNORE_PREFIXES`: comma-separated prefixes to drop from the diff (default includes `dist/`, `node_modules/`, `build/`, `out/`, `coverage/`, `target/`, `.next/`, `.turbo/`).

Provider selection:

- `AIC_PROVIDER`: force `openai` | `claude` | `gemini` | `custom` (auto-detect otherwise).
- `AIC_COMBINE_PROVIDER`, `AIC_COMBINE_MODEL`, `AIC_COMBINE_SUGGESTIONS`: overrides for the combine step.
- `OPENAI_API_KEY`, `CLAUDE_API_KEY`, `GEMINI_API_KEY`: API keys for the corresponding providers.
- Custom server: `CUSTOM_BASE_URL` (default `http://127.0.0.1:1234`), `CUSTOM_CHAT_COMPLETIONS_PATH` (default `/v1/chat/completions`), `CUSTOM_API_KEY` (optional).

## Mocking and debugging

- `AIC_MOCK=1 bun run src/cli.ts` produces deterministic suggestions without calling any API.
- `AIC_DEBUG=1` prints extra detail, including summary content and token estimates.

## Development

- The `dist/` folder contains the built CLI (`dist/aic`, `dist/cli.js`), but the recommended entrypoint during development is `bun run src/cli.ts`.
- A version banner is printed with `aic --version`.
- Future work: implement the `analyze` and `update` subcommands.
