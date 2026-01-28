import type { ProviderName } from "./providers/index";
import { TOKEN_LIMITS, GENERATION_CONFIG, DEFAULT_IGNORE_PREFIXES } from "./constants";

// Env keys
export const Env = {
  OPENAI_API_KEY: "OPENAI_API_KEY",
  CLAUDE_API_KEY: "CLAUDE_API_KEY",
  GEMINI_API_KEY: "GEMINI_API_KEY",
  CUSTOM_API_KEY: "CUSTOM_API_KEY",

  AIC_MODEL: "AIC_MODEL",
  AIC_MODEL_SMALL: "AIC_MODEL_SMALL",
  AIC_MODEL_LARGE: "AIC_MODEL_LARGE",
  AIC_SUGGESTIONS: "AIC_SUGGESTIONS",
  AIC_PROVIDER: "AIC_PROVIDER",
  AIC_COMBINE_PROVIDER: "AIC_COMBINE_PROVIDER",
  AIC_COMBINE_MODEL: "AIC_COMBINE_MODEL",
  AIC_COMBINE_SUGGESTIONS: "AIC_COMBINE_SUGGESTIONS",
  AIC_DEBUG: "AIC_DEBUG",
  AIC_MOCK: "AIC_MOCK",
  AIC_NON_INTERACTIVE: "AIC_NON_INTERACTIVE",
  AIC_AUTO_COMMIT: "AIC_AUTO_COMMIT",
  AIC_NO_COLOR: "AIC_NO_COLOR",
  AIC_DAEMON: "AIC_DAEMON",
  AIC_DEAMON_ALIAS: "AIC_DEAMON",

  NO_COLOR: "NO_COLOR",
  TERM: "TERM",
  COLUMNS: "COLUMNS",

  CUSTOM_BASE_URL: "CUSTOM_BASE_URL",
  CUSTOM_CHAT_COMPLETIONS_PATH: "CUSTOM_CHAT_COMPLETIONS_PATH",
  AIC_IGNORE_PREFIXES: "AIC_IGNORE_PREFIXES",
  AIC_SUMMARIZE_CONCURRENCY: "AIC_SUMMARIZE_CONCURRENCY",
  AIC_SUGGESTION_CONCURRENCY: "AIC_SUGGESTION_CONCURRENCY",
} as const;

export function getEnv(key: string): string {
  return process.env[key] ?? "";
}

export function envBool(key: string): boolean {
  const v = (process.env[key] ?? "").trim().toLowerCase();
  if (!v) return false;
  if (["0", "false", "no", "off"].includes(v)) return false;
  return true;
}

export function envIntInRange(key: string, def: number, min: number, max: number): number {
  const raw = (process.env[key] ?? "").trim();
  const n = Number.parseInt(raw, 10);
  if (!Number.isFinite(n)) return def;
  if (n < min || n > max) return def;
  return n;
}

export function warnUnknownAICEnv() {
  const known = new Set<string>([
    Env.AIC_MODEL,
    Env.AIC_MODEL_SMALL,
    Env.AIC_MODEL_LARGE,
    Env.AIC_SUGGESTIONS,
    Env.AIC_MOCK,
    Env.AIC_DEBUG,
    Env.AIC_NON_INTERACTIVE,
    Env.AIC_AUTO_COMMIT,
    Env.AIC_NO_COLOR,
    Env.AIC_DAEMON,
    Env.AIC_DEAMON_ALIAS,
    Env.AIC_PROVIDER,
    Env.AIC_COMBINE_PROVIDER,
    Env.AIC_COMBINE_MODEL,
    Env.AIC_COMBINE_SUGGESTIONS,
    Env.CUSTOM_BASE_URL,
    Env.CUSTOM_CHAT_COMPLETIONS_PATH,
    Env.CUSTOM_API_KEY,
    Env.AIC_IGNORE_PREFIXES,
    Env.AIC_SUMMARIZE_CONCURRENCY,
    Env.AIC_SUGGESTION_CONCURRENCY,
  ]);
  let headerPrinted = false;
  for (const [k] of Object.entries(process.env)) {
    if (!k.startsWith("AIC_")) continue;
    if (known.has(k)) continue;
    if (!headerPrinted) {
      console.error("[aic] Notes about environment variables:");
      headerPrinted = true;
    }
    console.error(`  - ${k} is not recognized; check for typos or remove it.`);
  }
}

