#!/usr/bin/env node
// Builds one @vider-app/<os>-<arch> npm package around a prebuilt Go binary.
// Used by the release workflow; also handy for local dry runs:
//   go build -o /tmp/vider .
//   node scripts/pack-platform.mjs --os darwin --arch arm64 --version 0.1.0 --bin /tmp/vider --out dist

import { chmod, copyFile, mkdir, writeFile } from "node:fs/promises";
import path from "node:path";
import { pathToFileURL } from "node:url";
import {
  packageName,
  requiredArg,
  TARGETS,
  validateVersion,
} from "./npm-package-config.mjs";

export async function packPlatform({ target, version, bin, out }) {
  validateVersion(version);

  const name = `${target.npmOs}-${target.npmCpu}`;
  const dir = path.join(out, "@vider-app", name);
  const destination = path.join(dir, target.binary);
  await mkdir(dir, { recursive: true });
  await copyFile(bin, destination);
  if (target.npmOs !== "win32") {
    await chmod(destination, 0o755);
  }
  await writeFile(
    path.join(dir, "index.js"),
    `"use strict";\nmodule.exports = require("node:path").join(__dirname, ${JSON.stringify(target.binary)});\n`
  );

  const manifest = {
    name: packageName(target),
    version,
    description: `Prebuilt vider engine for ${name}. Do not install directly; install vider instead.`,
    license: "MIT",
    repository: "github:maful/vider",
    os: [target.npmOs],
    cpu: [target.npmCpu],
    main: "index.js",
    files: [target.binary, "index.js"],
    publishConfig: { access: "public" },
  };
  await writeFile(
    path.join(dir, "package.json"),
    JSON.stringify(manifest, null, 2) + "\n"
  );

  console.log(`packed ${dir}`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const goos = requiredArg("os");
  const goarch = requiredArg("arch");
  const target = TARGETS.find(
    (candidate) => candidate.goos === goos && candidate.goarch === goarch
  );
  if (!target) {
    throw new Error(`unsupported target: ${goos}-${goarch}`);
  }

  await packPlatform({
    target,
    version: requiredArg("version"),
    bin: requiredArg("bin"),
    out: requiredArg("out"),
  });
}
