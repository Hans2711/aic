import { Color } from "./colors";

export type SelectOptions = {
  title?: string;
  items: string[];
};

export async function selectInteractive(opts: SelectOptions): Promise<string> {
  const items = opts.items.slice(0, 10);
  if (items.length === 0) throw new Error("no suggestions to select");

  if (!process.stdin.isTTY) {
    // Fallback simple selection via stdin prompt
    process.stdout.write(`${Color.gray}${Color.bold} ${opts.title ?? "Suggestions"}:${Color.reset}\n`);
    for (let i = 0; i < items.length; i++) {
      process.stdout.write(`  ${Color.yellow}[${i + 1}]${Color.reset} ${Color.cyan}${items[i]}${Color.reset}\n`);
    }
    process.stdout.write(`\n${Color.bold}Select [1-${items.length}]${Color.reset} ${Color.dim}[default: 1]${Color.reset}: ${Color.cyan}`);
    const choice = await readLine();
    const n = choice.trim() === "" ? 1 : Math.min(Math.max(parseInt(choice, 10) || 1, 1), items.length);
    process.stdout.write(Color.reset);
    return items[n - 1];
  }

  return await rawModeSelect(opts.title ?? "Commit message suggestions", items);
}

async function rawModeSelect(title: string, items: string[]): Promise<string> {
  const stdin = process.stdin;
  stdin.setRawMode?.(true);
  stdin.resume();
  stdin.setEncoding("utf8");
  let selected = 0;
  const n = items.length;
  const cols = getCols();

  const truncated = (s: string) => {
    const max = Math.max(10, cols - 10); // leave room for prefix and colors
    const arr = [...s];
    if (arr.length <= max) return s;
    return arr.slice(0, Math.max(0, max - 4)).join("") + "....";
  };

  const render = () => {
    process.stdout.write(`\r\x1b[2K${Color.gray}${Color.bold} ${title}:${Color.reset}\n`);
    let printed = 1; // header
    for (let i = 0; i < n; i++) {
      const idx = i === 9 ? 0 : i + 1;
      const prefix = i === selected ? `${Color.yellow}> ${Color.reset}` : "  ";
      const color = i === selected ? Color.green + Color.bold : Color.cyan;
      const lines = splitLines(items[i]);
      if (lines.length === 0) {
        process.stdout.write(`${prefix}[${idx}] ${color}${Color.reset}\n`);
        printed++;
        continue;
      }
      // first line with index
      process.stdout.write(`${prefix}[${idx}] ${color}${truncated(lines[0])}${Color.reset}\n`);
      printed++;
      // remaining lines indented
      const indent = "    ";
      for (let j = 1; j < lines.length; j++) {
        process.stdout.write(`${indent}${color}${truncated(lines[j])}${Color.reset}\n`);
        printed++;
      }
    }
    process.stdout.write(`${Color.dim}Use ↑/↓ or j/k, numbers to pick, Enter to confirm.${Color.reset}\n`);
    printed++;
    return printed;
  };
  let backLines = render();

  const moveUp = (lines: number) => {
    if (lines > 0) process.stdout.write(`\x1b[${lines}A`);
  };
  const clearLine = () => process.stdout.write("\x1b[2K\r");

  const cleanup = () => {
    stdin.setRawMode?.(false);
    stdin.pause();
  };

  return await new Promise<string>((resolve) => {
    const onData = (chunk: string) => {
      const b = chunk;
      if (!b) return;
      switch (b) {
        case "\u0003": // Ctrl+C
          cleanup();
          stdin.off("data", onData);
          process.exit(130);
          return;
        case "\r":
        case "\n":
          cleanup();
          stdin.off("data", onData);
          resolve(items[selected]);
          return;
        case "k":
          if (selected > 0) selected--;
          break;
        case "j":
          if (selected < n - 1) selected++;
          break;
        default:
          if (b === "\u001b[A") {
            if (selected > 0) selected--;
          } else if (b === "\u001b[B") {
            if (selected < n - 1) selected++;
          } else if (b >= "1" && b <= "9") {
            const v = b.charCodeAt(0) - "0".charCodeAt(0);
            if (v >= 1 && v <= n) {
              cleanup();
              stdin.off("data", onData);
              resolve(items[v - 1]);
              return;
            }
          } else if (b === "0" && n === 10) {
            cleanup();
            stdin.off("data", onData);
            resolve(items[9]);
            return;
          }
      }
      // Re-render in place
      moveUp(backLines);
      for (let i = 0; i < backLines; i++) { clearLine(); if (i < backLines - 1) process.stdout.write("\n"); }
      moveUp(backLines - 1);
      backLines = render();
    };
    stdin.on("data", onData);
  });
}

function readLine(): Promise<string> {
  return new Promise((resolve) => {
    let buf = "";
    const onData = (chunk: string) => {
      if (chunk === "\n" || chunk === "\r") {
        process.stdin.off("data", onData);
        resolve(buf);
      } else {
        buf += chunk;
      }
    };
    process.stdin.on("data", onData);
  });
}

function getCols(): number {
  const c = (process.stdout as any).columns as number | undefined;
  if (typeof c === "number" && c > 20) return c;
  const env = Number.parseInt(process.env.COLUMNS || "", 10);
  return Number.isFinite(env) && env > 20 ? env : 80;
}

function splitLines(s: string): string[] {
  // Preserve author-intended lines, and prevent soft wraps by truncating later
  return s.replace(/\r\n?/g, "\n").split("\n");
}
