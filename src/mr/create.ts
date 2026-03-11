import { spawn } from "node:child_process";
import { currentBranch, currentRemote, insideRepo, repoHostInfo, type RepoHostingProvider } from "../git";
import { Env, envBool, loadConfig } from "../config";
import { getApiKeyForProvider, newProviderClient, type ProviderName } from "../providers";
import { Color } from "../ui/colors";
import { GENERATION_CONFIG } from "../constants";
import { debugInfo } from "../debug";

export type MergeRequestConfig = ReturnType<typeof loadConfig> & { provider: ProviderName };

type CreateMergeRequestOptions = {
  targetBranch?: string;
  draft?: boolean;
  confirm?: boolean;
};

type CommitInfo = {
  sha: string;
  shortSha: string;
  subject: string;
  body: string;
};

type BranchTarget = {
  remote: string;
  targetBranch: string;
  baseRef: string;
  repoProvider: RepoHostingProvider;
  hostname: string;
  repoUrl: string;
  ghRepo: string;
};

type MergeRequestContent = {
  title: string;
  description: string;
};

type ExistingReview = {
  title: string;
  description: string;
  url: string;
};

export type PreparedMergeRequest = {
  branch: string;
  targetBranch: string;
  repoProvider: RepoHostingProvider;
  hostname: string;
  repoUrl: string;
  ghRepo: string;
  existingReview?: ExistingReview;
  content: MergeRequestContent;
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
      const msg = errorMessage(error) || "spawn failed";
      resolve({ stdout: "", stderr: msg, code: 127 });
      return;
    }
    let out = "";
    let err = "";
    child.stdout?.setEncoding("utf8");
    child.stderr?.setEncoding("utf8");
    child.stdout?.on("data", (d) => (out += d));
    child.stderr?.on("data", (d) => (err += d));
    child.on("error", (error: unknown) => {
      const msg = errorMessage(error) || "spawn error";
      resolve({ stdout: out, stderr: msg, code: 127 });
    });
    child.on("close", (code) => resolve({ stdout: out, stderr: err, code: code ?? 0 }));
    if (opts?.stdin) child.stdin?.write(opts.stdin);
    child.stdin?.end();
  });
}

async function git(...args: string[]): Promise<{ stdout: string; stderr: string; code: number }> {
  return run("git", args);
}

async function gh(...args: string[]): Promise<{ stdout: string; stderr: string; code: number }> {
  return run("gh", args);
}

async function glab(...args: string[]): Promise<{ stdout: string; stderr: string; code: number }> {
  return run("glab", args);
}

async function gitStdout(...args: string[]): Promise<string> {
  const res = await git(...args);
  if (res.code !== 0) {
    throw new Error(res.stderr || res.stdout || `git ${args.join(" ")} failed`);
  }
  return res.stdout;
}

async function safeGit(...args: string[]): Promise<string> {
  const res = await git(...args);
  if (res.code !== 0) return "";
  return res.stdout;
}

async function resolveTarget(branch: string, requestedTarget?: string): Promise<BranchTarget> {
  const host = await repoHostInfo();
  const remote = await currentRemote();
  const targetBranch = requestedTarget?.trim() || (await detectDefaultBranch(remote));
  const baseRef = await resolveBaseRef(remote, targetBranch);
  return {
    remote,
    targetBranch,
    baseRef,
    repoProvider: host.provider,
    hostname: host.hostname,
    repoUrl: host.url,
    ghRepo: host.ghRepo,
  };
}

async function detectDefaultBranch(remote: string): Promise<string> {
  if (remote) {
    const symRef = (await safeGit("symbolic-ref", `refs/remotes/${remote}/HEAD`)).trim();
    const prefix = `refs/remotes/${remote}/`;
    if (symRef.startsWith(prefix)) return symRef.slice(prefix.length);

    const remoteShow = await safeGit("remote", "show", "-n", remote);
    const match = /^\s*HEAD branch:\s+(.+)$/m.exec(remoteShow);
    if (match?.[1]) return match[1].trim();
  }

  const configured = (await safeGit("config", "--get", "init.defaultBranch")).trim();
  if (configured) return configured;

  for (const candidate of ["main", "master"]) {
    const exists = (await safeGit("rev-parse", "--verify", candidate)).trim();
    if (exists) return candidate;
  }

  throw new Error("could not determine the default branch");
}

