#!/usr/bin/env node
"use strict";

// npx @vider-app/cli → this shim → the prebuilt Go binary for the running platform.
// The TUI needs the real TTY, so stdout is inherited, never piped.

const { spawn } = require("node:child_process");
const { resolveBinary } = require("./resolve.js");

function fail(message, exitCode = 1) {
  console.error(message);
  process.exitCode = exitCode;
}

function main() {
  if (!process.stdout.isTTY) {
    fail(
      "vider needs an interactive terminal. Run it in a terminal, for example: npx @vider-app/cli"
    );
    return;
  }

  let binPath;
  try {
    binPath = resolveBinary({
      platform: process.platform,
      arch: process.arch,
      env: process.env,
    });
  } catch (err) {
    fail(err.message, err.exitCode ?? 1);
    return;
  }

  const child = spawn(binPath, process.argv.slice(2), { stdio: "inherit" });
  child.on("error", (err) => {
    fail(`vider: failed to start ${binPath}: ${err.message}`);
  });
  child.on("close", (code, signal) => {
    process.exitCode = code ?? (signal ? 1 : 0);
  });
  for (const sig of ["SIGINT", "SIGTERM"]) {
    process.on(sig, () => child.kill(sig));
  }
}

main();
