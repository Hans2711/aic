import { getApiKeyForProvider, newProviderClient, type ProviderName } from "../providers";
import { repoContext, stagedDiff, worktreeDiff } from "../git";
import { progressiveSummarizeDiff, splitDiffIntoFiles } from "./summarize";
import { wrapCommitMessage } from "./wrap";
import { stripLeadingListMarker } from "./listmarker";
import { envBool, loadConfig, daemonEnabled, modelForTokens, Env, getIgnorePrefixes } from "../config";
import { debugLog } from "../debug";
import { estimateTokens, fingerprintText } from "./tokens";

export type SuggestionConfig = ReturnType<typeof loadConfig> & { provider: ProviderName };

const NON_SOURCE_DIR_PREFIXES = [
  "dist/",
  "bin/",
  "build/",
  "out/",
  "coverage/",
  "node_modules/",
  "vendor/",
  "tmp/",
  "temp/",
];

const NON_SOURCE_EXTENSIONS = [
  ".txt",
  ".log",
  ".csv",
  ".tsv",
  ".lock",
  ".patch",
  ".ico",
  ".png",
  ".jpg",
  ".jpeg",
  ".gif",
  ".mp4",
  ".mp3",
  ".zip",
  ".tar",
  ".gz",
  ".tgz",
  ".xz",
  ".7z",
];

