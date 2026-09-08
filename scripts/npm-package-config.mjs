export const TARGETS = Object.freeze([
  {
    goos: "darwin",
    goarch: "arm64",
    npmOs: "darwin",
    npmCpu: "arm64",
    asset: "vider_darwin_arm64",
    binary: "vider",
  },
  {
    goos: "darwin",
    goarch: "amd64",
    npmOs: "darwin",
    npmCpu: "x64",
    asset: "vider_darwin_amd64",
    binary: "vider",
  },
  {
    goos: "linux",
    goarch: "arm64",
    npmOs: "linux",
    npmCpu: "arm64",
    asset: "vider_linux_arm64",
    binary: "vider",
  },
  {
    goos: "linux",
    goarch: "amd64",
    npmOs: "linux",
    npmCpu: "x64",
    asset: "vider_linux_amd64",
    binary: "vider",
  },
  {
    goos: "windows",
    goarch: "amd64",
    npmOs: "win32",
    npmCpu: "x64",
    asset: "vider_windows_amd64.exe",
    binary: "vider.exe",
  },
]);

export function packageName(target) {
  return `@vider-app/${target.npmOs}-${target.npmCpu}`;
}

export function requiredArg(name) {
  const index = process.argv.indexOf(`--${name}`);
  if (index === -1 || index + 1 >= process.argv.length) {
    throw new Error(`missing --${name}`);
  }
  return process.argv[index + 1];
}

export function validateVersion(version) {
  const semver = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$/;
  if (!semver.test(version)) {
    throw new Error(`invalid release version: ${version}`);
  }
}
