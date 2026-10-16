#!/usr/bin/env node
"use strict";

// This npm package is retired. hub-clone-tool is now a single Go binary with no
// Node.js dependency; this stub only tells you how to get it.

const bold = (s) => `\u001b[1m${s}\u001b[0m`;
const yellow = (s) => `\u001b[33m${s}\u001b[0m`;
const cyan = (s) => `\u001b[36m${s}\u001b[0m`;

const lines = [
  "",
  yellow(bold("hub-clone-tool has moved to a native Go binary and this npm package is no longer maintained.")),
  "",
  "The new version keeps the same workflow and config (github.apikey, code.home) and adds",
  "a live status bar while cloning.",
  "",
  bold("Install it:"),
  "",
  `  Download a binary for your platform from`,
  `    ${cyan("https://github.com/paul-ridgway/hub-clone-tool/releases/latest")}`,
  `  and put it on your PATH, or build from source with Go:`,
  `    ${cyan("go install github.com/paul-ridgway/hub-clone-tool@latest")}`,
  `    ${cyan("go install github.com/paul-ridgway/hub-clone-tool/cmd/hct@latest")}  (for the short hct command)`,
  "",
  bold("Then remove this package:"),
  "",
  `    ${cyan("npm uninstall -g hub-clone-tool")}`,
  "",
];

process.stderr.write(lines.join("\n") + "\n");
process.exit(1);