const MAX_NON_SOURCE_BLOCK_CHARS = 120_000;

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
  if (!forcedModel) {
    const diffTokens = await estimateTokens(client, cfg.model, diff, {
      budgetTokens: 120_000,
      cacheKey: fingerprintText(diff, `${cfg.model}:full-diff`),
      label: "full-diff-model-select",
    });
    debugLog("diff tokens≈", String(diffTokens));
    chosenModel = modelForTokens(cfg.provider, diffTokens);
  }
  debugLog("provider=", cfg.provider, "model=", chosenModel, forcedModel ? "(forced)" : "(auto)");

  // Choose summarization model and budgets
  const summarizeModel = chosenModel; // reuse current
  const chunkBudget = 6000; // tokens per chunk input (safe default)
  const finalBudget = 12000; // tokens for final combined summary fed into generation

  // Summarize entire diff progressively without truncation
  let summary = await progressiveSummarizeDiff(client, summarizeModel, diff, chunkBudget, finalBudget);
  debugLog("===== SUMMARY START =====\n" + summary + "\n===== SUMMARY END =====");
  const MAX_SUMMARY_CHARS = 6000;
  if (summary.length > MAX_SUMMARY_CHARS) {
    summary = summary.slice(0, MAX_SUMMARY_CHARS) + "\n... (summary truncated due to size, relying on key highlights) ...";
  }

  // Compose messages: include FILE LIST, DIFF SUMMARY, and RAW DIFF
  const ctx = await repoContext();
  let userContent = "";
  if (ctx) userContent += ctx + "\n\n";
  const impact = analyzeDiffImpact(diff);
  const filesList = extractFileList(diff).slice(0, 100);
  const highlights = extractHighlights(summary).slice(0, 10);
  const structuredHighlights = formatStructuredHighlights(highlights, impact).slice(0, 10);
  if (filesList.length) {
    userContent += "FILES CHANGED (paths):\n" + filesList.map((f) => `- ${f}`).join("\n") + "\n\n";
  }
  if (impact.topFiles.length) {
    userContent += "IMPACT CANDIDATES:\n" + impact.topFiles.map((info) => `- ${info.path} (${info.area}) — +${info.additions}/-${info.deletions} lines`).join("\n") + "\n\n";
  }
  if (impact.areaSummaries.length) {
    const areaLines = impact.areaSummaries.slice(0, 5).map((a) => `- ${a.area}: ${a.totalChanges} line changes across ${a.fileCount} file(s)`);
    userContent += "AREA OVERVIEW:\n" + areaLines.join("\n") + "\n\n";
  }
  if (impact.testFiles.length) {
    const tests = impact.testFiles.slice(0, 10).map((f) => `- ${f}`);
    userContent += "TEST FILES CHANGED:\n" + tests.join("\n") + "\n\n";
  }
  if (structuredHighlights.length) {
    userContent += "STRUCTURED HIGHLIGHTS:\n" + structuredHighlights.join("\n") + "\n\n";
  } else if (highlights.length) {
    userContent += "HIGHLIGHTS (from summary):\n" + highlights.map((h) => `- ${h}`).join("\n") + "\n\n";
  }
  if (summary && summary.trim()) {
    userContent += "DIFF SUMMARY\n" + summary.trim() + "\n\n";
  }
  const baseUserContent = userContent;
  const rawDiffInfo = buildRawDiffBlock(diff);
  const baseContext = baseUserContent + (rawDiffInfo.note ? rawDiffInfo.note + "\n\n" : "");
  userContent = baseContext + rawDiffInfo.block;

  let systemMsg = "You write detailed, descriptive Git commit subjects that provide clear context about the changes and their purpose. " +
    "Rules: write messages with meaningful detail, imperative mood, no trailing period; " +
    "Do NOT use type prefixes or scopes (no 'feat:' or 'feat(scope):'). " +
    "Summarize what changed and why it matters in the subject when possible; prefer concise what+why phrasing. " +
    "Use IMPACT CANDIDATES and AREA OVERVIEW to judge which change is primary before drafting. " +
    "If the message would exceed 80 characters, wrap ONLY between sentences; never break mid-sentence. If a sentence exceeds 80 characters, keep it on a single line. " +
    "Ground your subjects strictly on the provided FILES CHANGED, STRUCTURED HIGHLIGHTS, DIFF SUMMARY, and RAW DIFF; do not ask for additional input or return placeholders. " +
    "First determine which change is most important/impactful (e.g., largest scope, user-facing effects, architectural/security implications, or release-impacting) and prioritize that in the subject. Do not default to the first file or first bullet in the summary; choose based on impact. If several small changes share a theme, prefer a unifying subject. " +
    "Produce distinct alternatives with different phrasing and emphasis (varied verbs, structures, focus). Prefer richer, more informative subjects; reflect top STRUCTURED HIGHLIGHTS explicitly when helpful; include a brief rationale or impact as a second line if it improves clarity. " +
    "Output only the subjects, one per choice.";
  if (impact.testFiles.length) {
    systemMsg += " At least one test file changed; call out test coverage or verification when relevant.";
  }
  if (cfg.systemAddition) systemMsg += " Additional user instructions: " + cfg.systemAddition;
  const enforced = await enforceContextLimit(client, chosenModel, systemMsg, baseContext, rawDiffInfo.block);
  userContent = enforced.userContent;
  if (enforced.note) debugLog(enforced.note);
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
  const lines = msg.includes("\n") ? msg.split("\n").map((l) => l.trim()) : [msg];
  const out: string[] = [];
  for (let ln of lines) {
    ln = stripLeadingListMarker(ln);
    if (!ln) continue;
    const wrapped = wrapCommitMessage(ln);
    if (wrapped) out.push(wrapped);
  }
  if (!out.length) return "";
  const flattened = out.join("\n").split("\n").map((ln) => ln.trim()).filter(Boolean);
  return flattened.slice(0, 2).join("\n");
}

type ImpactFileInfo = {
  path: string;
  additions: number;
  deletions: number;
  totalChanges: number;
  area: string;
  isTest: boolean;
  keywords: string[];
  basename: string;
};

type ImpactAreaInfo = {
  area: string;
  totalChanges: number;
  fileCount: number;
  keywords: string[];
};

type ImpactAnalysis = {
  files: ImpactFileInfo[];
  topFiles: ImpactFileInfo[];
  areaSummaries: ImpactAreaInfo[];
  testFiles: string[];
};

