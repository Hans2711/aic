/**
 * Table formatting utilities for help text
 */

/**
 * Get terminal width from environment or use default
 */
function getTerminalWidth(): number {
  // Try process.stdout.columns first (most reliable)
  if (process.stdout.columns && process.stdout.columns > 0) {
    return process.stdout.columns;
  }

  // Fall back to COLUMNS env var
  const cols = process.env.COLUMNS;
  if (cols) {
    const n = Number.parseInt(cols, 10);
    if (Number.isFinite(n) && n > 0) return n;
  }

  // Default to a reasonable width that works for most terminals
  return 100;
}

/**
 * Word-wrap text to fit within a given width
 */
function wrapText(text: string, width: number): string[] {
  const lines: string[] = [];
  let currentLine = "";
  const words = text.split(/\s+/);

  for (const word of words) {
    if (!word) continue;

    // If adding this word would exceed width, start new line
    if (currentLine && currentLine.length + 1 + word.length > width) {
      lines.push(currentLine);
      currentLine = word;
    } else {
      currentLine = currentLine ? `${currentLine} ${word}` : word;
    }
  }

  if (currentLine) {
    lines.push(currentLine);
  }

  return lines.length > 0 ? lines : [""];
}

/**
 * Pad string to specified width
 */
function padRight(text: string, width: number): string {
  if (text.length >= width) return text;
  return text + " ".repeat(width - text.length);
}

/**
 * Format a 2-column table with word wrapping
 *
 * @param rows - Array of [key, value] pairs
 * @param options - Formatting options
 * @returns Formatted table as string
 */
export function formatTable(
  rows: Array<[string, string]>,
  options: {
    header?: [string, string];
    indent?: string;
    minKeyWidth?: number;
    maxKeyWidth?: number;
    gutter?: number;
  } = {}
): string {
  const { header, indent = "  ", minKeyWidth = 20, maxKeyWidth = 35, gutter = 3 } = options;

  const termWidth = getTerminalWidth();

  // Calculate actual column widths
  const maxKeyLen = rows.reduce((max, [key]) => Math.max(max, key.length), 0);
  const keyWidth = Math.max(minKeyWidth, Math.min(maxKeyWidth, maxKeyLen));

  // Available width for description column
  const descWidth = termWidth - indent.length - keyWidth - gutter;

  const lines: string[] = [];

  // Add header if provided
  if (header) {
    const [headerKey, headerDesc] = header;
    lines.push(indent + padRight(headerKey, keyWidth) + " ".repeat(gutter) + headerDesc);
    lines.push(indent + "─".repeat(keyWidth) + " ".repeat(gutter) + "─".repeat(descWidth));
  }

  // Format each row
  for (const [key, desc] of rows) {
    // Wrap description text
    const descLines = wrapText(desc, descWidth);

    // First line: key + first line of description
    lines.push(indent + padRight(key, keyWidth) + " ".repeat(gutter) + descLines[0]);

    // Continuation lines: blank key column + wrapped description
    for (let i = 1; i < descLines.length; i++) {
      lines.push(indent + " ".repeat(keyWidth) + " ".repeat(gutter) + descLines[i]);
    }
  }

  return lines.join("\n");
}

/**
 * Format environment variables help section for use in printHelp()
 * Uses table format with proper word wrapping
 */
export function formatEnvVarsTable(coreRows: Array<[string, string]>, customRows: Array<[string, string]>): string {
  const parts: string[] = [];

  parts.push("Environment variables:");
  parts.push("");

  // Core variables
  parts.push(
    formatTable(coreRows, {
      header: ["Variable Name", "Description"],
      indent: "  ",
    })
  );

  // Custom provider variables (if any)
  if (customRows.length > 0) {
    parts.push("");
    parts.push(
      formatTable(customRows, {
        indent: "  ",
      })
    );
  }

  return parts.join("\n");
}

/**
 * Format environment variables help section for Clipanion (simple list format)
 * Uses simple list to avoid Clipanion's text re-wrapping
 */
export function formatEnvVarsHelp(coreRows: Array<[string, string]>, customRows: Array<[string, string]>): string {
  const parts: string[] = [];

  parts.push("Environment variables:");
  parts.push("");

  // Format as a simple list to avoid Clipanion's text wrapping issues
  for (const [key, desc] of coreRows) {
    parts.push(`  ${key}`);
    parts.push(`    ${desc}`);
    parts.push("");
  }

  // Custom provider variables (if any)
  if (customRows.length > 0) {
    for (const [key, desc] of customRows) {
      parts.push(`  ${key}`);
      parts.push(`    ${desc}`);
      parts.push("");
    }
  }

  return parts.join("\n");
}