async function resolveBaseRef(remote: string, targetBranch: string): Promise<string> {
  if (remote) {
    const remoteRef = `${remote}/${targetBranch}`;
    const exists = (await safeGit("rev-parse", "--verify", remoteRef)).trim();
    if (exists) return remoteRef;
  }

  const local = (await safeGit("rev-parse", "--verify", targetBranch)).trim();
  if (local) return targetBranch;

  throw new Error(`could not resolve target branch '${targetBranch}' locally`);
}

async function collectCommits(baseRef: string): Promise<CommitInfo[]> {
  const format = "%H%x1f%h%x1f%s%x1f%b%x1e";
  const stdout = await gitStdout("log", "--reverse", `--pretty=format:${format}`, `${baseRef}..HEAD`);
  return stdout
    .split("\x1e")
    .map((record) => record.trim())
    .filter(Boolean)
    .map((record) => {
      const [sha = "", shortSha = "", subject = "", body = ""] = record.split("\x1f");
      return {
        sha: sha.trim(),
        shortSha: shortSha.trim(),
        subject: subject.trim(),
        body: body.trim(),
      };
    })
    .filter((commit) => commit.sha && commit.subject);
}

async function changedFiles(baseRef: string): Promise<string[]> {
  const stdout = await gitStdout("diff", "--name-only", `${baseRef}...HEAD`);
  return stdout
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
}

async function diffStat(baseRef: string): Promise<string> {
  return (await safeGit("diff", "--stat=120,80", "--find-renames", `${baseRef}...HEAD`)).trim();
}

function buildPrompt(
  branch: string,
  target: BranchTarget,
  commits: CommitInfo[],
  files: string[],
  stat: string,
  systemAddition: string,
  existingReview?: ExistingReview
): Array<{ role: "system" | "user"; content: string }> {
  const reviewTerm = target.repoProvider === "github" ? "pull request" : "merge request";
  let system =
    `You write ${reviewTerm} titles and descriptions. ` +
    "Ground the output strictly in the provided branch, commit list, changed files, and diff stats. " +
    "The title must be one line, specific, and no more than 72 characters, with no trailing period. " +
    "The description must be Markdown and contain exactly these sections: " +
    "'## Summary', '## Testing', and '## Commit Breakdown'. " +
    "In '## Summary', write 2-5 bullets that summarize the branch as a whole. " +
    "In '## Testing', if no testing evidence is provided, write exactly '- Not run (not mentioned in commits)'. " +
    "In '## Commit Breakdown', include one bullet per commit and make sure every listed commit appears exactly once, using its short SHA and a concise explanation grounded in the commit subject/body. " +
    "If an existing review title or description is provided, treat it as the current version and propose an improved update rather than repeating it verbatim unless it is already optimal. " +
    "Do not invent issues, deployment steps, metrics, or tests. " +
    "Output exactly in this format:\nTITLE: <title>\nDESCRIPTION:\n<markdown>";
  if (systemAddition) system += ` Additional user instructions: ${systemAddition}`;

  const commitLines = commits.map((commit, index) => {
    const body = commit.body ? `\nBody:\n${trimBody(commit.body)}` : "";
    return `${index + 1}. ${commit.shortSha} ${commit.subject}${body}`;
  });

  let user = "";
  user += `Source branch: ${branch}\n`;
  user += `Target branch: ${target.targetBranch}\n`;
  if (target.remote) user += `Remote: ${target.remote}\n`;
  user += `Commit count: ${commits.length}\n`;
  if (files.length) {
    user += `Changed files (${files.length}):\n${files.map((file) => `- ${file}`).join("\n")}\n`;
  }
  if (stat) {
    user += `\nDiff stat:\n${stat}\n`;
  }
  if (existingReview) {
    user += `\nExisting ${reviewTerm} title:\n${existingReview.title}\n`;
    user += `\nExisting ${reviewTerm} description:\n${existingReview.description || "(empty)"}\n`;
  }
  user += `\nCommits on this branch not in ${target.baseRef}:\n${commitLines.join("\n\n")}`;

  return [
    { role: "system", content: system },
    { role: "user", content: user.trim() },
  ];
}

