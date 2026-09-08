"use strict";

// Maps "<platform>-<arch>" to the npm package that carries the prebuilt Go
// binary for it. Kept in one place so tests can drive every branch.
const PLATFORM_PACKAGES = {
  "darwin-arm64": "@vider-app/darwin-arm64",
  "darwin-x64": "@vider-app/darwin-x64",
  "linux-arm64": "@vider-app/linux-arm64",
  "linux-x64": "@vider-app/linux-x64",
  "win32-x64": "@vider-app/win32-x64",
};

class ViderError extends Error {
  constructor(message, exitCode = 1) {
    super(message);
    this.name = "ViderError";
    this.exitCode = exitCode;
  }
}

function platformPackageName(platform, arch) {
  return PLATFORM_PACKAGES[`${platform}-${arch}`] ?? null;
}

// resolveBinary returns the path of the vider executable to run:
// $VIDER_BIN when set (local development), otherwise the binary shipped in
// the platform package for the running platform. Platform packages export
// the absolute path of their binary from index.js.
function resolveBinary({ platform, arch, env = {}, load = require }) {
  const override = env.VIDER_BIN;
  if (override) {
    return override;
  }

  const pkg = platformPackageName(platform, arch);
  if (!pkg) {
    const supported = Object.keys(PLATFORM_PACKAGES).join(", ");
    throw new ViderError(
      `vider does not ship a binary for ${platform}-${arch} (supported: ${supported}).\n` +
        "Build the engine from source: https://github.com/maful/vider#develop-the-engine"
    );
  }

  try {
    return load(pkg);
  } catch (err) {
    if (err && err.code === "MODULE_NOT_FOUND") {
      throw new ViderError(
        `vider is missing its binary package "${pkg}".\n` +
          "Reinstall Vider (npm install -g @vider-app/cli, or npx -y @vider-app/cli@latest)."
      );
    }
    throw err;
  }
}

module.exports = {
  PLATFORM_PACKAGES,
  ViderError,
  platformPackageName,
  resolveBinary,
};
