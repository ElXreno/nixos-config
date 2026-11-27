#!/usr/bin/env node
import { spawn } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const playwrightCli = require.resolve("@playwright/cli/playwright-cli.js");

const browserEnvKeys = [
  "HOME",
  "USER",
  "LOGNAME",
  "PATH",
  "LANG",
  "LANGUAGE",
  "LC_ALL",
  "TZ",
  "TMPDIR",
  "DISPLAY",
  "WAYLAND_DISPLAY",
  "XAUTHORITY",
  "XDG_RUNTIME_DIR",
  "XDG_DATA_DIRS",
  "XDG_CONFIG_HOME",
  "XDG_CACHE_HOME",
  "XDG_STATE_HOME",
  "DBUS_SESSION_BUS_ADDRESS",
  "PULSE_SERVER",
  "PIPEWIRE_REMOTE",
  "GDK_BACKEND",
  "MOZ_ENABLE_WAYLAND",
  "SSL_CERT_FILE",
  "NIX_SSL_CERT_FILE",
  "LOCALE_ARCHIVE",
  "TZDIR",
];

const ownFlags = new Set(["--os", "--locale"]);

function usage() {
  process.stderr.write(
    [
      "camoufox-cli: playwright-cli driving the Camoufox browser",
      "",
      "  camoufox-cli [--session NAME | -s=NAME] <command> [args]",
      "  camoufox-cli open [url] [--headed] [--os=windows|macos|linux] [--locale=xx-YY]",
      "",
      "Every playwright-cli command works; run `camoufox-cli --help` for the list.",
      "",
    ].join("\n"),
  );
}

function parse(argv) {
  let session;
  const rest = [];
  const own = {};
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (arg === "--session" || arg === "-s") {
      session = argv[++i];
    } else if (arg.startsWith("--session=") || arg.startsWith("-s=")) {
      session = arg.slice(arg.indexOf("=") + 1);
    } else if (ownFlags.has(arg.split("=")[0])) {
      const [key, value] = arg.includes("=") ? arg.split(/=(.*)/s) : [arg, argv[++i]];
      own[key.slice(2)] = value;
    } else {
      rest.push(arg);
    }
  }
  return { session, rest, own };
}

function stateDir() {
  const base = process.env.XDG_STATE_HOME || path.join(os.homedir(), ".local", "state");
  const dir = path.join(base, "camoufox-cli");
  fs.mkdirSync(dir, { recursive: true, mode: 0o700 });
  return dir;
}

async function writeConfig(session, rest, own) {
  const { launchOptions } = await import("camoufox");
  const headed = rest.includes("--headed");
  const env = {};
  for (const key of browserEnvKeys) {
    if (process.env[key] !== undefined) env[key] = process.env[key];
  }
  const options = await launchOptions({
    headless: !headed,
    os: own.os,
    locale: own.locale,
    env,
  });
  const config = {
    browser: {
      browserName: "firefox",
      launchOptions: options,
      contextOptions: { viewport: null },
    },
  };
  const file = path.join(stateDir(), `${session ?? "default"}.json`);
  fs.writeFileSync(file, JSON.stringify(config), { mode: 0o600 });
  return file;
}

async function main() {
  const argv = process.argv.slice(2);
  if (argv.length === 0) {
    usage();
    process.exit(2);
  }
  const { session, rest, own } = parse(argv);
  const command = rest.find((arg) => !arg.startsWith("-"));
  const args = [];
  if (session) args.push(`-s=${session}`);
  if (command === "open" && !rest.some((arg) => arg.startsWith("--config"))) {
    args.push(...rest, `--config=${await writeConfig(session, rest, own)}`);
  } else {
    args.push(...rest);
  }
  const child = spawn(process.execPath, [playwrightCli, ...args], {
    stdio: "inherit",
    env: { ...process.env, NO_UPDATE_NOTIFIER: "1" },
  });
  child.on("exit", (code, signal) => {
    if (signal) process.kill(process.pid, signal);
    else process.exit(code ?? 1);
  });
}

main().catch((error) => {
  process.stderr.write(`camoufox-cli: ${error?.message ?? error}\n`);
  process.exit(1);
});
