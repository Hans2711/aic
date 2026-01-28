import { Env } from "./config";
import { Color, Icon } from "./ui/colors";

// Debug level: 0=off, 1=basic, 2=verbose
let debugLevel = -1; // -1 means not initialized
let startTime = 0;

function getDebugLevel(): number {
  // Always re-read from environment to support dynamic changes in tests
  const val = process.env[Env.AIC_DEBUG];
  if (!val) return 0;
  
  // Parse as number, default to 1 for any truthy value
  let level = 0;
  if (val === "1" || val === "true" || val === "yes" || val === "on") {
    level = 1;
  } else if (val === "2") {
    level = 2;
  } else if (val === "0" || val === "false" || val === "no" || val === "off") {
    level = 0;
  } else {
    level = 1; // Default to basic debug for any other value
  }
  
  if (level > 0 && startTime === 0) {
    startTime = Date.now();
  }
  
  return level;
}

export function debugEnabled(minLevel: number = 1): boolean {
  return getDebugLevel() >= minLevel;
}

function getTimestamp(): string {
  if (startTime === 0) return "";
  const elapsed = (Date.now() - startTime) / 1000;
  return `${Color.dim}[+${elapsed.toFixed(3)}s]${Color.reset}`;
}

function formatValue(value: any): string {
  if (typeof value === "string") {
    return value;
  }
  
  if (typeof value === "number") {
    // Format numbers with thousands separators
    return value.toLocaleString();
  }
  
  if (value === null || value === undefined) {
    return String(value);
  }
  
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

export type LogCategory = "API" | "TOKEN" | "MODEL" | "GIT" | "WORKER" | "CONTENT" | "SYSTEM";
export type LogLevel = "info" | "warn" | "error" | "success" | "metric" | "verbose";

interface LogOptions {
  category?: LogCategory;
  level?: LogLevel;
  minDebugLevel?: number;
}

function log(message: string, options: LogOptions = {}) {
  const minLevel = options.minDebugLevel ?? 1;
  if (!debugEnabled(minLevel)) return;
  
  const { category, level = "info" } = options;
  
  // Determine color and icon based on level
  let color = Color.cyan;
  let icon = "";
  
  switch (level) {
    case "error":
      color = Color.red;
      icon = Icon.error;
      break;
    case "warn":
      color = Color.yellow;
      icon = "⚠";
      break;
    case "success":
      color = Color.green;
      icon = Icon.success;
      break;
    case "metric":
      color = Color.magenta;
      icon = "📊";
      break;
    case "verbose":
      color = Color.dim;
      icon = "";
      break;
    default:
      color = Color.cyan;
      icon = "";
  }
  
  const timestamp = getTimestamp();
  const categoryTag = category ? `${Color.bold}[${category}]${Color.reset}` : "";
  const iconStr = icon ? ` ${icon}` : "";
  
  const prefix = `${Color.gray}[aic][debug]${Color.reset}${timestamp}${categoryTag}`;
  const formattedMsg = `${color}${message}${Color.reset}${iconStr}`;
  
  process.stderr.write(`${prefix} ${formattedMsg}\n`);
}

// Legacy compatibility function (now enhanced)
export function debugLog(...args: any[]) {
  const msg = args.map(formatValue).join(" ");
  log(msg, { level: "info" });
}

// Categorized logging functions
export function debugInfo(category: LogCategory, message: string, minLevel: number = 1) {
  log(message, { category, level: "info", minDebugLevel: minLevel });
}

export function debugWarn(category: LogCategory, message: string) {
  log(message, { category, level: "warn" });
}

export function debugError(category: LogCategory, message: string) {
  log(message, { category, level: "error" });
}

export function debugSuccess(category: LogCategory, message: string) {
  log(message, { category, level: "success" });
}

export function debugMetric(category: LogCategory, message: string, minLevel: number = 1) {
  log(message, { category, level: "metric", minDebugLevel: minLevel });
}

export function debugVerbose(category: LogCategory, message: string) {
  log(message, { category, level: "verbose", minDebugLevel: 2 });
}

// Utility for logging objects in a structured way
export function debugObject(category: LogCategory, label: string, obj: any, minLevel: number = 1) {
  if (!debugEnabled(minLevel)) return;
  
  debugInfo(category, `${label}:`, minLevel);
  const formatted = formatValue(obj);
  formatted.split("\n").forEach(line => {
    log(`  ${line}`, { category, level: "verbose", minDebugLevel: minLevel });
  });
}

// Utility for logging performance metrics
export function debugPerf(category: LogCategory, operation: string, durationMs: number) {
  debugMetric(category, `${operation} completed in ${durationMs.toFixed(2)}ms`);
}

// Utility for logging section headers
export function debugSection(title: string, minLevel: number = 1) {
  if (!debugEnabled(minLevel)) return;
  
  const line = "═".repeat(Math.min(60, title.length + 10));
  log(`${Color.bold}${line}${Color.reset}`, { level: "info", minDebugLevel: minLevel });
  log(`${Color.bold}${title}${Color.reset}`, { level: "info", minDebugLevel: minLevel });
  log(`${Color.bold}${line}${Color.reset}`, { level: "info", minDebugLevel: minLevel });
}
