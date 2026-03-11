import { spawn } from "node:child_process";
import { debugInfo } from "./debug";
import { DISPLAY_LIMITS } from "./constants";

export type RepoHostingProvider = "github" | "gitlab" | "unknown";

export type RepoHostInfo = {
  provider: RepoHostingProvider;
  remote: string;
  url: string;
  hostname: string;
};

function errorMessage(error: unknown): string {
  if (error instanceof Error && error.message) return error.message;
  return String(error);
}

function run(
  cmd: string,
  args: string[],
  opts?: { stdin?: string }
): Promise<{ stdout: string; stderr: string; code: number }> {
  return new Promise((resolve) => {
    let child: ReturnType<typeof spawn> | undefined;
    try {
      child = spawn(cmd, args, { stdio: ["pipe", "pipe", "pipe"] });
    } catch (error: unknown) {
      // Gracefully handle missing executables (e.g., gh not installed)
      const msg = errorMessage(error) || "spawn failed";
      resolve({ stdout: "", stderr: msg, code: 127 });
      return;
    }
    let out = "";
    let err = "";
    child.stdout!.setEncoding("utf8");
    child.stderr!.setEncoding("utf8");
    child.stdout!.on("data", (d) => (out += d));
    child.stderr!.on("data", (d) => (err += d));
    child.on("error", (error: unknown) => {
      const msg = errorMessage(error) || "spawn error";
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

async function safeGit(...args: string[]): Promise<string> {
  const res = await git(...args);
  if (res.code !== 0) return "";
  return res.stdout;
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

export async function currentRemote(): Promise<string> {
  const branch = await currentBranch();
  if (branch && branch !== "HEAD") {
    const configured = (await safeGit("config", "--get", `branch.${branch}.remote`)).trim();
    if (configured) return configured;
  }
  const remotes = (await safeGit("remote"))
    .split("\n")
    .map((s) => s.trim())
    .filter(Boolean);
  if (remotes.includes("origin")) return "origin";
  return remotes[0] || "";
}

export async function remoteUrl(remote?: string): Promise<string> {
  const name = remote?.trim() || (await currentRemote());
  if (!name) return "";
  return (await safeGit("remote", "get-url", name)).trim();
}

function parseRemoteHostname(url: string): string {
  const trimmed = url.trim();
  if (!trimmed) return "";
  const scpLike = /^[^@]+@([^:]+):/.exec(trimmed);
  if (scpLike?.[1]) return scpLike[1].toLowerCase();
  try {
    return new URL(trimmed).hostname.toLowerCase();
  } catch {
    const hostMatch = /^(?:ssh:\/\/)?(?:[^@]+@)?([^/:]+)(?::\d+)?[:/]/.exec(trimmed);
    return hostMatch?.[1]?.toLowerCase() || "";
  }
}

function providerFromHostname(hostname: string): RepoHostingProvider {
  if (!hostname) return "unknown";
  if (hostname === "github.com" || hostname.endsWith(".github.com")) return "github";
  if (hostname === "gitlab.com" || hostname.endsWith(".gitlab.com")) return "gitlab";
  if (hostname.includes("github")) return "github";
  if (hostname.includes("gitlab")) return "gitlab";
  return "unknown";
}

export async function repoHostInfo(): Promise<RepoHostInfo> {
  const remote = await currentRemote();
  const url = await remoteUrl(remote);
  const hostname = parseRemoteHostname(url);
  return {
    provider: providerFromHostname(hostname),
    remote,
    url,
    hostname,
  };
}

type GhPrView = {
  title?: string;
  closingIssuesReferences?: Array<{ number: number }>; // when available
};

type GhIssueView = { title?: string; body?: string };
type GlabMrView = { title?: string; description?: string; references?: { full?: string } };

async function gh(...args: string[]): Promise<{ stdout: string; stderr: string; code: number }> {
  return run("gh", args);
}

async function glab(...args: string[]): Promise<{ stdout: string; stderr: string; code: number }> {
  return run("glab", args);
}

async function githubPrInfo(): Promise<{ title: string; issues: string[] }> {
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
          const is = await githubIssueText(num);
          if (is) issues.push(is);
        }
      }
    }
    return { title, issues };
  } catch {
    return { title: res.stdout.trim(), issues: [] };
  }
}

async function githubIssueText(num: number): Promise<string> {
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

async function gitlabMrInfo(branch: string): Promise<{ title: string; issues: string[] }> {
  const ref = branch.trim() || undefined;
  const args = ref ? ["mr", "view", ref, "--output", "json"] : ["mr", "view", "--output", "json"];
  const res = await glab(...args);
  if (res.code !== 0) {
    if (res.code === 127) debugInfo("GIT", "glab not found; skipping MR context");
    return { title: "", issues: [] };
  }
  try {
    const data: GlabMrView = JSON.parse(res.stdout);
    const title = (data.title || "").trim();
    const issues = extractGitLabIssueRefs(data.description || "");
    return { title, issues };
  } catch {
    return { title: "", issues: [] };
  }
}

function extractGitLabIssueRefs(description: string): string[] {
  const refs = new Set<string>();
  const regex = /(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s+([#&][\w-]+(?:\/[\w.-]+)?|\S*#\d+)/gi;
  for (const match of description.matchAll(regex)) {
    const raw = (match[1] || "").trim();
    if (!raw) continue;
    refs.add(raw.replace(/[),.;]+$/, ""));
  }
  return Array.from(refs);
}

function formatContext(branch: string, reviewLabel: string, title: string, issues: string[]): string {
  const parts: string[] = [];
  if (branch.trim()) parts.push("Branch: " + branch.trim());
  if (title.trim()) parts.push(`${reviewLabel}: ` + title.trim());
  for (const is of issues) {
    const s = is.trim();
    if (s) parts.push("Issue: " + s);
  }
  return parts.length ? parts.join("\n") : "";
}

export async function repoContext(): Promise<string> {
  const branch = await currentBranch();
  const host = await repoHostInfo();
  let reviewLabel = "Review Title";
  let title = "";
  let issues: string[] = [];
  if (host.provider === "github") {
    reviewLabel = "PR Title";
    ({ title, issues } = await githubPrInfo());
  } else if (host.provider === "gitlab") {
    reviewLabel = "MR Title";
    ({ title, issues } = await gitlabMrInfo(branch));
  }
  return formatContext(branch, reviewLabel, title, issues);
}
