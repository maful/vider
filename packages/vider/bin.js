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
  // The child is either starting, running, or it failed to start. A binary
  // that cannot start fires "error" and then "close" with a negative errno
  // as its code, so only a child that did start may set the exit code from
  // "close". An "error" after the start (a signal that could not be
  // delivered, for example) does not end the child: "close" still reports it.
  let started = false;
  child.on("spawn", () => {
    started = true;
  });
  child.on("error", (err) => {
    if (!started) {
      fail(`vider: failed to start ${binPath}: ${err.message}`);
    }
  });
  child.on("close", (code, signal) => {
    if (started) {
      process.exitCode = code ?? (signal ? 1 : 0);
    }
  });
  for (const sig of ["SIGINT", "SIGTERM"]) {
    process.on(sig, () => child.kill(sig));
  }
}

main();