function analyzeDiffImpact(diff: string): ImpactAnalysis {
  const blocks = splitDiffIntoFiles(diff);
  const files: ImpactFileInfo[] = [];
  const areaMap = new Map<string, { totalChanges: number; files: Set<string>; keywords: Set<string> }>();
  for (const block of blocks) {
    const path = extractPathFromDiffBlock(block);
    if (!path) continue;
    const { additions, deletions } = countLineChanges(block);
    const totalChanges = additions + deletions;
    const area = deriveAreaFromPath(path);
    const isTest = isTestFile(path);
    const basename = path.split("/").pop() ?? path;
    const keywords = buildKeywords(path, area, basename);
    const info: ImpactFileInfo = { path, additions, deletions, totalChanges, area, isTest, keywords, basename };
    files.push(info);
    const existing = areaMap.get(area);
    if (existing) {
      existing.totalChanges += totalChanges;
      existing.files.add(path);
      keywords.forEach((kw) => existing.keywords.add(kw));
    } else {
      areaMap.set(area, { totalChanges, files: new Set([path]), keywords: new Set(keywords) });
    }
  }
  const topFiles = files
    .slice()
    .sort((a, b) => {
      if (b.totalChanges !== a.totalChanges) return b.totalChanges - a.totalChanges;
      return a.path.localeCompare(b.path);
    })
    .slice(0, 3);
  const areaSummaries: ImpactAreaInfo[] = Array.from(areaMap.entries())
    .map(([area, data]) => ({
      area,
      totalChanges: data.totalChanges,
      fileCount: data.files.size,
      keywords: Array.from(data.keywords),
    }))
    .sort((a, b) => {
      if (b.totalChanges !== a.totalChanges) return b.totalChanges - a.totalChanges;
      return a.area.localeCompare(b.area);
    });
  const testFiles = files.filter((f) => f.isTest).map((f) => f.path);
  return { files, topFiles, areaSummaries, testFiles };
}

function formatStructuredHighlights(highlights: string[], impact: ImpactAnalysis): string[] {
  const results: string[] = [];
  for (const raw of highlights) {
    const cleaned = collapseWhitespace(raw);
    if (!cleaned) continue;
    const { change, rationale } = splitHighlightChange(cleaned);
    const inferredArea = inferAreaFromHighlight(cleaned, impact) ?? inferAreaFromHighlight(change, impact) ?? inferAreaFallback(impact);
    const parts = [`Area: ${inferredArea}`];
    parts.push(`Change: ${capitalizeSentence(change)}`);
    if (rationale) parts.push(`Rationale: ${capitalizeSentence(rationale)}`);
    results.push("- " + parts.join(" | "));
  }
  return results;
}

function inferAreaFallback(impact: ImpactAnalysis): string {
  return impact.topFiles[0]?.area || impact.areaSummaries[0]?.area || "general";
}

function inferAreaFromHighlight(text: string, impact: ImpactAnalysis): string | undefined {
  const lower = text.toLowerCase();
  let bestArea: string | undefined;
  let bestScore = 0;
  for (const file of impact.files) {
    const score = keywordScore(lower, file.keywords) + (lower.includes(file.basename.toLowerCase()) ? 1 : 0);
    if (score > bestScore) {
      bestScore = score;
      bestArea = file.area;
    }
  }
  if (bestArea) return bestArea;
  for (const area of impact.areaSummaries) {
    const score = keywordScore(lower, area.keywords);
    if (score > bestScore) {
      bestScore = score;
      bestArea = area.area;
    }
  }
  if (/test/i.test(text) || /coverage/i.test(text)) return "tests";
  return bestArea;
}

function keywordScore(text: string, keywords: string[]): number {
  let score = 0;
  for (const kw of keywords) {
    if (!kw || kw.length < 3) continue;
    if (text.includes(kw)) score += 1;
  }
  return score;
}

