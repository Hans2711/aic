import { envBool, Env } from "./config";

export function debugEnabled(): boolean {
  return envBool(Env.AIC_DEBUG);
}

export function debugLog(...args: any[]) {
  if (!debugEnabled()) return;
  // Write to stderr to avoid interfering with normal stdout output
  const msg = args
    .map((a) =>
      typeof a === "string"
        ? a
        : (() => {
            try {
              return JSON.stringify(a, null, 2);
            } catch {
              return String(a);
            }
          })()
    )
    .join(" ");
  process.stderr.write(`[aic][debug] ${msg}\n`);
}
