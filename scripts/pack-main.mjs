#!/usr/bin/env node

import { chmod, copyFile, mkdir, writeFile } from "node:fs/promises";
import path from "node:path";
import { pathToFileURL } from "node:url";
import {
  packageName,
  requiredArg,
  TARGETS,
  validateVersion,
} from "./npm-package-config.mjs";

export async function packMain({ version, source, out }) {
  validateVersion(version);

  const dir = path.join(out, "vider");
  await mkdir(dir, { recursive: true });
  for (const file of ["bin.js", "resolve.js", "README.md"]) {
    await copyFile(path.join(source, file), path.join(dir, file));
  }
  await chmod(path.join(dir, "bin.js"), 0o755);

  const optionalDependencies = Object.fromEntries(
    TARGETS.map((target) => [packageName(target), version])
  );
  const manifest = {
    name: "vider",
    version,
    description:
      "Find and delete node_modules directories to free up disk space.",
    license: "MIT",
    repository: "github:maful/vider",
    bin: { vider: "bin.js" },
    files: ["bin.js", "resolve.js", "README.md"],
    engines: { node: ">=20" },
    optionalDependencies,
    publishConfig: { access: "public" },
  };
  await writeFile(
    path.join(dir, "package.json"),
    JSON.stringify(manifest, null, 2) + "\n"
  );

  console.log(`packed ${dir}`);
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  await packMain({
    version: requiredArg("version"),
    source: requiredArg("source"),
    out: requiredArg("out"),
  });
}
