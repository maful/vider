import { readFileSync } from "node:fs";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
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
  it("pins every platform package to the same version", () => {
    const pkg = JSON.parse(
      readFileSync(path.join(here, "..", "package.json"), "utf8")
    );
    expect(Object.keys(pkg.optionalDependencies)).toHaveLength(
      Object.keys(PLATFORM_PACKAGES).length
    );
    const versions = new Set(Object.values(pkg.optionalDependencies));
    expect(versions.size).toBe(1);
    expect(versions.has(pkg.version)).toBe(true);
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
