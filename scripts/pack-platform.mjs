#!/usr/bin/env node
// Builds one @vider/<os>-<arch> npm package around a prebuilt Go binary.
// Used by the release workflow; also handy for local dry runs:
//   go build -o /tmp/vider .
//   node scripts/pack-platform.mjs --os darwin --arch arm64 --version 0.1.0 --bin /tmp/vider --out dist

import { copyFile, mkdir, writeFile } from "node:fs/promises";
import path from "node:path";

function arg(name) {
  const i = process.argv.indexOf(`--${name}`);
  if (i === -1 || i + 1 >= process.argv.length) {
    console.error(`missing --${name}`);
    process.exit(2);
  }
  return process.argv[i + 1];
}

const os = arg("os"); // goos: darwin | linux | windows
const arch = arg("arch"); // goarch: amd64 | arm64
const version = arg("version");
const bin = arg("bin");
const out = arg("out");

const npmOs = os === "windows" ? "win32" : os;
const npmCpu = arch === "amd64" ? "x64" : arch;
const name = `${npmOs}-${npmCpu}`;
const binName = os === "windows" ? "vider.exe" : "vider";

const dir = path.join(out, "@vider-app", name);
await mkdir(dir, { recursive: true });
await copyFile(bin, path.join(dir, binName));
await writeFile(
  path.join(dir, "index.js"),
  `"use strict";\nmodule.exports = require("node:path").join(__dirname, ${JSON.stringify(binName)});\n`
);

const manifest = {
  name: `@vider-app/${name}`,
  version,
  description: `Prebuilt vider engine for ${name}. Do not install directly; install vider instead.`,
  license: "MIT",
  repository: "github:maful/vider",
  os: [npmOs],
  cpu: [npmCpu],
  main: "index.js",
  files: [binName, "index.js"],
};
await writeFile(
  path.join(dir, "package.json"),
  JSON.stringify(manifest, null, 2) + "\n"
);

console.log(`packed ${dir}`);
