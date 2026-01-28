import type { ProviderClient } from "../providers";
import { debugVerbose, debugInfo, debugMetric } from "../debug";
import { estimateTokens, fingerprintText } from "./tokens";

const chunkSummaryCache = new Map<string, string>();

// Split a unified diff into file-level blocks using lines starting with "diff --git ".
export function splitDiffIntoFiles(diff: string): string[] {
  const lines = diff.split("\n");
  const blocks: string[] = [];
  let cur: string[] = [];
  for (const ln of lines) {
    if (ln.startsWith("diff --git ")) {
      if (cur.length) blocks.push(cur.join("\n"));
      cur = [ln];
    } else {
      cur.push(ln);
    }
  }
  if (cur.length) blocks.push(cur.join("\n"));
  // If no split markers were found, return the whole diff
  if (blocks.length === 0) return [diff];
  return blocks;
}

// Group file blocks into chunks so that each chunk's token count is <= budget.
export async function chunkFilesByBudget(
  client: ProviderClient,
  model: string,
  files: string[],
  budgetTokens: number
): Promise<string[]> {
  const chunks: string[] = [];
  let buf: string[] = [];
  let curTokens = 0;
  for (const originalFile of files) {
    let file = originalFile;
    let tok = await estimateTokens(client, model, file, {
      budgetTokens,
      cacheKey: fingerprintText(file, `${model}:diff-block`),
      label: "diff-block",
    });
    debugVerbose("TOKEN", `file chunk candidate tokens=${tok}, length=${file.length}`);
    if (tok > budgetTokens) {
      const shrunk = shrinkDiffForSummarization(file, budgetTokens);
      if (shrunk.modified) {
        file = shrunk.content;
        tok = await estimateTokens(client, model, file, {
          budgetTokens,
          cacheKey: fingerprintText(file, `${model}:diff-block-shrunk`),
          label: "diff-block-shrunk",
        });
        debugVerbose("CONTENT", `shrunk large diff block; tokens≈${tok}, length=${file.length}`);
      }
      if (tok > budgetTokens) {
        const stub = buildLargeDiffStubForSummaries(file);
        file = stub;
        tok = await estimateTokens(client, model, file, {
          budgetTokens,
          cacheKey: fingerprintText(file, `${model}:diff-block-stub`),
          label: "diff-block-stub",
        });
        debugVerbose("CONTENT", `replaced diff block with stub; tokens≈${tok}, length=${file.length}`);
      }
    }
    if (buf.length === 0) {
      buf.push(file);
      curTokens = tok;
      continue;
    }
    if (curTokens + tok <= budgetTokens) {
      buf.push(file);
      curTokens += tok;
    } else {
      debugVerbose("CONTENT", `closing chunk with tokens≈${curTokens}`);
      chunks.push(buf.join("\n"));
      buf = [file];
      curTokens = tok;
    }
  }
  if (buf.length) chunks.push(buf.join("\n"));
  debugInfo("CONTENT", `chunkFilesByBudget produced ${chunks.length} chunks (budget=${budgetTokens})`);
  return chunks;
}

// Summarize a chunk into concise bullet-like lines highlighting file changes and key impacts.
export async function summarizeChunk(client: ProviderClient, model: string, chunk: string): Promise<string> {
  const system =
    "You summarize git diffs by file. For each file (<=1 line), state the nature of change (add/remove/modify/rename) and highlight API changes, new/removed public functions, dependency/version changes, security-sensitive changes, and configuration updates. After the list, include a short 'Key Impacts:' section (<=3 lines). No commit messages, no speculation. Do not ask for more information; if parts of the diff are large or missing context, still produce the best possible summary from available headers and hunks.";
  const user = chunk;
  debugVerbose("SYSTEM", `summarizeChunk: system prompt: ${system}`);
  debugVerbose("CONTENT", `summarizeChunk: user chunk length=${chunk.length}`);
  const resp = await client.chat({
    model,
    messages: [
      { role: "system", content: system },
      { role: "user", content: user },
    ],
    maxTokens: 512,
    temperature: 0.2,
  });
  let out = (resp.choices?.[0] || "").trim();
  if (!out) {
    // Fallback: derive a minimal summary from diff headers
    out = fallbackSummaryForChunk(chunk);
  }
  return out;
}

async function summarizeChunksConcurrently(
  client: ProviderClient,
  model: string,
  chunks: string[],
  concurrency: number,
  label: string
): Promise<string[]> {
  const total = chunks.length;
  const results = new Array<string>(total);
  let nextIndex = 0;
  const workerCount = Math.max(1, Math.min(concurrency, total));

  const workers = Array.from({ length: workerCount }, () =>
    (async () => {
      while (true) {
        const current = nextIndex;
        if (current >= total) return;
        nextIndex++;
        const chunk = chunks[current];
        const cacheKey = fingerprintText(chunk, `${model}:summary:${label}`);
        const cached = chunkSummaryCache.get(cacheKey);
        if (typeof cached === "string") {
          results[current] = cached;
          continue;
        }
        const summary = await summarizeChunk(client, model, chunk);
        chunkSummaryCache.set(cacheKey, summary);
        debugInfo("WORKER", `summarizeChunk worker(${label}) completed index=${current}`);
        results[current] = summary;
      }
    })()
  );

  await Promise.all(workers);
  return results;
}

