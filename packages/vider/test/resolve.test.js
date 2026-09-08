import { readFileSync } from "node:fs";
import {
  mkdir,
  mkdtemp,
  readFile,
  rm,
  stat,
  writeFile,
} from "node:fs/promises";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import {
  PLATFORM_PACKAGES,
  ViderError,
  platformPackageName,
  resolveBinary,
} from "../resolve.js";

const run = promisify(execFile);
const here = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(here, "..", "..", "..");
// expect wraps commands in a pty for the end-to-end test (macOS `script`
// rejects piped stdin, so it cannot be used there).
const hasExpect = await run("expect", ["-v"])
  .then(() => true)
  .catch(() => false);

describe("platformPackageName", () => {
  it("maps every supported platform", () => {
    expect(platformPackageName("darwin", "arm64")).toBe("@vider-app/darwin-arm64");
    expect(platformPackageName("darwin", "x64")).toBe("@vider-app/darwin-x64");
    expect(platformPackageName("linux", "arm64")).toBe("@vider-app/linux-arm64");
    expect(platformPackageName("linux", "x64")).toBe("@vider-app/linux-x64");
    expect(platformPackageName("win32", "x64")).toBe("@vider-app/win32-x64");
  });

  it("returns null for unsupported platforms", () => {
    expect(platformPackageName("linux", "x86")).toBeNull();
    expect(platformPackageName("freebsd", "arm64")).toBeNull();
    expect(platformPackageName("win32", "arm64")).toBeNull();
    expect(platformPackageName("darwin", "ppc64")).toBeNull();
  });
});

describe("resolveBinary", () => {
  it("prefers the VIDER_BIN override", () => {
    const bin = resolveBinary({
      platform: "darwin",
      arch: "arm64",
      env: { VIDER_BIN: "/tmp/vider-dev" },
      load: () => {
        throw new Error("must not load the platform package");
      },
    });
    expect(bin).toBe("/tmp/vider-dev");
  });

  it("loads the binary path from the platform package", () => {
    const bin = resolveBinary({
      platform: "linux",
      arch: "x64",
      env: {},
      load: (name) => {
        expect(name).toBe("@vider-app/linux-x64");
        return "/pkg/bin/vider";
      },
    });
    expect(bin).toBe("/pkg/bin/vider");
  });

  it("explains unsupported platforms", () => {
    try {
      resolveBinary({
        platform: "sunos",
        arch: "x64",
        env: {},
        load: () => "/x",
      });
      expect.unreachable();
    } catch (err) {
      expect(err).toBeInstanceOf(ViderError);
      expect(err.message).toContain("sunos-x64");
      expect(err.exitCode).toBe(1);
    }
  });

  it("explains a missing platform package", () => {
    const notFound = new Error("Cannot find module");
    notFound.code = "MODULE_NOT_FOUND";
    try {
      resolveBinary({
        platform: "linux",
        arch: "x64",
        env: {},
        load: () => {
          throw notFound;
        },
      });
      expect.unreachable();
    } catch (err) {
      expect(err).toBeInstanceOf(ViderError);
      expect(err.message).toContain("@vider-app/linux-x64");
    }
  });
});

describe("package manifest", () => {
  it("is private and contains development dependencies only", () => {
    const pkg = JSON.parse(
      readFileSync(path.join(here, "..", "package.json"), "utf8")
    );
    expect(pkg.name).toBe("vider-dev");
    expect(pkg.private).toBe(true);
    expect(pkg.optionalDependencies).toBeUndefined();
  });
});

describe("generated npm packages", () => {
  it("uses one release version for the main and platform packages", async () => {
    const tmp = await mkdtemp(path.join(os.tmpdir(), "vider-npm-test-"));
    const binDir = path.join(tmp, "bin");
    const out = path.join(tmp, "out");
    const assets = [
      "vider_darwin_arm64",
      "vider_darwin_amd64",
      "vider_linux_arm64",
      "vider_linux_amd64",
      "vider_windows_amd64.exe",
    ];

    try {
      await mkdir(binDir);
      await Promise.all(
        assets.map((asset) => writeFile(path.join(binDir, asset), "test binary"))
      );
      await run(
        process.execPath,
        [
          path.join(repoRoot, "scripts", "pack-npm.mjs"),
          "--version",
          "1.2.3",
          "--bin-dir",
          binDir,
          "--out",
          out,
        ],
        { cwd: repoRoot }
      );

      const mainDir = path.join(out, "cli");
      const main = JSON.parse(
        await readFile(path.join(mainDir, "package.json"), "utf8")
      );
      expect(main.name).toBe("@vider-app/cli");
      expect(main.version).toBe("1.2.3");
      expect(main.bin).toEqual({ vider: "bin.js" });
      expect(main.publishConfig).toEqual({ access: "public" });
      expect(main.optionalDependencies).toEqual(
        Object.fromEntries(
          Object.values(PLATFORM_PACKAGES).map((name) => [name, "1.2.3"])
        )
      );

      for (const [platform, name] of Object.entries(PLATFORM_PACKAGES)) {
        const dir = path.join(out, ...name.split("/"));
        const manifest = JSON.parse(
          await readFile(path.join(dir, "package.json"), "utf8")
        );
        const [npmOs, npmCpu] = platform.split("-");
        expect(manifest.name).toBe(name);
        expect(manifest.version).toBe("1.2.3");
        expect(manifest.os).toEqual([npmOs]);
        expect(manifest.cpu).toEqual([npmCpu]);
      }

      const mode = (
        await stat(path.join(out, "@vider-app", "linux-x64", "vider"))
      ).mode;
      expect(mode & 0o111).not.toBe(0);

      const packed = await run("npm", ["pack", "--dry-run", "--json"], {
        cwd: mainDir,
        env: { ...process.env, npm_config_cache: path.join(tmp, "npm-cache") },
      });
      const [{ files }] = JSON.parse(packed.stdout);
      expect(files.map((file) => file.path).sort()).toEqual([
        "README.md",
        "bin.js",
        "package.json",
        "resolve.js",
      ]);
    } finally {
      await rm(tmp, { recursive: true, force: true });
    }
  });
});

describe("bin.js", () => {
  it("refuses to run the TUI without a TTY", async () => {
    await expect(
      run("node", [path.join(here, "..", "bin.js")])
    ).rejects.toMatchObject({ code: 1 });
  }, 15_000);
});

describe("end-to-end with a real binary", () => {
  const bin = process.env.VIDER_BIN;

  it.runIf(bin && hasExpect)(
    "spawns the Go binary and propagates its exit code",
    async () => {
      const script =
        "spawn $env(VIDER_BIN) /nonexistent-vider-e2e\n" +
        "expect eof\n" +
        "catch wait result\n" +
        "exit [lindex $result 3]";
      const res = await run("expect", ["-c", script]).catch((err) => err);
      expect(res.code).toBe(1);
      expect(String(res.stdout)).toContain("root not accessible");
    },
    30_000
  );
});