function trimBody(body: string): string {
  const trimmed = body.trim();
  if (!trimmed) return "";
  const lines = trimmed.split(/\r?\n/).slice(0, 12);
  const text = lines.join("\n").trim();
  if (trimmed.length <= text.length) return text;
  return `${text}\n...`;
}

function parseMergeRequestContent(text: string): MergeRequestContent {
  const raw = text.trim();
  const titleMatch = /^\s*TITLE:\s*(.+)\s*$/m.exec(raw);
  const descMatch = /^\s*DESCRIPTION:\s*$/m.exec(raw);

  if (titleMatch && descMatch && typeof descMatch.index === "number") {
    const description = raw.slice(descMatch.index + descMatch[0].length).trim();
    return normalizeContent(titleMatch[1], description);
  }

  const lines = raw.split(/\r?\n/).map((line) => line.trimEnd());
  const firstNonEmpty = lines.find((line) => line.trim());
  const title = firstNonEmpty || "Update branch changes";
  const description = lines
    .slice(lines.indexOf(firstNonEmpty || "") + 1)
    .join("\n")
    .trim();
  return normalizeContent(
    title,
    description ||
      "## Summary\n- Summary unavailable\n\n## Testing\n- Not run (not mentioned in commits)\n\n## Commit Breakdown\n- Details unavailable"
  );
}

