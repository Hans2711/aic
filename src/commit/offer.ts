import { spawn } from "node:child_process";
import { Color } from "../ui/colors";
import { selectInteractive } from "../ui/select";
import { envBool, Env } from "../config";

function run(cmd: string, args: string[], opts?: { stdin?: string; stdio?: "pipe" | "inherit" }): Promise<{ code: number; stdout: string; stderr: string }> {
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
    if (child.stdout) { child.stdout.setEncoding("utf8"); child.stdout.on("data", (d) => (out += d)); }
    if (child.stderr) { child.stderr.setEncoding("utf8"); child.stderr.on("data", (d) => (err += d)); }
    child.on("close", (code) => resolve({ code: code ?? 0, stdout: out, stderr: err }));
    if (opts?.stdin && child.stdin) { child.stdin.write(opts.stdin); child.stdin.end(); }
  });
}

async function yesNoPrompt(question: string, defYes: boolean): Promise<boolean> {
  const defLabel = defYes ? `${Color.yellow}[Y|n]${Color.reset} ${Color.dim}[default: Y]${Color.reset}` : `${Color.yellow}[y|N]${Color.reset} ${Color.dim}[default: N]${Color.reset}`;
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
      if (chunk === "\u0003") { // Ctrl+C
        stdin.off("data", onData);
        stdin.pause();
        process.exit(130);
      }
      if (chunk === "\n" || chunk === "\r") { stdin.off("data", onData); stdin.pause(); resolve(buf); }
      else { buf += chunk; }
    };
    stdin.on("data", onData);
  });
}

async function readLineWithTimeout(ms: number, def: string): Promise<string> {
  return new Promise(async (resolve) => {
    let settled = false;
    const t = setTimeout(() => {
      if (!settled) {
        settled = true;
        try { process.stdin.pause(); } catch {}
        resolve(def);
      }
    }, ms);
    const ans = await readLine();
    if (!settled) {
      settled = true;
      clearTimeout(t);
      resolve(ans);
    }
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
      await git("commit", "-m", message);
    }
    return;
  }

  // Ask to commit
  const doCommit = await yesNoPrompt("Commit with this message now?", true);
  if (!doCommit) return;
  await gitExec("commit", "-m", message);

  // Ask to push
  const doPush = await yesNoPrompt("Push to current branch now?", true);
  if (doPush) {
    try {
      await pushCurrentBranch();
      // Ask about tagging
      const doTag = await yesNoPrompt("Increment latest tag?", false);
      if (doTag) {
        const info = await latestSemverTag();
        if (!info) {
          const create = await yesNoPrompt("No existing semver-like tag found. Create initial tag?", false);
          if (!create) return;
          // 'v' prefix prompt
          process.stdout.write(`${Color.bold}Use 'v' prefix?${Color.reset} ${Color.yellow}[Y|n]${Color.reset} ${Color.dim}[default: Y]${Color.reset}: ${Color.cyan}`);
          const pref = (await readLineWithTimeout(30000, "")).trim().toLowerCase();
          process.stdout.write(Color.reset);
          const vPref = !(pref === "n" || pref === "no");
          // initial version prompt
          process.stdout.write(`${Color.bold}Initial version${Color.reset} ${Color.dim}[default: 0.1.0]${Color.reset}: ${Color.cyan}`);
          let ver = (await readLineWithTimeout(30000, "")).trim();
          process.stdout.write(Color.reset);
          if (!/^\d+\.\d+\.\d+$/.test(ver)) ver = "0.1.0";
          const [maj, min, pat] = ver.split(".").map((n) => parseInt(n, 10));
          const initTag = formatTag(vPref, maj, min, pat);
          const msg = await buildTagMessage("HEAD");
          await createTag(initTag, msg);
          await pushTag(initTag);
          process.stdout.write(`${Color.green}Pushed tag:${Color.reset} ${initTag}\n`);
          return;
        }
        const { tag, vPrefix, major, minor, patch } = info;
        const majTag = formatTag(vPrefix, major + 1, 0, 0);
        const minTag = formatTag(vPrefix, major, minor + 1, 0);
        const patTag = formatTag(vPrefix, major, minor, patch + 1);
        // Simple numeric prompt to avoid raw-mode TTY issues
        process.stdout.write(`${Color.gray}${Color.bold} Select version bump:${Color.reset}\n`);
        process.stdout.write(`  ${Color.yellow}[1]${Color.reset} Major -> ${majTag} (from ${tag})\n`);
        process.stdout.write(`  ${Color.yellow}[2]${Color.reset} Minor -> ${minTag} (from ${tag})\n`);
        process.stdout.write(`  ${Color.yellow}[3]${Color.reset} Patch -> ${patTag} (from ${tag})\n`);
        process.stdout.write(`\n${Color.bold}Choose [1-3]${Color.reset} ${Color.dim}[default: 3]${Color.reset}: ${Color.cyan}`);
        const choice = (await readLineWithTimeout(60000, "")).trim();
        let newTag = patTag;
        if (choice === "1") newTag = majTag;
        else if (choice === "2") newTag = minTag;
        else if (choice === "3" || choice === "") newTag = patTag;
        process.stdout.write(Color.reset);
        if (!newTag) return;
        const tagMsg = await buildTagMessage(tag);
        await createTag(newTag, tagMsg);
        await pushTag(newTag);
        process.stdout.write(`${Color.green}Pushed tag:${Color.reset} ${newTag}\n`);
      }
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
    const rems = (await safeGit("remote")).split("\n").map((s) => s.trim()).filter(Boolean);
    remote = rems.includes("origin") ? "origin" : (rems[0] || "");
  }
  if (!remote) throw new Error("no Git remote configured");
  await gitExec("push", "-u", remote, cur);
}

async function safeGit(...args: string[]): Promise<string> {
  try { return await git(...args); } catch { return ""; }
}

type TagInfo = { tag: string; vPrefix: boolean; major: number; minor: number; patch: number };

async function latestSemverTag(): Promise<TagInfo | null> {
  const out = (await safeGit("tag", "-l", "--sort=-v:refname")).trim();
  if (!out) return null;
  const lines = out.split("\n").map((s) => s.trim()).filter(Boolean);
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

function formatTag(vPrefix: boolean, major: number, minor: number, patch: number): string {
  return (vPrefix ? "v" : "") + `${major}.${minor}.${patch}`;
}

async function buildTagMessage(fromTag: string): Promise<string> {
  const out = (await safeGit("log", `${fromTag}..HEAD`, "--pretty=format:%s")).trim();
  const lines = out ? out.split("\n").map((s) => s.trim()).filter(Boolean) : [];
  const bullets = lines.map((l) => `- ${l}`).join("\n");
  return `Changes since ${fromTag}:\n${bullets}`;
}

async function createTag(tag: string, message: string): Promise<void> {
  if (message && message.trim()) await gitExec("tag", "-a", tag, "-m", message);
  else await gitExec("tag", tag);
}

async function pushTag(tag: string): Promise<void> {
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
    const rems = (await safeGit("remote")).split("\n").map((s) => s.trim()).filter(Boolean);
    remote = rems.includes("origin") ? "origin" : (rems[0] || "");
  }
  if (remote) { await gitExec("push", remote, tag); return; }
  // Last resort
  await gitExec("push", "--tags");
}