export function daemonEnabled(): boolean {
  return envBool(Env.AIC_DAEMON) || envBool(Env.AIC_DEAMON_ALIAS);
}

// Model defaults (parity with Go implementation)
const openAISmall = "gpt-5-nano";
const openAILarge = "gpt-5-mini";
const claudeSmall = "claude-haiku-3";
const claudeLarge = "claude-sonnet-4-20250514";
const geminiSmall = "gemini-2.5-flash";
const geminiLarge = "gemini-2.5-pro";

export function smallModelFor(provider: ProviderName): string {
  const v = getEnv(Env.AIC_MODEL_SMALL).trim();
  if (v) return v;
  switch (provider) {
    case "claude":
      return claudeSmall;
    case "gemini":
      return geminiSmall;
    case "custom":
    default:
      return openAISmall;
  }
}

export function largeModelFor(provider: ProviderName): string {
  const v = getEnv(Env.AIC_MODEL_LARGE).trim();
  if (v) return v;
  switch (provider) {
    case "claude":
      return claudeLarge;
    case "gemini":
      return geminiLarge;
    case "custom":
    default:
      return openAILarge;
  }
}

function defaultModelFor(provider: ProviderName): string {
  return largeModelFor(provider);
}

export function defaultCombineModelFor(provider: ProviderName): string {
  return smallModelFor(provider);
}

export function modelForTokens(provider: ProviderName, tokens: number): string {
  if (tokens < TOKEN_LIMITS.SMALL_MODEL_THRESHOLD) return smallModelFor(provider);
  return largeModelFor(provider);
}

export type RuntimeConfig = {
  provider: ProviderName;
  model: string;
  suggestions: number;
  systemAddition: string;
  summarizeConcurrency: number;
  suggestionConcurrency: number;
};

export function autodetectProvider(): ProviderName {
  const forced = getEnv(Env.AIC_PROVIDER).trim().toLowerCase() as ProviderName | "";
  if (forced) return forced;
  const hasOpenAI = !!getEnv(Env.OPENAI_API_KEY).trim();
  const hasClaude = !!getEnv(Env.CLAUDE_API_KEY).trim();
  const hasGemini = !!getEnv(Env.GEMINI_API_KEY).trim();
  if (hasOpenAI) return "openai";
  if (hasClaude) return "claude";
  if (hasGemini) return "gemini";
  return "openai"; // default; error surfaced when key missing during real calls
}

export function loadConfig(systemAddition = ""): RuntimeConfig {
  const provider = autodetectProvider();
  let model = defaultModelFor(provider);
  const suggestionsDefault = envBool(Env.AIC_NON_INTERACTIVE)
    ? GENERATION_CONFIG.DEFAULT_SUGGESTIONS_CI
    : GENERATION_CONFIG.DEFAULT_SUGGESTIONS;
  const suggestions = envIntInRange(
    Env.AIC_SUGGESTIONS,
    suggestionsDefault,
    GENERATION_CONFIG.SUGGESTIONS_MIN,
    GENERATION_CONFIG.SUGGESTIONS_MAX
  );
  const userModel = getEnv(Env.AIC_MODEL).trim();
  if (userModel) model = userModel;
  const summarizeConcurrency = envIntInRange(
    Env.AIC_SUMMARIZE_CONCURRENCY,
    GENERATION_CONFIG.DEFAULT_SUMMARIZE_CONCURRENCY,
    GENERATION_CONFIG.CONCURRENCY_MIN,
    GENERATION_CONFIG.CONCURRENCY_MAX
  );
  const suggestionConcurrency = envIntInRange(
    Env.AIC_SUGGESTION_CONCURRENCY,
    GENERATION_CONFIG.DEFAULT_SUGGESTION_CONCURRENCY,
    GENERATION_CONFIG.CONCURRENCY_MIN,
    GENERATION_CONFIG.CONCURRENCY_MAX
  );
  return {
    provider,
    model,
    suggestions,
    systemAddition: systemAddition.trim(),
    summarizeConcurrency,
    suggestionConcurrency,
  };
}

