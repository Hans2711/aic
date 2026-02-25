import { spawn } from "node:child_process";
import { Color } from "../ui/colors";
import { envBool, Env } from "../config";

function run(
  cmd: string,
  args: string[],
  opts?: { stdin?: string; stdio?: "pipe" | "inherit" }
): Promise<{ code: number; stdout: string; stderr: string }> {
  return new Promise((resolve) => {
    let child: ReturnType<typeof spawn> | undefined;
    try {
      const stdio = opts?.stdio || "pipe";
      child = spawn(cmd, args, { stdio });
    } catch (e: any) {
      const msg = e?.message || "spawn failed";
      return resolve({ code: 127, stdout: "", stderr: msg });
    }
    let out = "";
    let err = "";
    if (child.stdout) {
      child.stdout.setEncoding("utf8");
      child.stdout.on("data", (d) => (out += d));
    }
    if (child.stderr) {
      child.stderr.setEncoding("utf8");
      child.stderr.on("data", (d) => (err += d));
    }
    child.on("close", (code) => resolve({ code: code ?? 0, stdout: out, stderr: err }));
    if (opts?.stdin && child.stdin) {
      child.stdin.write(opts.stdin);
      child.stdin.end();
    }
  });
}

async function yesNoPrompt(question: string, defYes: boolean): Promise<boolean> {
  const defLabel = defYes
    ? `${Color.yellow}[Y|n]${Color.reset} ${Color.dim}[default: Y]${Color.reset}`
    : `${Color.yellow}[y|N]${Color.reset} ${Color.dim}[default: N]${Color.reset}`;
  process.stdout.write(`\n${Color.bold}${question}${Color.reset} ${defLabel}: ${Color.cyan}`);
  const ans = await readLine();
  process.stdout.write(Color.reset);
  const v = ans.trim().toLowerCase();
  if (!v) return defYes;
  return v === "y" || v === "yes";
}

function readLine(): Promise<string> {
  return new Promise((resolve) => {
    const stdin = process.stdin;
    // Ensure line-mode input: no raw mode, resumed, correct encoding
    stdin.setRawMode?.(false);
    stdin.setEncoding("utf8");
    stdin.resume();
    let buf = "";
    const onData = (chunk: string) => {
      if (chunk === "\u0003") {
        // Ctrl+C
        stdin.off("data", onData);
        stdin.pause();
        process.exit(130);
      }
      if (chunk === "\n" || chunk === "\r") {
        stdin.off("data", onData);
        stdin.pause();
        resolve(buf);
      } else {
        buf += chunk;
      }
    };
    stdin.on("data", onData);
  });
}

async function _readLineWithTimeout(ms: number, def: string): Promise<string> {
  return new Promise((resolve) => {
    let settled = false;
    const t = setTimeout(() => {
      if (!settled) {
        settled = true;
        try {
          process.stdin.pause();
        } catch {}
        resolve(def);
      }
    }, ms);
    void readLine().then((ans) => {
      if (!settled) {
        settled = true;
        clearTimeout(t);
        resolve(ans);
      }
    });
  });
}

async function git(...args: string[]) {
  const res = await run("git", args, { stdio: "pipe" });
  if (res.code !== 0) throw new Error(res.stderr || res.stdout || `git ${args.join(" ")} failed`);
  return res.stdout;
}

async function gitExec(...args: string[]) {
  const res = await run("git", args, { stdio: "inherit" });
  if (res.code !== 0) throw new Error(`git ${args.join(" ")} failed`);
  return;
}

export async function offerCommit(message: string): Promise<void> {
  // Non-interactive behavior
  if (envBool(Env.AIC_NON_INTERACTIVE)) {
    if (envBool(Env.AIC_AUTO_COMMIT)) {
      try {
        await git("commit", "-m", message);
      } catch (e: any) {
        const ok = await copyToClipboard(message);
        if (ok) {
          process.stdout.write(`${Color.yellow}Commit failed; message copied to clipboard.${Color.reset}\n`);
        } else {
          process.stdout.write(
            `${Color.yellow}Commit failed; could not copy to clipboard, please copy manually.${Color.reset}\n`
          );
        }
        throw e;
      }
    }
    return;
  }

  // Ask to commit
  const doCommit = await yesNoPrompt("Commit with this message now?", true);
  if (!doCommit) {
    const ok = await copyToClipboard(message);
    if (ok) {
      process.stdout.write(`${Color.green}Message copied to clipboard.${Color.reset}\n`);
    } else {
      process.stdout.write(`${Color.yellow}Could not copy to clipboard; please copy manually.${Color.reset}\n`);
    }
    return;
  }
  try {
    await gitExec("commit", "-m", message);
  } catch (e: any) {
    const ok = await copyToClipboard(message);
    if (ok) {
      process.stdout.write(`${Color.yellow}Commit failed; message copied to clipboard.${Color.reset}\n`);
    } else {
      process.stdout.write(
        `${Color.yellow}Commit failed; could not copy to clipboard, please copy manually.${Color.reset}\n`
      );
    }
    throw e;
  }

  // Ask to push
  const doPush = await yesNoPrompt("Push to current branch now?", true);
  if (doPush) {
    try {
      await pushCurrentBranch();
      // After push, just exit (no tag increment feature)
    } catch (e: any) {
      process.stdout.write(`${Color.yellow}Push failed:${Color.reset} ${e?.message || e}\n`);
    }
  }
}

