import { getApiKeyForProvider, newProviderClient, type ProviderName } from "../providers";
import { repoContext, stagedDiff, worktreeDiff } from "../git";
import { progressiveSummarizeDiff, splitDiffIntoFiles } from "./summarize";
import { wrapCommitMessage } from "./wrap";
import { stripLeadingListMarker } from "./listmarker";
import { envBool, loadConfig, daemonEnabled, modelForTokens, Env, getIgnorePrefixes } from "../config";
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
  let diff = (await (daemonEnabled() ? worktreeDiff() : stagedDiff())).trim();
  if (!diff) throw new Error(daemonEnabled() ? "no changes compared to HEAD" : "no staged changes");
  debugLog("diff length=", String(diff.length));

  // Filter out build artifacts and ignored prefixes to improve relevance
  const { filtered, removedCount } = filterDiffByPrefixes(diff, getIgnorePrefixes());
  if (removedCount > 0) {
    debugLog(`filtered out ${removedCount} file block(s) by prefix`);
    diff = filtered.trim() || diff;
  }

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
  let summary = await progressiveSummarizeDiff(client, summarizeModel, diff, chunkBudget, finalBudget);
  debugLog("===== SUMMARY START =====\n" + summary + "\n===== SUMMARY END =====");

  // Compose messages: include FILE LIST, DIFF SUMMARY, and RAW DIFF
  const ctx = await repoContext();
  let userContent = "";
  if (ctx) userContent += ctx + "\n\n";
  const filesList = extractFileList(diff).slice(0, 100);
  const highlights = extractHighlights(summary).slice(0, 10);
  if (filesList.length) {
    userContent += "FILES CHANGED (paths):\n" + filesList.map((f) => `- ${f}`).join("\n") + "\n\n";
  }
  if (highlights.length) {
    userContent += "HIGHLIGHTS (from summary):\n" + highlights.map((h) => `- ${h}`).join("\n") + "\n\n";
  }
  if (summary && summary.trim()) {
    userContent += "DIFF SUMMARY\n" + summary.trim() + "\n\n";
  }
  userContent += "--- BEGIN RAW DIFF ---\n" + diff + "\n--- END RAW DIFF ---";

  let systemMsg = "You write detailed, descriptive Git commit subjects that provide clear context about the changes. " +
    "Rules: write messages with meaningful detail, imperative mood, no trailing period; " +
    "Do NOT use type prefixes or scopes (no 'feat:' or 'feat(scope):'). " +
    "Describe only what changed in clear, specific terms. Do NOT include why the change was made. " +
    "If the message would exceed 80 characters, wrap ONLY between sentences; never break mid-sentence. If a sentence exceeds 80 characters, keep it on a single line. " +
    "Ground your subjects strictly on the provided FILES CHANGED, HIGHLIGHTS, DIFF SUMMARY, and RAW DIFF; do not ask for additional input or return placeholders. " +
    "First determine which change is most important/impactful (e.g., largest scope, user-facing effects, architectural/security implications, or release-impacting) and prioritize that in the subject. Do not default to the first file or first bullet in the summary; choose based on impact. If several small changes share a theme, prefer a unifying subject. " +
    "Produce distinct alternatives with different phrasing and emphasis (varied verbs, structures, focus). Prefer richer, more informative subjects; where appropriate, reflect top HIGHLIGHTS explicitly; include specific details about the changes themselves, but never explain rationale, motivation, or reasons for the change (use a second line if needed). " +
    "Output only the subjects, one per choice.";
  if (cfg.systemAddition) systemMsg += " Additional user instructions: " + cfg.systemAddition;
  debugLog("suggestions system prompt:", systemMsg);

  // Temperature and limits
  const temperature = 0.6; // encourage diversity
  const maxTokens = chosenModel.includes('gpt-5') ? 1500 : 768;   // reasoning models need more tokens

  const messages = [
    { role: "system" as const, content: systemMsg },
    { role: "user" as const, content: userContent },
  ];
  debugLog("suggestions user content length=", String(userContent.length));

  const suggestions: string[] = [];
  const seen = new Set<string>();
  const target = Math.max(1, cfg.suggestions);
  let attempts = 0;
  let retriedWithFallback = false;
  
  while (suggestions.length < target && attempts < target * 3) {
    attempts++;
    const need = Math.max(1, target - suggestions.length);
    
    try {
      const resp = await client.chat({ model: chosenModel, messages, maxTokens, temperature, n: need });
      let emptyCount = 0;
      for (const raw of resp.choices) {
        if (!raw || !raw.trim()) emptyCount++;
        const cleaned = postProcess(raw);
        const norm = cleaned.toLowerCase();
        if (cleaned && !seen.has(norm)) {
          seen.add(norm);
          suggestions.push(cleaned);
        }
        if (suggestions.length >= target) break;
      }
      // Fail fast if all responses were empty (reasoning model using all tokens)
      if (emptyCount === resp.choices.length && emptyCount > 0) {
        throw new Error(`Model returned ${emptyCount} empty response(s). Reasoning model may be using all tokens for internal reasoning.`);
      }
    } catch (error: any) {
      // If reasoning model fails with empty responses and we haven't retried yet, fallback to gpt-4o
      if (!retriedWithFallback && chosenModel.includes('gpt-5') && error.message.includes('empty response')) {
        debugLog(`Reasoning model ${chosenModel} failed with empty responses, falling back to gpt-4o`);
        chosenModel = 'gpt-4o';
        retriedWithFallback = true;
        attempts--; // Don't count this as an attempt
        continue; // Retry with fallback model
      }
      // Otherwise, re-throw the error
      throw error;
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

function filterDiffByPrefixes(diff: string, prefixes: string[]): { filtered: string; removedCount: number } {
  const files = splitDiffIntoFiles(diff);
  const kept: string[] = [];
  let removed = 0;
  for (const block of files) {
    // Identify path from header
    let path = "";
    let m = /^diff\s+--git\s+(\S+)\s+(\S+)/m.exec(block);
    if (m) {
      const a = m[1];
      const b = m[2];
      path = stripAB(a === b ? b : b || a);
    } else {
      m = /^diff\s+--git\s+a\/(\S+)\s+b\/(\S+)/m.exec(block);
      if (m) path = m[2] || m[1];
    }
    const skip = path && prefixes.some((p) => path.startsWith(p));
    if (skip) {
      removed++;
      continue;
    }
    kept.push(block);
  }
  return { filtered: kept.join("\n"), removedCount: removed };
}

function stripAB(p: string): string {
  if (p.startsWith("a/")) return p.slice(2);
  if (p.startsWith("b/")) return p.slice(2);
  return p;
}

function extractFileList(diff: string): string[] {
  const paths = new Set<string>();
  for (const block of splitDiffIntoFiles(diff)) {
    let m = /^diff\s+--git\s+(\S+)\s+(\S+)/m.exec(block);
    if (m) {
      const a = stripAB(m[1]);
      const b = stripAB(m[2]);
      paths.add(b || a);
      continue;
    }
    m = /^diff\s+--git\s+a\/(\S+)\s+b\/(\S+)/m.exec(block);
    if (m) {
      paths.add(m[2] || m[1]);
      continue;
    }
  }
  return Array.from(paths);
}

function extractHighlights(summary: string): string[] {
  if (!summary) return [];
  const lines = summary.split(/\r?\n/);
  const out: string[] = [];
  let inHighlights = false;
  for (let i = 0; i < lines.length; i++) {
    const ln = lines[i].trim();
    if (/^Key Impacts:\s*$/i.test(ln)) { inHighlights = true; continue; }
    if (inHighlights) {
      if (!ln) break;
      const m = /^[-*]\s*(.+)$/.exec(ln);
      if (m) out.push(m[1].trim());
      else break;
    }
  }
  // If no explicit Key Impacts, heuristically take the first few bullet lines
  if (!out.length) {
    for (const ln of lines) {
      const m = /^[-*]\s*(.+)$/.exec(ln.trim());
      if (m) out.push(m[1].trim());
      if (out.length >= 5) break;
    }
  }
  return out;
}
