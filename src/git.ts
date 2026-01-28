import { spawn } from "node:child_process";
import { debugInfo } from "./debug";
import { DISPLAY_LIMITS } from "./constants";

function run(
  cmd: string,
  args: string[],
  opts?: { stdin?: string }
): Promise<{ stdout: string; stderr: string; code: number }> {
  return new Promise((resolve) => {
    let child: ReturnType<typeof spawn> | undefined;
    try {
      child = spawn(cmd, args, { stdio: ["pipe", "pipe", "pipe"] });
    } catch (e: any) {
      // Gracefully handle missing executables (e.g., gh not installed)
      const msg = e?.message || "spawn failed";
      resolve({ stdout: "", stderr: msg, code: 127 });
      return;
    }
    let out = "";
    let err = "";
    child.stdout!.setEncoding("utf8");
    child.stderr!.setEncoding("utf8");
    child.stdout!.on("data", (d) => (out += d));
    child.stderr!.on("data", (d) => (err += d));
    child.on("error", (e) => {
      const msg = e?.message || "spawn error";
      resolve({ stdout: out, stderr: msg, code: 127 });
    });
    child.on("close", (code) => resolve({ stdout: out, stderr: err, code: code ?? 0 }));
    if (opts?.stdin) {
      child.stdin!.write(opts.stdin);
    }
    child.stdin!.end();
  });
}

async function git(...args: string[]): Promise<{ stdout: string; stderr: string; code: number }> {
  return run("git", args);
}

export async function insideRepo(): Promise<boolean> {
  const { stdout, code } = await git("rev-parse", "--is-inside-work-tree");
  return code === 0 && stdout.trim() === "true";
}

export async function stagedDiff(): Promise<string> {
  if (!(await insideRepo())) {
    throw new Error("not a git repository (run 'git init')");
  }
  const variants: string[][] = [
    ["diff", "--cached", "--minimal", "--unified=0", "--no-prefix", "--color=never"],
    ["diff", "--staged", "--minimal", "--unified=0", "--no-prefix", "--color=never"],
    ["diff", "--minimal", "--unified=0", "--no-prefix", "--color=never"],
  ];
  let lastErr = "";
  for (let i = 0; i < variants.length; i++) {
    const args = variants[i];
    const res = await git(...args);
    if (res.code === 0) return res.stdout;
    // allow unknown option on first two variants
    if (i < 2 && (res.stderr.includes("unknown option") || res.stdout.includes("unknown option"))) {
      lastErr = res.stderr || res.stdout;
      continue;
    }
    throw new Error(`git diff failed (${args.join(" ")}) ${res.code}: ${res.stderr || res.stdout}`);
  }
  if (lastErr) throw new Error(lastErr);
  throw new Error("failed to obtain git diff");
}

export async function worktreeDiff(): Promise<string> {
  if (!(await insideRepo())) {
    throw new Error("not a git repository (run 'git init')");
  }
  // Prefer a single comparison against HEAD which includes staged + unstaged
  {
    const res = await git("diff", "--minimal", "--unified=0", "--no-prefix", "--color=never", "HEAD");
    if (res.code === 0) return res.stdout;
    // If fails (e.g., initial commit), fall through
  }
  // Combine staged + unstaged
  let combined = "";
  try {
    const s = await stagedDiff();
    if (s.trim()) combined += s.endsWith("\n") ? s : s + "\n";
  } catch {
    // ignore
  }
  {
    const res = await git("diff", "--minimal", "--unified=0", "--no-prefix", "--color=never");
    if (res.code === 0 && res.stdout.trim()) {
      combined += res.stdout.endsWith("\n") ? res.stdout : res.stdout + "\n";
    }
  }
  if (!combined) throw new Error("failed to obtain git worktree diff");
  return combined;
}

export async function stagedFiles(): Promise<string[]> {
  if (!(await insideRepo())) return [];
  const variants: string[][] = [
    ["diff", "--name-only", "--cached"],
    ["diff", "--name-only", "--staged"],
    ["diff", "--name-only"],
  ];
  for (let i = 0; i < variants.length; i++) {
    const args = variants[i];
    const res = await git(...args);
    if (res.code === 0) {
      return res.stdout
        .split("\n")
        .map((s) => s.trim())
        .filter(Boolean);
    }
    if (i < 2 && (res.stderr.includes("unknown option") || res.stdout.includes("unknown option"))) {
      continue;
    }
    throw new Error(`git diff --name-only failed (${args.join(" ")}) ${res.code}: ${res.stderr || res.stdout}`);
  }
  return [];
}

export async function currentBranch(): Promise<string> {
  const res = await git("rev-parse", "--abbrev-ref", "HEAD");
  if (res.code !== 0) return "";
  return res.stdout.trim();
}

type GhPrView = {
  title?: string;
  closingIssuesReferences?: Array<{ number: number }>; // when available
};

type GhIssueView = { title?: string; body?: string };

async function gh(...args: string[]): Promise<{ stdout: string; stderr: string; code: number }> {
  return run("gh", args);
}

async function prInfo(): Promise<{ title: string; issues: string[] }> {
  const res = await gh("pr", "view", "--json", "title,closingIssuesReferences");
  if (res.code !== 0) {
    if (res.code === 127) debugInfo("GIT", "gh not found; skipping PR context");
    return { title: "", issues: [] };
  }
  try {
    const data: GhPrView = JSON.parse(res.stdout);
    const title = (data.title || "").trim();
    const issues: string[] = [];
    if (Array.isArray(data.closingIssuesReferences)) {
      for (const ref of data.closingIssuesReferences) {
        const num = ref?.number ?? 0;
        if (num > 0) {
          const is = await issueText(num);
          if (is) issues.push(is);
        }
      }
    }
    return { title, issues };
  } catch {
    return { title: res.stdout.trim(), issues: [] };
  }
}

async function issueText(num: number): Promise<string> {
  const res = await gh("issue", "view", String(num), "--json", "title,body");
  if (res.code !== 0) return "";
  try {
    const data: GhIssueView = JSON.parse(res.stdout);
    const title = (data.title || "").trim();
    let body = (data.body || "").trim();
    const r = [...body];
    if (r.length > DISPLAY_LIMITS.MAX_ISSUE_BODY_CHARS) body = r.slice(0, DISPLAY_LIMITS.MAX_ISSUE_BODY_CHARS).join("");
    return (title + ": " + body).trim();
  } catch {
    return "";
  }
}

function formatContext(branch: string, prTitle: string, issues: string[]): string {
  const parts: string[] = [];
  if (branch.trim()) parts.push("Branch: " + branch.trim());
  if (prTitle.trim()) parts.push("PR Title: " + prTitle.trim());
  for (const is of issues) {
    const s = is.trim();
    if (s) parts.push("Issue: " + s);
  }
  return parts.length ? parts.join("\n") : "";
}

export async function repoContext(): Promise<string> {
  const branch = await currentBranch();
  const { title, issues } = await prInfo();
  return formatContext(branch, title, issues);
}