function normalizeContent(title: string, description: string): MergeRequestContent {
  const cleanTitle = title
    .replace(/^["'\s]+|["'\s]+$/g, "")
    .replace(/\.+$/, "")
    .trim();
  const cleanDescription = description.trim();
  if (!cleanTitle) throw new Error("generated MR title was empty");
  if (!cleanDescription) throw new Error("generated MR description was empty");
  return { title: cleanTitle, description: cleanDescription };
}

function mockMergeRequestContent(branch: string, commits: CommitInfo[]): MergeRequestContent {
  const titleBase = commits[commits.length - 1]?.subject || `Update ${branch}`;
  const title = titleBase.replace(/\.+$/, "").trim();
  const description = [
    "## Summary",
    `- Update \`${branch}\` with ${commits.length} branch commit${commits.length === 1 ? "" : "s"}.`,
    "- Generated in mock mode; review before creating the merge request.",
    "",
    "## Testing",
    "- Not run (not mentioned in commits)",
    "",
    "## Commit Breakdown",
    ...commits.map((commit) => `- ${commit.shortSha} ${commit.subject}`),
  ].join("\n");
  return { title, description };
}

async function generateMergeRequestContent(
  cfg: MergeRequestConfig,
  branch: string,
  target: BranchTarget,
  commits: CommitInfo[],
  files: string[],
  stat: string,
  existingReview?: ExistingReview
): Promise<MergeRequestContent> {
  if (envBool(Env.AIC_MOCK)) {
    return mockMergeRequestContent(branch, commits);
  }

  const apiKey = getApiKeyForProvider(cfg.provider);
  const client = newProviderClient(cfg.provider, apiKey);
  const messages = buildPrompt(branch, target, commits, files, stat, cfg.systemAddition, existingReview);
  const maxTokens = cfg.model.includes("gpt-5") ? Math.max(GENERATION_CONFIG.MAX_TOKENS_REASONING, 2200) : 1400;
  const response = await client.chat({
    model: cfg.model,
    messages,
    maxTokens,
    temperature: 0.3,
    n: 1,
  });
  const text = response.choices[0];
  if (!text?.trim()) throw new Error("empty MR draft from provider");
  return parseMergeRequestContent(text);
}

async function readLine(): Promise<string> {
  return new Promise((resolve) => {
    const stdin = process.stdin;
    stdin.setRawMode?.(false);
    stdin.setEncoding("utf8");
    stdin.resume();
    let buffer = "";
    const onData = (chunk: string) => {
      if (chunk === "\u0003") {
        stdin.off("data", onData);
        stdin.pause();
        process.exit(130);
      }
      if (chunk === "\n" || chunk === "\r") {
        stdin.off("data", onData);
        stdin.pause();
        resolve(buffer);
        return;
      }
      buffer += chunk;
    };
    stdin.on("data", onData);
  });
}

async function confirmCreate(question: string): Promise<boolean> {
  process.stdout.write(`\n${Color.bold}${question}${Color.reset} ${Color.yellow}[Y|n]${Color.reset}: ${Color.cyan}`);
  const answer = (await readLine()).trim().toLowerCase();
  process.stdout.write(Color.reset);
  return !answer || answer === "y" || answer === "yes";
}

function renderPreview(content: MergeRequestContent): void {
  process.stdout.write(`\n${Color.bold}Review title:${Color.reset}\n`);
  process.stdout.write(`  ${Color.green}${content.title}${Color.reset}\n`);
  process.stdout.write(`\n${Color.bold}Review description:${Color.reset}\n`);
  process.stdout.write(`${content.description}\n`);
}

function renderExistingReview(existingReview: ExistingReview): void {
  process.stdout.write(`\n${Color.bold}Current review title:${Color.reset}\n`);
  process.stdout.write(`  ${Color.yellow}${existingReview.title}${Color.reset}\n`);
  process.stdout.write(`\n${Color.bold}Current review description:${Color.reset}\n`);
  process.stdout.write(`${existingReview.description || "(empty)"}\n`);
}

async function pushCurrentBranch(remote: string, branch: string): Promise<void> {
  if (!remote) throw new Error("no Git remote configured for the current branch");
  const upstream = (await safeGit("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")).trim();
  if (upstream) {
    const res = await git("push");
    if (res.code !== 0) throw new Error(res.stderr || res.stdout || "git push failed");
    return;
  }
  const res = await git("push", "-u", remote, branch);
  if (res.code !== 0) throw new Error(res.stderr || res.stdout || `git push -u ${remote} ${branch} failed`);
}

function compactMessage(text: string): string {
  return text.replace(/\s+/g, " ").trim();
}

async function ensureCliAuth(provider: RepoHostingProvider, hostname: string): Promise<void> {
  if (provider === "github") {
    const args = ["auth", "status"];
    if (hostname) args.push("--hostname", hostname);
    const res = await gh(...args);
    if (res.code !== 0) {
      const detail = compactMessage(res.stderr || res.stdout);
      const hostLabel = hostname || "the GitHub host";
      throw new Error(
        `gh is not authenticated for ${hostLabel}; run 'gh auth login --hostname ${hostLabel}'${detail ? ` (${detail})` : ""}`
      );
    }
    return;
  }
  if (provider === "gitlab") {
    const args = ["auth", "status"];
    if (hostname) args.push("--hostname", hostname);
    const res = await glab(...args);
    if (res.code !== 0) {
      const detail = compactMessage(res.stderr || res.stdout);
      const hostLabel = hostname || "the GitLab host";
      throw new Error(
        `glab is not authenticated for ${hostLabel}; run 'glab auth login --hostname ${hostLabel}'${detail ? ` (${detail})` : ""}`
      );
    }
  }
}

async function createWithGh(
  ghRepo: string,
  branch: string,
  targetBranch: string,
  content: MergeRequestContent,
  draft: boolean
): Promise<string> {
  const args = [
    "pr",
    "create",
    "--head",
    branch,
    "--base",
    targetBranch,
    "--title",
    content.title,
    "--body",
    content.description,
  ];
  if (ghRepo) args.push("-R", ghRepo);
  if (draft) args.push("--draft");
  const res = await gh(...args);
  if (res.code === 127) {
    throw new Error("gh is not installed or not available in PATH");
  }
  if (res.code !== 0) {
    throw new Error(res.stderr || res.stdout || "gh pr create failed");
  }
  return (res.stdout || res.stderr).trim();
}

async function updateWithGh(
  ghRepo: string,
  branch: string,
  targetBranch: string,
  content: MergeRequestContent
): Promise<string> {
  const args = ["pr", "edit", branch, "--title", content.title, "--body", content.description, "--base", targetBranch];
  if (ghRepo) args.push("-R", ghRepo);
  const res = await gh(...args);
  if (res.code === 127) {
    throw new Error("gh is not installed or not available in PATH");
  }
  if (res.code !== 0) {
    throw new Error(res.stderr || res.stdout || "gh pr edit failed");
  }
  return (res.stdout || res.stderr).trim();
}

async function createWithGlab(
  repoUrl: string,
  branch: string,
  targetBranch: string,
  content: MergeRequestContent,
  draft: boolean
): Promise<string> {
  const args = [
    "mr",
    "create",
    "--source-branch",
    branch,
    "--target-branch",
    targetBranch,
    "--title",
    content.title,
    "--description",
    content.description,
    "--push",
    "--yes",
  ];
  if (repoUrl) args.push("--repo", repoUrl);
  if (draft) args.push("--draft");

  const res = await glab(...args);
  if (res.code === 127) {
    throw new Error("glab is not installed or not available in PATH");
  }
  if (res.code !== 0) {
    throw new Error(res.stderr || res.stdout || "glab mr create failed");
  }
  return (res.stdout || res.stderr).trim();
}

async function updateWithGlab(
  repoUrl: string,
  branch: string,
  targetBranch: string,
  content: MergeRequestContent,
  draft: boolean
): Promise<string> {
  const args = [
    "mr",
    "update",
    branch,
    "--title",
    content.title,
    "--description",
    content.description,
    "--target-branch",
    targetBranch,
    "--yes",
  ];
  if (repoUrl) args.push("--repo", repoUrl);
  if (draft) args.push("--draft");

  const res = await glab(...args);
  if (res.code === 127) {
    throw new Error("glab is not installed or not available in PATH");
  }
  if (res.code !== 0) {
    throw new Error(res.stderr || res.stdout || "glab mr update failed");
  }
  return (res.stdout || res.stderr).trim();
}

async function findExistingGithubReview(ghRepo: string, branch: string): Promise<ExistingReview | undefined> {
  const args = ["pr", "view", branch, "--json", "title,body,url"];
  if (ghRepo) args.push("-R", ghRepo);
  const res = await gh(...args);
  if (res.code !== 0) return undefined;
  try {
    const data = JSON.parse(res.stdout) as { title?: string; body?: string; url?: string };
    const title = (data.title || "").trim();
    if (!title) return undefined;
    return {
      title,
      description: (data.body || "").trim(),
      url: (data.url || "").trim(),
    };
  } catch {
    return undefined;
  }
}

async function findExistingGitlabReview(repoUrl: string, branch: string): Promise<ExistingReview | undefined> {
  const args = ["mr", "view", branch, "--output", "json"];
  if (repoUrl) args.push("--repo", repoUrl);
  const res = await glab(...args);
  if (res.code !== 0) return undefined;
  try {
    const data = JSON.parse(res.stdout) as { title?: string; description?: string; web_url?: string };
    const title = (data.title || "").trim();
    if (!title) return undefined;
    return {
      title,
      description: (data.description || "").trim(),
      url: (data.web_url || "").trim(),
    };
  } catch {
    return undefined;
  }
}

async function findExistingReview(
  provider: RepoHostingProvider,
  repoUrl: string,
  ghRepo: string,
  branch: string
): Promise<ExistingReview | undefined> {
  if (provider === "github") return findExistingGithubReview(ghRepo, branch);
  if (provider === "gitlab") return findExistingGitlabReview(repoUrl, branch);
  return undefined;
}

async function createWithHost(
  provider: RepoHostingProvider,
  hostname: string,
  remote: string,
  repoUrl: string,
  ghRepo: string,
  branch: string,
  targetBranch: string,
  content: MergeRequestContent,
  existingReview: ExistingReview | undefined,
  draft: boolean
): Promise<string> {
  if (provider === "github") {
    await ensureCliAuth(provider, hostname);
    if (existingReview) {
      return updateWithGh(ghRepo, branch, targetBranch, content);
    }
    await pushCurrentBranch(remote, branch);
    return createWithGh(ghRepo, branch, targetBranch, content, draft);
  }
  if (provider === "gitlab") {
    await ensureCliAuth(provider, hostname);
    if (existingReview) {
      return updateWithGlab(repoUrl, branch, targetBranch, content, draft);
    }
    return createWithGlab(repoUrl, branch, targetBranch, content, draft);
  }
  throw new Error("unsupported Git remote host; expected GitHub or GitLab");
}

export async function draftMergeRequest(
  cfg: MergeRequestConfig,
  options: CreateMergeRequestOptions = {}
): Promise<PreparedMergeRequest> {
  if (!(await insideRepo())) {
    throw new Error("not a git repository");
  }

  const branch = (await currentBranch()).trim();
  if (!branch || branch === "HEAD") {
    throw new Error("cannot create a merge request from a detached HEAD");
  }

  const target = await resolveTarget(branch, options.targetBranch);
  if (target.repoProvider === "unknown") {
    throw new Error("unsupported Git remote host; expected GitHub or GitLab");
  }
  if (branch === target.targetBranch) {
    throw new Error(`current branch '${branch}' matches the target branch`);
  }

  debugInfo("GIT", `MR source=${branch}, target=${target.targetBranch}, baseRef=${target.baseRef}`);
  const existingReview = await findExistingReview(target.repoProvider, target.repoUrl, target.ghRepo, branch);

  const commits = await collectCommits(target.baseRef);
  if (commits.length === 0) {
    throw new Error(`no commits found on '${branch}' that are not already in '${target.baseRef}'`);
  }

  const [files, stat] = await Promise.all([changedFiles(target.baseRef), diffStat(target.baseRef)]);
  const content = await generateMergeRequestContent(cfg, branch, target, commits, files, stat, existingReview);
  return {
    branch,
    targetBranch: target.targetBranch,
    repoProvider: target.repoProvider,
    hostname: target.hostname,
    repoUrl: target.repoUrl,
    ghRepo: target.ghRepo,
    existingReview,
    content,
  };
}

export async function submitMergeRequest(
  prepared: PreparedMergeRequest,
  options: CreateMergeRequestOptions = {}
): Promise<void> {
  if (prepared.existingReview) renderExistingReview(prepared.existingReview);
  renderPreview(prepared.content);
  const reviewLabel = prepared.repoProvider === "github" ? "pull request" : "merge request";
  const action = prepared.existingReview ? "Update" : "Create";
  const shouldCreate = options.confirm === false ? true : await confirmCreate(`${action} this ${reviewLabel} now?`);
  if (!shouldCreate) {
    process.stdout.write(`${Color.yellow}${action} canceled.${Color.reset}\n`);
    return;
  }

  const remote = await currentRemote();
  const output = await createWithHost(
    prepared.repoProvider,
    prepared.hostname,
    remote,
    prepared.repoUrl,
    prepared.ghRepo,
    prepared.branch,
    prepared.targetBranch,
    prepared.content,
    prepared.existingReview,
    !!options.draft
  );
  if (output) process.stdout.write(`\n${output}\n`);
}

export async function createMergeRequest(
  cfg: MergeRequestConfig,
  options: CreateMergeRequestOptions = {}
): Promise<void> {
  const prepared = await draftMergeRequest(cfg, options);
  await submitMergeRequest(prepared, options);
}