// Progressively summarize the full diff without truncation:
// 1) split by file boundaries
// 2) pack into chunks by token budget
// 3) summarize each chunk
// 4) if combined summaries still exceed target budget, re-chunk and summarize again until it fits
export async function progressiveSummarizeDiff(
  client: ProviderClient,
  model: string,
  diff: string,
  chunkBudgetTokens: number,
  targetBudgetTokens: number,
  concurrency: number
): Promise<string> {
  const files = splitDiffIntoFiles(diff);
  debugInfo(
    "CONTENT",
    `progressiveSummarizeDiff: files=${files.length}, chunkBudget=${chunkBudgetTokens}, targetBudget=${targetBudgetTokens}, concurrency=${concurrency}`
  );
  // First-level chunking
  const chunks = await chunkFilesByBudget(client, model, files, chunkBudgetTokens);
  const rawSummaries = await summarizeChunksConcurrently(
    client,
    model,
    chunks,
    Math.min(concurrency, chunks.length),
    "primary"
  );
  let summaries = rawSummaries.map((s, i) => `Chunk ${i + 1}/${chunks.length}\n${s}`);
  let combined = summaries.join("\n\n");
  // Re-summarize until within target budget
  while (
    (await estimateTokens(client, model, combined, {
      budgetTokens: targetBudgetTokens,
      cacheKey: fingerprintText(combined, `${model}:combined-summary`),
      label: "combined-summary",
    })) > targetBudgetTokens &&
    summaries.length > 1
  ) {
    debugInfo("CONTENT", "combined summary over target budget; re-chunking for second-pass summarization");
    // Re-group summaries by budget and summarize groups
    const groupChunks = await chunkFilesByBudget(
      client,
      model,
      summaries,
      Math.max(512, Math.floor(targetBudgetTokens * 0.8))
    );
    const nextRaw = await summarizeChunksConcurrently(
      client,
      model,
      groupChunks,
      Math.min(concurrency, groupChunks.length),
      "secondary"
    );
    summaries = nextRaw;
    combined = summaries.join("\n\n");
    if (summaries.length === 1) break;
  }
  const finalTokens = await estimateTokens(client, model, combined, {
    budgetTokens: targetBudgetTokens,
    cacheKey: fingerprintText(combined, `${model}:combined-summary-final`),
    label: "combined-summary-final",
  });
  debugMetric("TOKEN", `final combined summary tokens≈${finalTokens}`);
  return combined;
}

function fallbackSummaryForChunk(chunk: string): string {
  // Extract file paths from diff headers; support both with and without a/ b/ prefixes
  const files = new Set<string>();
  let m: RegExpExecArray | null;
  const reNoPrefix = /^diff\s+--git\s+(\S+)\s+(\S+)/gm;
  while ((m = reNoPrefix.exec(chunk)) !== null) {
    const a = stripABPrefix(m[1]);
    const b = stripABPrefix(m[2]);
    files.add(b || a);
  }
  if (files.size === 0) {
    const reWithPrefix = /^diff\s+--git\s+a\/(\S+)\s+b\/(\S+)/gm;
    while ((m = reWithPrefix.exec(chunk)) !== null) {
      const a = m[1];
      const b = m[2];
      files.add(b || a);
    }
  }
  if (files.size === 0) {
    const re2a = /^\+\+\+\s+b\/(\S+)/gm;
    const re2b = /^\+\+\+\s+(\S+)/gm;
    while ((m = re2a.exec(chunk)) !== null) files.add(m[1]);
    while ((m = re2b.exec(chunk)) !== null) files.add(stripABPrefix(m[1]));
  }
  if (files.size === 0) return "(summary unavailable)";
  const lines = Array.from(files)
    .slice(0, 50)
    .map((f) => `- ${f}: changed`);
  return `Files changed (fallback):\n${lines.join("\n")}`;
}

function shrinkDiffForSummarization(diffBlock: string, _targetTokens: number): { content: string; modified: boolean } {
  const MAX_TOTAL_LINES = 360;
  const HEAD_LINES = 200;
  const TAIL_LINES = 120;
  const MAX_CHARS = 80_000;
  if (!diffBlock) return { content: diffBlock, modified: false };
  const lines = diffBlock.split(/\r?\n/);
  if (lines.length <= MAX_TOTAL_LINES && diffBlock.length <= MAX_CHARS) {
    return { content: diffBlock, modified: false };
  }
  const headerLines = Math.min(20, lines.length);
  const head = lines.slice(headerLines, Math.min(headerLines + HEAD_LINES, lines.length));
  const tail = TAIL_LINES > 0 ? lines.slice(-TAIL_LINES) : [];
  const marker = `... (diff truncated for summarization; showing first ${head.length} and last ${tail.length} body lines) ...`;
  const combinedLines = [...lines.slice(0, headerLines), ...head];
  if (tail.length) combinedLines.push(marker, ...tail);
  else combinedLines.push(marker);
  let content = combinedLines.join("\n");
  if (content.length > MAX_CHARS) {
    content = content.slice(0, MAX_CHARS) + "\n... (diff further truncated for summarization due to size) ...";
  }
  return { content, modified: true };
}

function buildLargeDiffStubForSummaries(diffBlock: string): string {
  const lines = diffBlock.split(/\r?\n/);
  const header = lines.find((ln) => ln.startsWith("diff --git ")) ?? lines[0] ?? "diff --git a/??? b/???";
  const pathLine =
    lines.find((ln) => ln.startsWith("+++ ")) ?? lines.find((ln) => ln.startsWith("--- ")) ?? "(path unavailable)";
  return `${header}\n${pathLine}\n... (diff omitted for summarization due to size; rely on impact analysis) ...`;
}

function stripABPrefix(p: string): string {
  if (!p) return p;
  if (p.startsWith("a/")) return p.slice(2);
  if (p.startsWith("b/")) return p.slice(2);
  return p;
}
