import { getApiKeyForProvider, newProviderClient, type ProviderName } from "../providers";
import { wrapCommitMessage } from "./wrap";
import { stripLeadingListMarker } from "./listmarker";
import { debugVerbose } from "../debug";
import { GENERATION_CONFIG } from "../constants";

export type CombineConfig = {
  provider: ProviderName;
  model: string;
  suggestions: number;
  systemAddition: string;
};

export async function generateCombinedSuggestions(cfg: CombineConfig, selected: string[]): Promise<string[]> {
  if (!selected || selected.length < 2) throw new Error("need at least two messages to combine");
  const apiKey = getApiKeyForProvider(cfg.provider);
  const client = newProviderClient(cfg.provider, apiKey);

  const system =
    "You synthesize multiple draft commit messages into improved, concise natural-language Git commit subjects. " +
    "Rules: write descriptive commit messages, imperative mood, no trailing period; no type prefixes or scopes. " +
    "Prioritize clarity and meaningful detail; describe what changed in specific terms. Do NOT include why changes were made or any rationale. " +
    "Preserve the strongest concrete noun phrases from the drafts (key modules, features, or components), but rewrite with fresh sentence structure and verb choices. " +
    "Do not simply paraphrase one draft; synthesize the shared signal across drafts into stronger subjects. " +
    "Identify the most impactful shared theme (largest scope, user-facing, architectural/security, or release-impacting) and prioritize that in the subject. " +
    "Produce distinct alternatives with different phrasing and emphasis (varied verbs, structures, focus). " +
    "If the message would exceed 80 characters, wrap ONLY between sentences; never break mid-sentence. If a sentence exceeds 80 characters, keep it on a single line. " +
    "Return ONLY the subjects, with no numbering or bullets." +
    (cfg.systemAddition ? " Additional user instructions: " + cfg.systemAddition : "");

  const user = "Combine and refine these commit messages into consolidated alternatives:\n\n" + selected.join("\n");
  debugVerbose("SYSTEM", `combine system prompt: ${system}`);
  debugVerbose("CONTENT", `combine user content length=${user.length}`);

  const maxTokens = 768;

  const messages = [
    { role: "system" as const, content: system },
    { role: "user" as const, content: user },
  ];

  const out: string[] = [];
  const seen = new Set<string>();
  const target = Math.max(1, cfg.suggestions);
  let attempts = 0;
  // First attempt: ask for as many as needed in one go (providers that support n)
  {
    const need = target;
    const temperature = jitterTemperature(
      0,
      GENERATION_CONFIG.COMBINE_TEMPERATURE_MIN,
      GENERATION_CONFIG.COMBINE_TEMPERATURE_MAX,
      GENERATION_CONFIG.COMBINE_TEMPERATURE
    );
    const resp = await client.chat({ model: cfg.model, messages, maxTokens, temperature, n: need });
    for (const raw of resp.choices) {
      for (const s of extractSuggestions(raw)) {
        const norm = s.toLowerCase();
        if (!seen.has(norm)) {
          seen.add(norm);
          out.push(s);
        }
        if (out.length >= target) break;
      }
      if (out.length >= target) break;
    }
  }
  // Refill individually if needed
  while (out.length < target && attempts < target * 3) {
    attempts++;
    const temperature = jitterTemperature(
      attempts,
      GENERATION_CONFIG.COMBINE_TEMPERATURE_MIN,
      GENERATION_CONFIG.COMBINE_TEMPERATURE_MAX,
      GENERATION_CONFIG.COMBINE_TEMPERATURE
    );
    const resp = await client.chat({ model: cfg.model, messages, maxTokens, temperature });
    for (const raw of resp.choices) {
      for (const s of extractSuggestions(raw)) {
        const norm = s.toLowerCase();
        if (!seen.has(norm)) {
          seen.add(norm);
          out.push(s);
        }
        if (out.length >= target) break;
      }
      if (out.length >= target) break;
    }
  }
  // Fallbacks: ensure at least one result
  if (!out.length) {
    // simple merged baseline
    const merged = selected.join("; ");
    out.push(postProcess(merged) || merged);
  }
  return out.slice(0, target);
}

function postProcess(text: string): string {
  const msg = (text || "").trim();
  if (!msg) return "";
  const lines = msg.includes("\n")
    ? msg
        .split("\n")
        .map((l) => l.trim())
        .filter(Boolean)
    : [msg];
  const out: string[] = [];
  for (let ln of lines) {
    ln = stripLeadingListMarker(ln);
    if (!ln) continue;
    out.push(wrapCommitMessage(ln));
  }
  return out[0] || "";
}

function extractSuggestions(text: string): string[] {
  const res: string[] = [];
  const lines = (text || "")
    .split("\n")
    .map((l) => l.trim())
    .filter(Boolean);
  for (let ln of lines) {
    ln = stripLeadingListMarker(ln);
    if (!ln) continue;
    const wrapped = wrapCommitMessage(ln);
    if (wrapped) res.push(wrapped);
  }
  // If the model returned a single paragraph, take it as one suggestion
  if (!res.length) {
    const s = postProcess(text);
    if (s) res.push(s);
  }
  return res;
}

function jitterTemperature(seed: number, min: number, max: number, fallback: number): number {
  if (!Number.isFinite(min) || !Number.isFinite(max) || max <= min) return fallback;
  const value = pseudoRandom01(seed);
  const temp = min + value * (max - min);
  return Number(temp.toFixed(2));
}

function pseudoRandom01(seed: number): number {
  const x = Math.sin((seed + 1) * 12.9898) * 43758.5453;
  return x - Math.floor(x);
}