export function loadCombineConfig(systemAddition = ""): RuntimeConfig {
  // Start from base config
  const base = loadConfig(systemAddition);
  let provider = base.provider;
  const ovProvider = getEnv(Env.AIC_COMBINE_PROVIDER).trim().toLowerCase() as ProviderName | "";
  if (ovProvider) provider = ovProvider;

  // Determine model for combine step
  const combineModel = getEnv(Env.AIC_COMBINE_MODEL).trim();
  const userModel = getEnv(Env.AIC_MODEL).trim();
  let model = base.model;
  if (combineModel) {
    model = combineModel;
  } else {
    if (ovProvider || !userModel) {
      model = defaultCombineModelFor(provider);
    }
    if (provider === "openai" && model === "gpt-5") model = "gpt-5-2025-08-07";
  }

  // Suggestions override for combine
  const suggestions = envIntInRange(
    Env.AIC_COMBINE_SUGGESTIONS,
    base.suggestions,
    GENERATION_CONFIG.SUGGESTIONS_MIN,
    GENERATION_CONFIG.SUGGESTIONS_MAX
  );
  return {
    provider,
    model,
    suggestions,
    systemAddition: base.systemAddition,
    summarizeConcurrency: base.summarizeConcurrency,
    suggestionConcurrency: base.suggestionConcurrency,
  };
}

export function helpEnvRowsCore(): Array<[string, string]> {
  return [
    [Env.OPENAI_API_KEY, "(required for provider=openai) OpenAI API key"],
    [Env.CLAUDE_API_KEY, "(required for provider=claude) Claude API key"],
    [Env.GEMINI_API_KEY, "(required for provider=gemini) Gemini API key"],
    [Env.CUSTOM_API_KEY, "(optional for provider=custom) API key for custom server"],
    [Env.AIC_MODEL, "(optional) Override model selection (replaces small/large defaults)"],
    [Env.AIC_MODEL_SMALL, "(optional) Override small model for simple diffs"],
    [Env.AIC_MODEL_LARGE, "(optional) Override large model for complex diffs"],
    [Env.AIC_SUGGESTIONS, "(optional) Number of suggestions to generate (1-10, default: 5, CI: 1)"],
    [Env.AIC_PROVIDER, "(optional) AI provider: openai, claude, gemini, or custom (default: auto-detect)"],
    [Env.AIC_COMBINE_PROVIDER, "(optional) Provider for combine step (default: same as AIC_PROVIDER)"],
    [Env.AIC_COMBINE_MODEL, "(optional) Model to use for combining suggestions"],
    [
      Env.AIC_COMBINE_SUGGESTIONS,
      "(optional) Number of suggestions for combine step (default: same as AIC_SUGGESTIONS)",
    ],
    [Env.AIC_DEBUG, "(optional) Debug level: 1=basic info, 2=verbose details (default: 0/off)"],
    [Env.AIC_MOCK, "(optional) Set to 1 to use mock suggestions without API calls"],
    [Env.AIC_NON_INTERACTIVE, "(optional) Set to 1 to auto-select first suggestion and skip commit prompt"],
    [Env.AIC_AUTO_COMMIT, "(optional) With AIC_NON_INTERACTIVE, automatically perform the commit"],
    [Env.AIC_NO_COLOR, "(optional) Disable colored output (same as --no-color flag)"],
    [Env.AIC_DAEMON, "(optional) Daemon mode with minimal output using worktree diff"],
    [Env.AIC_DEAMON_ALIAS, "(alias) Legacy spelling of AIC_DAEMON"],
    [
      Env.AIC_IGNORE_PREFIXES,
      "(optional) Comma-separated path prefixes to ignore in diffs (default: dist/,node_modules/,build/,out/,coverage/,target/,.next/,.turbo/)",
    ],
    [Env.AIC_SUMMARIZE_CONCURRENCY, "(optional) Concurrent API calls for diff summarization (1-20, default: 10)"],
    [Env.AIC_SUGGESTION_CONCURRENCY, "(optional) Concurrent API calls for suggestions (1-20, default: 10)"],
    [Env.NO_COLOR, "(optional) Standard environment variable to disable colors"],
    [Env.TERM, "(optional) Terminal type for color detection"],
    [Env.COLUMNS, "(optional) Override terminal width for UI layout"],
  ];
}

export function getIgnorePrefixes(): string[] {
  const raw = getEnv(Env.AIC_IGNORE_PREFIXES).trim();
  if (!raw) return [...DEFAULT_IGNORE_PREFIXES];
  return raw
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
}

export function helpEnvRowsCustom(): Array<[string, string]> {
  return [
    [Env.CUSTOM_BASE_URL, "(custom provider) Base URL for custom API server (default: http://127.0.0.1:1234)"],
    [
      Env.CUSTOM_CHAT_COMPLETIONS_PATH,
      "(custom provider) Chat completions endpoint path (default: /v1/chat/completions)",
    ],
  ];
}