function splitHighlightChange(text: string): { change: string; rationale: string } {
  const sentenceMatches = text.match(/[^.!?]+[.!?]?/g);
  const sentences = sentenceMatches ? sentenceMatches.map((s) => s.trim()).filter(Boolean) : [text.trim()];
  if (!sentences.length) return { change: text, rationale: "" };
  let change = sentences[0];
  let rationale = sentences.slice(1).join(" ");
  if (!rationale) {
    const toMatch = /(.+?)\s+to\s+(.+)/i.exec(change);
    if (toMatch) {
      change = toMatch[1];
      rationale = toMatch[2];
    }
  }
  return { change: collapseWhitespace(change), rationale: collapseWhitespace(rationale) };
}

function collapseWhitespace(text: string): string {
  return text.replace(/\s+/g, " ").trim();
}

function capitalizeSentence(text: string): string {
  const trimmed = text.trim();
  if (!trimmed) return "";
  return trimmed[0].toUpperCase() + trimmed.slice(1);
}

function extractPathFromDiffBlock(block: string): string | "" {
  let m = /^diff\s+--git\s+(\S+)\s+(\S+)/m.exec(block);
  if (m) {
    const a = stripAB(m[1]);
    const b = stripAB(m[2]);
    return b || a;
  }
  m = /^diff\s+--git\s+a\/(\S+)\s+b\/(\S+)/m.exec(block);
  if (m) return m[2] || m[1];
  return "";
}

function countLineChanges(block: string): { additions: number; deletions: number } {
  let additions = 0;
  let deletions = 0;
  const lines = block.split(/\r?\n/);
  for (const line of lines) {
    if (!line) continue;
    if (line.startsWith("+++ ") || line.startsWith("--- ")) continue;
    if (line.startsWith("+")) additions++;
    else if (line.startsWith("-")) deletions++;
  }
  return { additions, deletions };
}

function deriveAreaFromPath(path: string): string {
  const segments = path.split("/").filter(Boolean);
  if (!segments.length) return "root";
  if (segments[0] === "src" && segments.length >= 2) return `src/${segments[1]}`;
  if (["packages", "apps", "services", "lib", "modules"].includes(segments[0]) && segments.length >= 2) {
    return `${segments[0]}/${segments[1]}`;
  }
  return segments[0];
}

function buildKeywords(path: string, area: string, basename: string): string[] {
  const set = new Set<string>();
  const lowerArea = area.toLowerCase();
  if (lowerArea) set.add(lowerArea);
  for (const segment of path.split("/")) {
    const clean = segment.replace(/\.[^/.]+$/, "").toLowerCase();
    if (clean && clean.length >= 3) set.add(clean);
  }
  const baseClean = basename.replace(/\.[^/.]+$/, "").toLowerCase();
  if (baseClean && baseClean.length >= 3) set.add(baseClean);
  return Array.from(set);
}

function isTestFile(path: string): boolean {
  return /(^|\/)tests?\//i.test(path) || /\.(test|spec)\./i.test(path);
}

async function enforceContextLimit(
  client: any,
  model: string,
  systemMsg: string,
  baseContent: string,
  rawDiffBlock: string,
): Promise<{ userContent: string; note?: string }> {
  const MAX_CONTEXT_TOKENS = 90_000;
  const MAX_CONTEXT_CHARS = 280_000;
  const attempts: Array<{ raw: string; note?: string }> = [];
  const seen = new Set<string>();

  const initialKey = rawDiffBlock;
  if (initialKey && !seen.has(initialKey)) {
    attempts.push({ raw: rawDiffBlock });
    seen.add(initialKey);
  }

  const truncationTargets = [600, 400, 200];
  for (const maxLines of truncationTargets) {
    const truncated = truncateRawDiffBlock(rawDiffBlock, maxLines);
    if (!truncated.truncated) continue;
    if (seen.has(truncated.block)) continue;
    seen.add(truncated.block);
    const note = `NOTE: RAW DIFF truncated to first ${truncated.headLines} and last ${truncated.tailLines} lines (limit ${maxLines}) due to context limit.`;
    attempts.push({ raw: truncated.block, note });
  }

  const omitNote = "NOTE: RAW DIFF omitted due to context limit; rely on structured context above.";
  attempts.push({ raw: "", note: omitNote });

  const fallback: { userContent: string; note?: string } = { userContent: baseContent };

  let attemptIndex = 0;
  for (const attempt of attempts) {
    const noteSegment = attempt.note ? attempt.note + "\n\n" : "";
    const candidateContent = baseContent + noteSegment + (attempt.raw || "");
    if (candidateContent.length > MAX_CONTEXT_CHARS) {
      debugLog(`context length ${candidateContent.length} chars exceeds soft cap ${MAX_CONTEXT_CHARS}`);
      attemptIndex++;
      continue;
    }
    const combined = `SYSTEM:\n${systemMsg}\nUSER:\n${candidateContent}`;
    const tokenEstimate = await estimateTokens(client, model, combined, {
      budgetTokens: MAX_CONTEXT_TOKENS,
      cacheKey: fingerprintText(combined, `${model}:context-${attemptIndex}`),
      label: "context-candidate",
    });
    attemptIndex++;
    if (tokenEstimate <= MAX_CONTEXT_TOKENS) {
      if (attempt.note && attempt.raw) {
        debugLog(`context trimmed: ${attempt.note}`);
      }
      if (attempt.note && !attempt.raw) {
        debugLog("context trimmed: raw diff omitted due to size");
      }
      return { userContent: candidateContent, note: attempt.note };
    }
  }

  return fallback;
}

