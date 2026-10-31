// SPDX-License-Identifier: MPL-2.0

import { readFileSync } from "node:fs";
import path from "node:path";

import { expect, it } from "vitest";

it("installs no package's scripts, so each package is its tarball and runs nothing at install", () => {
  const settings = readFileSync(path.join(import.meta.dirname, "../../../.npmrc"), "utf8")
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line !== "" && !line.startsWith("#"));
  expect(settings).toContain("ignore-scripts=true");
});
