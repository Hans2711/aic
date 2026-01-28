let enabled = true;

function detectNoColor(): boolean {
  if (process.env.AIC_NO_COLOR || process.env.NO_COLOR) return true;
  if (!process.stdout.isTTY) return true;
  const term = (process.env.TERM || "").toLowerCase();
  if (term.includes("dumb")) return true;
  return false;
}

export function initColors(explicitNoColor = false) {
  enabled = !(explicitNoColor || detectNoColor());
}

const code = (s: string) => (enabled ? s : "");

export const Color = {
  reset: code("\u001b[0m"),
  bold: code("\u001b[1m"),
  dim: code("\u001b[2m"),
  red: code("\u001b[31m"),
  green: code("\u001b[32m"),
  yellow: code("\u001b[33m"),
  blue: code("\u001b[34m"),
  magenta: code("\u001b[35m"),
  cyan: code("\u001b[36m"),
  gray: code("\u001b[90m"),
};

export const Icon = {
  success: "✓",
  error: "✗",
  prompt: "➤",
  info: "ℹ",
};
