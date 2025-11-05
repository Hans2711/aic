import { Color } from "./colors";
import { spinner } from "./spinner";

export type CombineSelectOptions = {
  title?: string;
  items: string[];
  onCombine: (selected: string[]) => Promise<{ suggestions: string[]; modelName?: string }>;
};

export async function selectWithCombine(opts: CombineSelectOptions): Promise<string> {
  const cols = getCols();
  const truncate = (s: string) => {
    const max = Math.max(10, cols - 12);
    const arr = [...s];
    return arr.length <= max ? s : arr.slice(0, Math.max(0, max - 4)).join("") + "....";
  };

  let items = opts.items.slice(0, 10);
  if (items.length === 0) throw new Error("no suggestions to select");

  if (!process.stdin.isTTY) {
    // Non-TTY fallback (no combine in this mode)
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

  const stdin = process.stdin;
  stdin.setRawMode?.(true);
  stdin.resume();
  stdin.setEncoding("utf8");

  let selected = 0;
  const checked: boolean[] = new Array(items.length).fill(false);
  const n = () => items.length;
  const header = () => `${Color.gray}${Color.bold} ${opts.title ?? "Commit message suggestions"}:${Color.reset}\n`;
  const instruction = () => `${Color.dim}Use ↑/↓ or j/k, Space to toggle select, numbers to pick, Enter to confirm.${Color.reset}\n`;

  const render = () => {
    process.stdout.write(`\r\x1b[2K` + header());
    let printed = 1; // header
    for (let i = 0; i < n(); i++) {
      const idx = i === 9 ? 0 : i + 1;
      const prefix = i === selected ? `${Color.yellow}> ${Color.reset}` : "  ";
      const color = i === selected ? Color.green + Color.bold : Color.cyan;
      const box = checked[i] ? "[x]" : "[ ]";
      const lines = splitLines(items[i]);
      if (lines.length === 0) {
        process.stdout.write(`${prefix}[${idx}] ${box} ${color}${Color.reset}\n`);
        printed++;
        continue;
      }
      process.stdout.write(`${prefix}[${idx}] ${box} ${color}${truncate(lines[0])}${Color.reset}\n`);
      printed++;
      const indent = "       "; // align under the start of text
      for (let j = 1; j < lines.length; j++) {
        process.stdout.write(`${indent}${color}${truncate(lines[j])}${Color.reset}\n`);
        printed++;
      }
    }
    process.stdout.write(instruction());
    printed++;
    return printed;
  };

  let backLines = render();
  const moveUp = (lines: number) => { if (lines > 0) process.stdout.write(`\x1b[${lines}A`); };
  const clearLine = () => process.stdout.write("\x1b[2K\r");
  const countChecked = () => checked.filter(Boolean).length;
  const cleanup = () => { stdin.setRawMode?.(false); stdin.pause(); };

  return await new Promise<string>((resolve) => {
    const onData = async (chunk: string) => {
      const b = chunk;
      if (!b) return;
      switch (b) {
        case "\u0003": // Ctrl+C
          cleanup();
          stdin.off("data", onData);
          process.exit(130);
          return;
        case " ": // Space toggle
          if (n()) checked[selected] = !checked[selected];
          break;
        case "k":
          if (selected > 0) selected--;
          break;
        case "j":
          if (selected < n() - 1) selected++;
          break;
        case "\r":
        case "\n": {
          const cnt = countChecked();
          if (cnt >= 2) {
            // Combine pathway
            const chosen: string[] = [];
            for (let i = 0; i < n(); i++) if (checked[i]) chosen.push(items[i]);
            cleanup();
            stdin.off("data", onData);
            const stop = spinner(`Combining ${chosen.length} selected messages`);
            try {
              const res = await opts.onCombine(chosen);
              stop(true);
              // Re-enter raw mode for new list
              stdin.setRawMode?.(true);
              stdin.resume();
              stdin.setEncoding("utf8");
              // Replace items and reset state
              items = res.suggestions.slice(0, 10);
              selected = 0;
              for (let i = 0; i < checked.length; i++) checked[i] = false;
              for (let i = checked.length; i < items.length; i++) checked[i] = false;
              // Re-render with new header
              process.stdout.write(`\n${Color.gray}${Color.bold} Combined suggestions:${Color.reset}\n`);
              render();
              backLines = n() + 2;
              // Reattach listener
              stdin.on("data", onData);
            } catch (e) {
              stop(false);
              // Re-enter raw mode and keep the current list so the user can try again
              stdin.setRawMode?.(true);
              stdin.resume();
              stdin.setEncoding("utf8");
              process.stderr.write(`${Color.red}Combine failed; keeping current suggestions. Try again or select a single item.${Color.reset}\n`);
              render();
              backLines = n() + 2;
              stdin.on("data", onData);
            }
            return;
          }
          // If exactly one is checked, return it; otherwise return current selection
          if (cnt === 1) {
            for (let i = 0; i < n(); i++) if (checked[i]) { cleanup(); stdin.off("data", onData); resolve(items[i]); return; }
          }
          // No boxes checked: accept the currently highlighted suggestion
          cleanup();
          stdin.off("data", onData);
          resolve(items[Math.min(selected, n() - 1)]);
          return;
        }
        default:
          if (b === "\u001b[A") { if (selected > 0) selected--; }
          else if (b === "\u001b[B") { if (selected < n() - 1) selected++; }
          else if (b >= "1" && b <= "9") {
            const v = b.charCodeAt(0) - "0".charCodeAt(0);
            if (v >= 1 && v <= n()) { cleanup(); stdin.off("data", onData); resolve(items[v - 1]); return; }
          } else if (b === "0" && n() === 10) { cleanup(); stdin.off("data", onData); resolve(items[9]); return; }
      }
      // Re-render with accurate line count
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
      if (chunk === "\n" || chunk === "\r") { process.stdin.off("data", onData); resolve(buf); }
      else { buf += chunk; }
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
  return s.replace(/\r\n?/g, "\n").split("\n");
}