function buildRawDiffBlock(diff: string): { block: string; note?: string } {
  const RAW_DIFF_OMIT_LINE_THRESHOLD = 800;
  const RAW_DIFF_OMIT_CHAR_THRESHOLD = 160_000;
  const RAW_DIFF_HARD_CAP = 300; // total diff lines (excluding header/footer) allowed before truncation
  const RAW_DIFF_CHAR_CAP = 80_000; // approx character cap for raw diff block
  let note: string | undefined;
  const lineCount = diff ? diff.split(/\r?\n/).length : 0;
  const charCount = diff.length;
  if (lineCount > RAW_DIFF_OMIT_LINE_THRESHOLD || charCount > RAW_DIFF_OMIT_CHAR_THRESHOLD) {
    const omitNote = `NOTE: RAW DIFF omitted due to size (${lineCount} lines, ${charCount} characters); rely on structured context above.`;
    return { block: "", note: omitNote };
  }
  const raw = "--- BEGIN RAW DIFF ---\n" + diff + "\n--- END RAW DIFF ---";
  let block = raw;
  if (lineCount > RAW_DIFF_HARD_CAP) {
    const truncated = truncateRawDiffBlock(raw, RAW_DIFF_HARD_CAP + 2);
    block = truncated.block;
    const lineNote = `NOTE: RAW DIFF truncated to first ${truncated.headLines} and last ${truncated.tailLines} lines (of ${lineCount}) to stay within context.`;
    note = lineNote;
  }
  const charResult = truncateRawDiffByChars(block, RAW_DIFF_CHAR_CAP);
  if (charResult.truncated) {
    block = charResult.block;
    const charNote = `NOTE: RAW DIFF truncated to ~${RAW_DIFF_CHAR_CAP} characters (from ${charResult.originalLength}) to stay within context.`;
    note = note ? `${note}\n${charNote}` : charNote;
  }
  return { block, note };
}

function truncateRawDiffBlock(
  rawBlock: string,
  maxLines: number,
): { block: string; truncated: boolean; headLines: number; tailLines: number } {
  if (!rawBlock) return { block: "", truncated: false, headLines: 0, tailLines: 0 };
  const lines = rawBlock.split(/\r?\n/);
  if (lines.length <= maxLines) {
    const bodyLength = Math.max(lines.length - 2, 0);
    return { block: rawBlock, truncated: false, headLines: bodyLength, tailLines: 0 };
  }
  const header = lines[0];
  const footer = lines[lines.length - 1];
  const body = lines.slice(1, -1);
  const available = Math.max(maxLines - 3, 0);
  const headCount = Math.max(Math.floor(available / 2), 0);
  const tailCount = Math.max(available - headCount, 0);
  const head = body.slice(0, headCount);
  const tail = tailCount > 0 ? body.slice(-tailCount) : [];
  const marker = `... (truncated raw diff, showing first ${head.length} and last ${tail.length} lines) ...`;
  const combined = [header, ...head, marker, ...tail, footer].join("\n");
  return { block: combined, truncated: true, headLines: head.length, tailLines: tail.length };
}