async function pushCurrentBranch(): Promise<void> {
  const cur = (await git("rev-parse", "--abbrev-ref", "HEAD")).trim();
  if (!cur || cur === "HEAD") throw new Error("cannot determine current branch (detached HEAD)");
  // If upstream configured, a plain 'git push' should work
  try {
    await git("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}");
    await gitExec("push");
    return;
  } catch {}
  // No upstream: determine remote
  let remote = (await safeGit("config", "--get", `branch.${cur}.remote`)).trim();
  if (!remote) {
    const rems = (await safeGit("remote"))
      .split("\n")
      .map((s) => s.trim())
      .filter(Boolean);
    remote = rems.includes("origin") ? "origin" : rems[0] || "";
  }
  if (!remote) throw new Error("no Git remote configured");
  await gitExec("push", "-u", remote, cur);
}

async function safeGit(...args: string[]): Promise<string> {
  try {
    return await git(...args);
  } catch {
    return "";
  }
}

type TagInfo = { tag: string; vPrefix: boolean; major: number; minor: number; patch: number };

async function _latestSemverTag(): Promise<TagInfo | null> {
  const out = (await safeGit("tag", "-l", "--sort=-v:refname")).trim();
  if (!out) return null;
  const lines = out
    .split("\n")
    .map((s) => s.trim())
    .filter(Boolean);
  const rx = /^(v)?(\d+)\.(\d+)\.(\d+)$/;
  for (const line of lines) {
    const m = rx.exec(line);
    if (!m) continue;
    const vPref = !!m[1];
    const maj = parseInt(m[2], 10);
    const min = parseInt(m[3], 10);
    const pat = parseInt(m[4], 10);
    return { tag: line, vPrefix: vPref, major: maj, minor: min, patch: pat };
  }
  return null;
}

function _formatTag(vPrefix: boolean, major: number, minor: number, patch: number): string {
  return (vPrefix ? "v" : "") + `${major}.${minor}.${patch}`;
}

async function _buildTagMessage(fromTag: string): Promise<string> {
  const out = (await safeGit("log", `${fromTag}..HEAD`, "--pretty=format:%s")).trim();
  const lines = out
    ? out
        .split("\n")
        .map((s) => s.trim())
        .filter(Boolean)
    : [];
  const bullets = lines.map((l) => `- ${l}`).join("\n");
  return `Changes since ${fromTag}:\n${bullets}`;
}

async function _createTag(tag: string, message: string): Promise<void> {
  if (message && message.trim()) await gitExec("tag", "-a", tag, "-m", message);
  else await gitExec("tag", tag);
}

async function _pushTag(tag: string): Promise<void> {
  // Try to detect upstream remote of current branch
  const upstream = (await safeGit("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")).trim();
  if (upstream && upstream.includes("/")) {
    const remote = upstream.split("/", 2)[0];
    await gitExec("push", remote, tag);
    return;
  }
  // Fallback remote
  const cur = (await safeGit("rev-parse", "--abbrev-ref", "HEAD")).trim();
  let remote = "";
  if (cur && cur !== "HEAD") {
    remote = (await safeGit("config", "--get", `branch.${cur}.remote`)).trim();
  }
  if (!remote) {
    const rems = (await safeGit("remote"))
      .split("\n")
      .map((s) => s.trim())
      .filter(Boolean);
    remote = rems.includes("origin") ? "origin" : rems[0] || "";
  }
  if (remote) {
    await gitExec("push", remote, tag);
    return;
  }
  // Last resort
  await gitExec("push", "--tags");
}

async function copyToClipboard(msg: string): Promise<boolean> {
  // Try common clipboard tools across platforms
  const tools: Array<{ cmd: string; args?: string[] }> = [
    { cmd: "pbcopy" },
    { cmd: "wl-copy" },
    { cmd: "xclip", args: ["-selection", "clipboard"] },
    { cmd: "clip" },
  ];
  for (const t of tools) {
    const res = await run(t.cmd, t.args || [], { stdin: msg, stdio: "pipe" });
    if (res.code === 0) return true;
  }
  return false;
}
