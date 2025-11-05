#!/usr/bin/env -S bun
// Build the CLI into dist/aic (single executable JS with a bun shebang)

const entry = "src/cli.ts";
const outdir = "dist";

const res = await Bun.build({
  entrypoints: [entry],
  outdir,
  target: "bun",
  format: "esm",
  splitting: false,
  minify: true,
  sourcemap: "none",
});

if (!res.success) {
  for (const log of res.logs) {
    console.error(log.message);
  }
  process.exit(1);
}

// Find the output file for the entry
let builtPath = res.outputs.find((o) => o.kind === "entry-point")?.path;
if (!builtPath) {
  // Fallback: first output
  builtPath = res.outputs[0]?.path;
}
if (!builtPath) {
  console.error("Build succeeded but no output path found");
  process.exit(1);
}

let js = await Bun.file(builtPath).text();
// Remove any existing shebangs from the bundled output to avoid syntax errors
js = js.replace(/^#!.*$/gm, "").replace(/^\n+/, "");
const shebang = "#!/usr/bin/env bun\n";
const finalPath = `${outdir}/aic`;
await Bun.write(finalPath, shebang + js);

// Make executable (best-effort)
try {
  await Bun.spawn(["chmod", "+x", finalPath]).exited;
} catch {}

console.log(`Built ${finalPath}`);
