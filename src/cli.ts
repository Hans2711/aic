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
import { selectInteractive } from "./ui/select";
import { selectWithCombine } from "./ui/combine";
import { generateSuggestions } from "./commit/generate";
import { loadCombineConfig } from "./config";
import { generateCombinedSuggestions } from "./commit/combine";
import { offerCommit } from "./commit/offer";
import { getApiKeyForProvider } from "./providers";
import { stagedFiles } from "./git";

class MainCommand extends Command {
  static paths = [Command.Default];

  // Enrich built-in --help with environment variables
  static usage = Command.Usage({
    description: "AI-assisted git commit message generator",
    details: (() => {
      const rows = [...helpEnvRowsCore(), ...helpEnvRowsCustom()];
      const maxKey = rows.reduce((m, [k]) => Math.max(m, k.length), 0);
      const pad = (s: string) => s + " ".repeat(maxKey - s.length);
      const lines: string[] = [];
      lines.push("Environment variables:");
      lines.push("");
      for (const [k, v] of rows) lines.push(`  ${pad(k)}  ${v}`);
      return lines.join("\n");
    })(),
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
      this.context.stderr.write(`${Color.yellow}Hint:${Color.reset} export a provider API key, e.g. ${Color.green}export OPENAI_API_KEY=sk-...${Color.reset}\n`);
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
          const combined = await generateCombinedSuggestions({ provider: cc.provider, model: cc.model, suggestions: cc.suggestions, systemAddition: cc.systemAddition }, selected);
          return { suggestions: combined, modelName: cc.model };
        },
      });
      this.context.stdout.write(`\n${Color.bold}Selected commit message:${Color.reset}\n  ${Color.green}${choice}${Color.reset}\n`);
      await offerCommit(choice);
    } catch (err) {
      stop(false);
      const msg = (err as Error)?.message || String(err);
      this.context.stderr.write(`${Color.bold}${Color.red} ${Icon.error} ERROR${Color.reset}  ${Color.red}${msg}${Color.reset}\n`);
      return 1;
    }
    return 0;
  }

  private printHelp(model: string) {
    const rows = [...helpEnvRowsCore(), ...helpEnvRowsCustom()];
    const maxKey = rows.reduce((m, [k]) => Math.max(m, k.length), 0);
    const pad = (s: string) => s + " ".repeat(maxKey - s.length);
    const b = this.context.stdout;
    b.write(`${Color.bold}${Color.cyan}aic${Color.reset} – ${Color.magenta}AI-assisted git commit message generator${Color.reset}\n\n`);
    b.write(`${Color.bold}Usage${Color.reset}:\n`);
    b.write("  aic [-s \"extra instruction\"] [--version] [--no-color]\n\n");
    b.write(`${Color.bold}Arguments & Environment${Color.reset}:\n`);
    for (const [k, v] of rows) {
      const key = pad(k);
      const color = v.includes("required") ? Color.red : Color.cyan;
      b.write(`  ${Color.bold}${key}${Color.reset}  ${color}${v}${Color.reset}\n`);
    }
    b.write("\n");
    b.write(`${Color.dim}${Icon.info}${Color.reset} Default model: ${Color.cyan}${model}${Color.reset}\n`);
  }
}

class AnalyzeCommand extends Command {
  static paths = [["analyze"]];
  limit = Option.String("--limit", { required: false });
  async execute() {
    this.context.stdout.write("analyze: not implemented yet (M1 scaffolding).\n");
    return 0;
  }
}

class UpdateCommand extends Command {
  static paths = [["update"]];
  async execute() {
    this.context.stdout.write("update: not implemented yet (M1 scaffolding).\n");
    return 0;
  }
}

const cli = new Cli({ binaryLabel: "aic", binaryName: "aic", binaryVersion: VERSION });
cli.register(MainCommand);
cli.register(AnalyzeCommand);
cli.register(UpdateCommand);

await cli.runExit(process.argv.slice(2), {
  stdin: process.stdin,
  stdout: process.stdout,
  stderr: process.stderr,
});
