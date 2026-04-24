#!/usr/bin/env node

"use strict";

const { execFileSync } = require("child_process");
const path = require("path");
const os = require("os");

const PLATFORM_MAP = {
  darwin: "darwin",
  linux: "linux",
  win32: "win32",
};

const ARCH_MAP = {
  x64: "x64",
  arm64: "arm64",
};

function getBinaryName() {
  const platform = PLATFORM_MAP[os.platform()];
  const arch = ARCH_MAP[os.arch()];

  if (!platform || !arch) {
    console.error(
      `memor: unsupported platform ${os.platform()}-${os.arch()}`
    );
    process.exit(1);
  }

  const ext = platform === "win32" ? ".exe" : "";
  return `memor-${platform}-${arch}${ext}`;
}

const binaryPath = path.join(__dirname, getBinaryName());

try {
  execFileSync(binaryPath, process.argv.slice(2), { stdio: "inherit" });
} catch (err) {
  if (err.status !== null) {
    process.exit(err.status);
  }
  console.error(`memor: failed to execute binary: ${err.message}`);
  process.exit(1);
}
