#!/usr/bin/env node
// Sets every @vider-app/* pin in packages/vider/package.json to <version>, so
// a published vider@X always resolves engine packages built from tag vX.
// release-please cannot bump these keys itself: the jsonpath-plus bundled in
// release-please-action parses "@..." path segments as value-type filters and
// throws "Unknown value type".
// Usage: node scripts/sync-pins.mjs <version> [path-to-package.json]

import { readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const version = process.argv[2];
const file =
  process.argv[3] ??
  fileURLToPath(new URL("../packages/vider/package.json", import.meta.url));

if (!version || !/^\d+\.\d+\.\d+/.test(version)) {
  console.error("usage: node scripts/sync-pins.mjs <version> [package.json path]");
  process.exit(2);
}

const pkg = JSON.parse(readFileSync(file, "utf8"));
const pins = pkg.optionalDependencies;
if (!pins || !Object.keys(pins).some((name) => name.startsWith("@vider-app/"))) {
  console.error(`no @vider-app/* pins found in ${file}`);
  process.exit(1);
}
for (const name of Object.keys(pins)) {
  if (name.startsWith("@vider-app/")) {
    pins[name] = version;
  }
}
writeFileSync(file, JSON.stringify(pkg, null, 2) + "\n");
console.log(`pins synced to ${version} in ${file}`);
