#!/usr/bin/env node

import path from "node:path";
import { fileURLToPath } from "node:url";
import {
  packageName,
  requiredArg,
  TARGETS,
  validateVersion,
} from "./npm-package-config.mjs";
import { packMain } from "./pack-main.mjs";
import { packPlatform } from "./pack-platform.mjs";

const version = requiredArg("version");
const binDir = requiredArg("bin-dir");
const out = requiredArg("out");
validateVersion(version);

const source = fileURLToPath(new URL("../packages/vider/", import.meta.url));
await Promise.all([
  packMain({ version, source, out }),
  ...TARGETS.map((target) =>
    packPlatform({
      target,
      version,
      bin: path.join(binDir, target.asset),
      out,
    })
  ),
]);

console.log(`generated vider ${version} and ${TARGETS.length} platform packages`);
for (const target of TARGETS) {
  console.log(`  ${packageName(target)}@${version}`);
}
