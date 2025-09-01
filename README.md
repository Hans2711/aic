# aic

AI-assisted git commit message generator.

## Features

- Generates N (configurable) commit message suggestions from OpenAI Chat Completions
- Default model: gpt-4o-mini (override with `AIC_MODEL`)
- Default suggestions: 5 (override with `AIC_SUGGESTIONS` 1-15)
- Optional extra system prompt via `-s` flag
- Interactive selection & auto commit (or copy)

## Install / Build

```bash
./scripts/build.sh
```

Binary output: `dist/aic`

Optionally place it in your PATH:

```bash
sudo mv dist/aic /usr/local/bin/
```

## Usage

```bash
aic aic [-s "extra instruction"]
```

Environment variables:

- `OPENAI_API_KEY` (required)
- `AIC_MODEL` (optional)
- `AIC_SUGGESTIONS` (optional)

## Testing Models

```bash
./scripts/test_models.sh
```

Set MODELS to space separated list to test custom models.

## Limitations / Notes

- Only OpenAI provider supported for now.
- Diff truncated at ~16k chars for safety.
- Suggestions limited to 15 for usability.

## Roadmap Ideas

- Support Anthropic / local providers via interface
- Add non-interactive mode for CI
- Streaming output
