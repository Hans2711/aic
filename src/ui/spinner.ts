import { Color, Icon } from "./colors";
import { debugEnabled } from "../debug";

export function spinner(message: string) {
  // In debug mode, avoid animation so logs remain readable
  if (debugEnabled(1)) {
    // Print a static preface line to stderr for context
    process.stderr.write(`${Color.dim}${message}${Color.reset}\n`);
    return (ok: boolean) => {
      const sym = ok ? Icon.success : Icon.error;
      const col = ok ? Color.green : Color.red;
      process.stderr.write(`${col}${sym}${Color.reset} ${Color.bold}${message}${Color.reset}\n`);
    };
  }
  const frames = ["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"];
  let i = 0;
  let active = true;
  const id = setInterval(() => {
    if (!active) return;
    process.stderr.write(`\r${frames[i % frames.length]} ${Color.dim}${message}${Color.reset}`);
    i++;
  }, 90);
  return (ok: boolean) => {
    active = false;
    clearInterval(id);
    const sym = ok ? Icon.success : Icon.error;
    const col = ok ? Color.green : Color.red;
    process.stderr.write(`\r${col}${sym}${Color.reset} ${Color.bold}${message}${Color.reset}\n`);
  };
}