function truncateRawDiffByChars(
  rawBlock: string,
  charLimit: number,
): { block: string; truncated: boolean; originalLength: number } {
  if (!rawBlock) return { block: "", truncated: false, originalLength: 0 };
  const originalLength = rawBlock.length;
  if (originalLength <= charLimit) {
    return { block: rawBlock, truncated: false, originalLength };
  }
  const lines = rawBlock.split(/\r?\n/);
  if (lines.length <= 2) {
    const header = lines[0] ?? "--- BEGIN RAW DIFF ---";
    const footer = lines[1] ?? "--- END RAW DIFF ---";
    const headSlice = header.slice(0, Math.max(Math.floor(charLimit / 2), 1));
    const tailSlice = footer.slice(-Math.max(Math.floor(charLimit / 2), 1));
    const marker = "... (truncated raw diff for size) ...";
    const combined = [headSlice, marker, tailSlice].join("\n");
    return { block: combined, truncated: true, originalLength };
  }
  const header = lines[0];
  const footer = lines[lines.length - 1];
  const body = lines.slice(1, -1);
  const marker = "... (truncated raw diff for size) ...";
  const budget = Math.max(charLimit - header.length - footer.length - marker.length - 2, 0);
  if (budget <= 0) {
    const combined = [header, marker, footer].join("\n");
    return { block: combined, truncated: true, originalLength };
  }
  const headLines: string[] = [];
  const tailLines: string[] = [];
  let headChars = 0;
  const headBudget = Math.floor(budget / 2);
  for (const line of body) {
    const next = line.length + 1;
    if (headChars + next > headBudget) break;
    headLines.push(line);
    headChars += next;
  }
  let tailChars = 0;
  const residualBudget = budget - headChars;
  for (let i = body.length - 1; i >= headLines.length; i--) {
    const line = body[i];
    const next = line.length + 1;
    if (tailChars + next > residualBudget) break;
    tailLines.unshift(line);
    tailChars += next;
  }
  if (headLines.length + tailLines.length >= body.length) {
    return { block: rawBlock, truncated: false, originalLength };
  }
  const markerLine = `${marker} (showing ${headLines.length} head lines and ${tailLines.length} tail lines)`;
  const combined = [header, ...headLines, markerLine, ...tailLines, footer].join("\n");
  return { block: combined, truncated: true, originalLength };
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
    if (path && shouldSkipDiffBlock(path, block)) {
      debugLog(`filtered diff block by heuristic: ${path}`);
      removed++;
      continue;
    }
    kept.push(block);
  }
  return { filtered: kept.join("\n"), removedCount: removed };
}

function shouldSkipDiffBlock(path: string, block: string): boolean {
  const lower = path.toLowerCase();
  if (NON_SOURCE_DIR_PREFIXES.some((dir) => lower.startsWith(dir))) return true;
  if (NON_SOURCE_EXTENSIONS.some((ext) => lower.endsWith(ext))) return true;
  if (isLikelyBinaryBlock(block)) return true;
  if (isLikelyHugeDiff(block, lower)) return true;
  return false;
}

function isLikelyBinaryBlock(block: string): boolean {
  return /Binary files\s+[ab]\//i.test(block) || /GIT binary patch/i.test(block);
}

function isLikelyHugeDiff(block: string, lowerPath: string): boolean {
  if (block.length <= MAX_NON_SOURCE_BLOCK_CHARS) return false;
  // Allow large code diffs that contain hunk headers
  if (/^@@/m.test(block)) return false;
  if (NON_SOURCE_EXTENSIONS.some((ext) => lowerPath.endsWith(ext))) return true;
  return false;
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
