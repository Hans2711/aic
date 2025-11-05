import type { ProviderClient } from "../providers";
import { debugLog } from "../debug";

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

async function tokenCount(client: ProviderClient, model: string, text: string): Promise<number> {
  if (typeof client.countTokens === "function") {
    try { return await client.countTokens(model, text); } catch {}
  }
  // Fallback heuristic
  return Math.ceil([...text].length / 4);
}

// Group file blocks into chunks so that each chunk's token count is <= budget.
export async function chunkFilesByBudget(client: ProviderClient, model: string, files: string[], budgetTokens: number): Promise<string[]> {
  const chunks: string[] = [];
  let buf: string[] = [];
  let curTokens = 0;
  for (const file of files) {
    const tok = await tokenCount(client, model, file);
    debugLog(`file chunk candidate tokens=${tok}, length=${file.length}`);
    if (buf.length === 0) {
      buf.push(file);
      curTokens = tok;
      continue;
    }
    if (curTokens + tok <= budgetTokens) {
      buf.push(file);
      curTokens += tok;
    } else {
      debugLog(`closing chunk with tokens≈${curTokens}`);
      chunks.push(buf.join("\n"));
      buf = [file];
      curTokens = tok;
    }
  }
  if (buf.length) chunks.push(buf.join("\n"));
  debugLog(`chunkFilesByBudget produced ${chunks.length} chunks (budget=${budgetTokens})`);
  return chunks;
}

// Summarize a chunk into concise bullet-like lines highlighting file changes and key impacts.
export async function summarizeChunk(client: ProviderClient, model: string, chunk: string): Promise<string> {
  const system = "You summarize git diffs by file. For each file (<=1 line), state the nature of change (add/remove/modify/rename) and highlight API changes, new/removed public functions, dependency/version changes, security-sensitive changes, and configuration updates. After the list, include a short 'Key Impacts:' section (<=3 lines). No commit messages, no speculation.";
  const user = chunk;
  debugLog("summarizeChunk: system prompt:", system);
  debugLog("summarizeChunk: user chunk length=", String(chunk.length));
  const resp = await client.chat({ model, messages: [ { role: "system", content: system }, { role: "user", content: user } ], maxTokens: 512, temperature: 0.2 });
  if (!resp.choices?.length) return "";
  return (resp.choices[0] || "").trim();
}

// Progressively summarize the full diff without truncation:
// 1) split by file boundaries
// 2) pack into chunks by token budget
// 3) summarize each chunk
// 4) if combined summaries still exceed target budget, re-chunk and summarize again until it fits
export async function progressiveSummarizeDiff(client: ProviderClient, model: string, diff: string, chunkBudgetTokens: number, targetBudgetTokens: number): Promise<string> {
  const files = splitDiffIntoFiles(diff);
  debugLog(`progressiveSummarizeDiff: files=${files.length}, chunkBudget=${chunkBudgetTokens}, targetBudget=${targetBudgetTokens}`);
  // First-level chunking
  let chunks = await chunkFilesByBudget(client, model, files, chunkBudgetTokens);
  let summaries: string[] = [];
  for (let i = 0; i < chunks.length; i++) {
    const s = await summarizeChunk(client, model, chunks[i]);
    summaries.push(`Chunk ${i + 1}/${chunks.length}\n${s}`);
  }
  let combined = summaries.join("\n\n");
  // Re-summarize until within target budget
  while (await tokenCount(client, model, combined) > targetBudgetTokens && summaries.length > 1) {
    debugLog("combined summary over target budget; re-chunking for second-pass summarization");
    // Re-group summaries by budget and summarize groups
    const groupChunks = await chunkFilesByBudget(client, model, summaries, Math.max(512, Math.floor(targetBudgetTokens * 0.8)));
    const next: string[] = [];
    for (let i = 0; i < groupChunks.length; i++) {
      const s = await summarizeChunk(client, model, groupChunks[i]);
      next.push(s);
    }
    summaries = next;
    combined = summaries.join("\n\n");
    if (summaries.length === 1) break;
  }
  debugLog("final combined summary tokens≈", String(await tokenCount(client, model, combined)));
  return combined;
}
