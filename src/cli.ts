#!/usr/bin/env bun
import { Cli, Command, Option } from "clipanion";
import { VERSION } from "./version";
import {
  Env,
  helpEnvRowsCore,
  helpEnvRowsCustom,
  warnUnknownAICEnv,
  loadConfig,
  envBool,
  daemonEnabled,
} from "./config";
import { Color, Icon, initColors } from "./ui/colors";
import { spinner } from "./ui/spinner";
import { selectWithCombine } from "./ui/combine";
import { generateSuggestions } from "./commit/generate";
import { loadCombineConfig } from "./config";
import { generateCombinedSuggestions } from "./commit/combine";
import { offerCommit } from "./commit/offer";
import { getApiKeyForProvider } from "./providers";
import { stagedFiles } from "./git";
import { formatEnvVarsHelp, formatEnvVarsTable } from "./ui/table";
import { draftMergeRequest, submitMergeRequest } from "./mr/create";

class MainCommand extends Command {
  static paths = [Command.Default];

  // Enrich built-in --help with environment variables
  static usage = Command.Usage({
    description: "AI-assisted git commit message generator",
    details: formatEnvVarsHelp(helpEnvRowsCore(), helpEnvRowsCustom()),
  });

  version = Option.Boolean("-v,--version", false, { description: "Show version and exit" });
  noColor = Option.Boolean("--no-color", false, { description: "Disable colored output" });
  systemAddition = Option.String("-s", { required: false, description: "Extra instruction for prompts" });

  async execute() {
    initColors(this.noColor);
    warnUnknownAICEnv();

    if (this.version) {
      this.context.stdout.write(`aic ${VERSION}\n`);
      return 0;
    }
    const cfg = loadConfig(this.systemAddition ?? "");
    // If no API key for the chosen provider (except custom), print help and exit gracefully
    const apiKey = getApiKeyForProvider(cfg.provider);
    if (!apiKey && cfg.provider !== "custom" && !envBool(Env.AIC_MOCK)) {
      this.printHelp(cfg.model);
      this.context.stderr.write(
        `${Color.yellow}Hint:${Color.reset} export a provider API key, e.g. ${Color.green}export OPENAI_API_KEY=sk-...${Color.reset}\n`
      );
      return 1;
    }
    // Show staged files included in the diff (transparency), unless in daemon mode
    if (!daemonEnabled()) {
      try {
        const files = await stagedFiles();
        if (files && files.length > 0) {
          this.context.stdout.write(`${Color.gray}${Color.bold} Staged changes:${Color.reset}\n`);
          for (const f of files) {
            this.context.stdout.write(`  ${Color.yellow}- ${f}${Color.reset}\n`);
          }
        }
      } catch {
        // ignore errors fetching staged files
      }
    }
    // Generate suggestions
    const stop = spinner(`Requesting ${cfg.suggestions} suggestions from ${cfg.model}`);
    try {
      const sugs = await generateSuggestions({ ...cfg, provider: cfg.provider });
      stop(true);
      const nonInteractive = envBool(Env.AIC_NON_INTERACTIVE);
      if (nonInteractive) {
        const choice = sugs[0];
        this.context.stdout.write(choice + "\n");
        return 0;
      }
      const choice = await selectWithCombine({
        title: "Commit message suggestions",
        items: sugs,
        onCombine: async (selected) => {
          const cc = loadCombineConfig(this.systemAddition ?? "");
          const combined = await generateCombinedSuggestions(
            { provider: cc.provider, model: cc.model, suggestions: cc.suggestions, systemAddition: cc.systemAddition },
            selected
          );
          return { suggestions: combined, modelName: cc.model };
        },
      });
      this.context.stdout.write(
        `\n${Color.bold}Selected commit message:${Color.reset}\n  ${Color.green}${choice}${Color.reset}\n`
      );
      await offerCommit(choice);
    } catch (err) {
      stop(false);
      const msg = (err as Error)?.message || String(err);
      this.context.stderr.write(
        `${Color.bold}${Color.red} ${Icon.error} ERROR${Color.reset}  ${Color.red}${msg}${Color.reset}\n`
      );
      return 1;
    }
    return 0;
  }

