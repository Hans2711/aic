import { createHash } from "crypto";
import type { ProviderClient } from "../providers";
import { debugLog } from "../debug";

const APPROX_CHARS_PER_TOKEN = 4;
const DEFAULT_MARGIN = 0.85;
const LARGE_TEXT_THRESHOLD_TOKENS = 20000;

const tokenEstimateCache = new Map<string, number>();

export type TokenEstimateOptions = {
  budgetTokens?: number;
  margin?: number;
  cacheKey?: string;
  label?: string;
};

export function approximateTokens(text: string): number {
  if (!text) return 0;
  return Math.ceil(text.length / APPROX_CHARS_PER_TOKEN);
}

export function fingerprintText(text: string, prefix = ""): string {
  const hash = createHash("sha1").update(text).digest("hex");
  return prefix ? `${prefix}:${hash}` : hash;
}

export async function estimateTokens(
  client: ProviderClient | undefined,
  model: string,
  text: string,
  options: TokenEstimateOptions = {}
): Promise<number> {
  const approx = approximateTokens(text);
  const { budgetTokens, margin = DEFAULT_MARGIN, cacheKey = fingerprintText(text, model), label } = options;
  const cached = tokenEstimateCache.get(cacheKey);
  if (typeof cached === "number") return cached;

  const shouldUseApi = (() => {
    if (!client || typeof (client as any).countTokens !== "function") return false;
    if (!text) return false;
    if (budgetTokens) {
      return approx > budgetTokens * margin;
    }
    return approx > LARGE_TEXT_THRESHOLD_TOKENS;
  })();

  if (!shouldUseApi) {
    tokenEstimateCache.set(cacheKey, approx);
    return approx;
  }

  try {
    const counted = await (client as any).countTokens(model, text);
    if (typeof counted === "number" && !Number.isNaN(counted)) {
      tokenEstimateCache.set(cacheKey, counted);
      return counted;
    }
  } catch (err) {
    debugLog("countTokens failed", label ? `${label}:` : "", String(err));
  }

  tokenEstimateCache.set(cacheKey, approx);
  return approx;
}

export function clearTokenEstimateCache(): void {
  tokenEstimateCache.clear();
}
