import { getApiKeyForProvider, newProviderClient, type ProviderName } from "../providers";
import { repoContext, stagedDiff, worktreeDiff } from "../git";
import { progressiveSummarizeDiff } from "./summarize";
import { wrapCommitMessage } from "./wrap";
import { stripLeadingListMarker } from "./listmarker";
import { envBool, loadConfig, daemonEnabled, modelForTokens, Env } from "../config";
import { debugLog } from "../debug";

export type SuggestionConfig = ReturnType<typeof loadConfig> & { provider: ProviderName };

export async function generateSuggestions(cfg: SuggestionConfig): Promise<string[]> {
  if (envBool(Env.AIC_MOCK)) {
    const mock = [
      "Update code for mock change",
      "Fix mock issue in logic",
      "Update dependencies",
      "Refactor component structure",
      "Improve test coverage",
    ];
    return mock.slice(0, Math.max(1, cfg.suggestions));
  }
  const apiKey = getApiKeyForProvider(cfg.provider);
  const client = newProviderClient(cfg.provider, apiKey);

  // Load the appropriate diff
  const diff = (await (daemonEnabled() ? worktreeDiff() : stagedDiff())).trim();
  if (!diff) throw new Error(daemonEnabled() ? "no changes compared to HEAD" : "no staged changes");
  debugLog("diff length=", String(diff.length));

  // Choose model using token count when user didn't force AIC_MODEL
  const forcedModel = (process.env.AIC_MODEL ?? "").trim();
  let chosenModel = cfg.model;
  if (!forcedModel && typeof (client as any).countTokens === "function") {
    try {
      const t = await (client as any).countTokens(cfg.model, diff);
      debugLog("diff tokens≈", String(t));
      chosenModel = modelForTokens(cfg.provider, t);
    } catch {
      // keep existing
    }
  }
  debugLog("provider=", cfg.provider, "model=", chosenModel, forcedModel ? "(forced)" : "(auto)");

  // Choose summarization model and budgets
  const summarizeModel = chosenModel; // reuse current
  const chunkBudget = 6000; // tokens per chunk input (safe default)
  const finalBudget = 12000; // tokens for final combined summary fed into generation

  // Summarize entire diff progressively without truncation
  const summary = await progressiveSummarizeDiff(client, summarizeModel, diff, chunkBudget, finalBudget);
  debugLog("===== SUMMARY START =====\n" + summary + "\n===== SUMMARY END =====");

  // Compose messages
  const ctx = await repoContext();
  let userContent = summary;
  if (ctx) userContent = `${ctx}\n\n${userContent}`;

  let systemMsg = "You write detailed, descriptive Git commit subjects that provide clear context about the changes and their purpose. " +
    "Rules: write messages with meaningful detail, imperative mood, no trailing period; " +
    "Do NOT use type prefixes or scopes (no 'feat:' or 'feat(scope):'). " +
    "Include what changed and why it matters when helpful. " +
    "If the message would exceed 80 characters, wrap ONLY between sentences; never break mid-sentence. If a sentence exceeds 80 characters, keep it on a single line. " +
    "First determine which change is most important/impactful (e.g., largest scope, user-facing effects, architectural/security implications, or release-impacting) and prioritize that in the subject. Do not default to the first file or first bullet in the summary; choose based on impact. If several small changes share a theme, prefer a unifying subject. " +
    "Produce distinct alternatives with different phrasing and emphasis (varied verbs, structures, focus). Prefer richer, more informative subjects; include brief rationale or impact if helpful (use a second line if needed). " +
    "Output only the subjects, one per choice.";
  if (cfg.systemAddition) systemMsg += " Additional user instructions: " + cfg.systemAddition;
  debugLog("suggestions system prompt:", systemMsg);

  // Temperature and limits
  const temperature = 0.6; // encourage diversity
  const maxTokens = 768;   // allow larger, more detailed subjects

  const messages = [
    { role: "system" as const, content: systemMsg },
    { role: "user" as const, content: userContent },
  ];
  debugLog("suggestions user content length=", String(userContent.length));

  const suggestions: string[] = [];
  const seen = new Set<string>();
  const target = Math.max(1, cfg.suggestions);
  let attempts = 0;
  while (suggestions.length < target && attempts < target * 3) {
    attempts++;
    const need = Math.max(1, target - suggestions.length);
    const resp = await client.chat({ model: chosenModel, messages, maxTokens, temperature, n: need });
    for (const raw of resp.choices) {
      const cleaned = postProcess(raw);
      const norm = cleaned.toLowerCase();
      if (cleaned && !seen.has(norm)) {
        seen.add(norm);
        suggestions.push(cleaned);
      }
      if (suggestions.length >= target) break;
    }
  }
  if (suggestions.length === 0) throw new Error("empty suggestions");
  // Ensure exactly target count
  return suggestions.slice(0, target);
}

function postProcess(text: string): string {
  let msg = (text || "").trim();
  if (!msg) return "";
  const lines = msg.includes("\n") ? msg.split("\n").map((l) => l.trim()).filter(Boolean) : [msg];
  const out: string[] = [];
  for (let ln of lines) {
    ln = stripLeadingListMarker(ln);
    if (!ln) continue;
    out.push(wrapCommitMessage(ln));
  }
  return out[0] || "";
}