  private printHelp(model: string) {
    const b = this.context.stdout;
    b.write(
      `${Color.bold}${Color.cyan}aic${Color.reset} – ${Color.magenta}AI-assisted git commit message generator${Color.reset}\n\n`
    );
    b.write(`${Color.bold}Usage${Color.reset}:\n`);
    b.write('  aic [-s "extra instruction"] [--version] [--no-color]\n\n');
    b.write(`${Color.bold}${formatEnvVarsTable(helpEnvRowsCore(), helpEnvRowsCustom())}${Color.reset}\n`);
    b.write("\n");
    b.write(`${Color.dim}${Icon.info}${Color.reset} Default model: ${Color.cyan}${model}${Color.reset}\n`);
  }
}

class AnalyzeCommand extends Command {
  static paths = [["analyze"]];
  limit = Option.String("--limit", { required: false });
  execute(): Promise<number> {
    this.context.stdout.write("analyze: not implemented yet (M1 scaffolding).\n");
    return Promise.resolve(0);
  }
}

class UpdateCommand extends Command {
  static paths = [["update"]];
  execute(): Promise<number> {
    this.context.stdout.write("update: not implemented yet (M1 scaffolding).\n");
    return Promise.resolve(0);
  }
}

class MergeRequestCommand extends Command {
  static paths = [["mr"]];

  static usage = Command.Usage({
    description: "Create a pull request or merge request for the current branch",
    details: formatEnvVarsHelp(helpEnvRowsCore(), helpEnvRowsCustom()),
  });

  noColor = Option.Boolean("--no-color", false, { description: "Disable colored output" });
  systemAddition = Option.String("-s", { required: false, description: "Extra instruction for MR generation" });
  targetBranch = Option.String("-b,--target-branch", { required: false, description: "Override the MR target branch" });
  draft = Option.Boolean("--draft", false, { description: "Create the merge request as a draft" });

  async execute() {
    initColors(this.noColor);
    warnUnknownAICEnv();

    const cfg = loadConfig(this.systemAddition ?? "");
    const apiKey = getApiKeyForProvider(cfg.provider);
    if (!apiKey && cfg.provider !== "custom" && !envBool(Env.AIC_MOCK)) {
      this.context.stdout.write(this.cli.usage(MergeRequestCommand, { detailed: true }));
      this.context.stderr.write(
        `${Color.yellow}Hint:${Color.reset} export a provider API key, e.g. ${Color.green}export OPENAI_API_KEY=sk-...${Color.reset}\n`
      );
      return 1;
    }

    const stop = spinner(`Drafting merge request for the current branch with ${cfg.model}`);
    try {
      const prepared = await draftMergeRequest(
        { ...cfg, provider: cfg.provider },
        { targetBranch: this.targetBranch, draft: this.draft }
      );
      stop(true);
      await submitMergeRequest(prepared, { draft: this.draft, confirm: !envBool(Env.AIC_NON_INTERACTIVE) });
      return 0;
    } catch (err) {
      stop(false);
      const msg = (err as Error)?.message || String(err);
      this.context.stderr.write(
        `${Color.bold}${Color.red} ${Icon.error} ERROR${Color.reset}  ${Color.red}${msg}${Color.reset}\n`
      );
      return 1;
    }
  }
}

const cli = new Cli({ binaryLabel: "aic", binaryName: "aic", binaryVersion: VERSION });
cli.register(MainCommand);
cli.register(MergeRequestCommand);
cli.register(AnalyzeCommand);
cli.register(UpdateCommand);

await cli.runExit(process.argv.slice(2), {
  stdin: process.stdin,
  stdout: process.stdout,
  stderr: process.stderr,
});
