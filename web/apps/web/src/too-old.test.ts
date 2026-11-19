// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { readFileSync } from "node:fs";
import path from "node:path";
import { runInNewContext } from "node:vm";

import { afterEach, describe, expect, it } from "vitest";

// Imported, not read, so the module graph sees what tests it (the dependency check's no-orphans).
import script from "../public/too-old.js?raw";

/** The page every part loads from (index.html). */
const page = readFileSync(path.join(import.meta.dirname, "../index.html"), "utf8");

const tooOld =
  "This browser is too old for Orpheum. Update it, or use another browser: Chrome, Edge, Firefox or Safari from the last two years.";

/** A page's root as index.html has it: the note for agents, until a script replaces it. */
function rootAsServed(): HTMLElement {
  document.body.innerHTML =
    '<div id="root"><p class="for-agents">An AI agent setting up Orpheum…</p></div>';
  const root = document.getElementById("root");
  if (root === null) {
    throw new Error("no root");
  }
  return root;
}

/** Runs the script as a browser whose Array and String have these methods on their prototypes. */
function runIn(arrays: object, strings: object): void {
  runInNewContext(script, {
    // The browser's own globals, by their own names.
    window: Object.fromEntries([
      ["Array", { prototype: arrays }],
      ["String", { prototype: strings }],
    ]),
    document,
  });
}

const present = (): undefined => undefined;

afterEach(() => {
  document.body.innerHTML = "";
});

// The first beta tester's TV ran Chrome 67, and the page stayed blank (the install report, 2026-11-19).
describe("a browser too old for the client's script", () => {
  it("is told so in the page's root, as Chrome 67 is, with neither method", () => {
    const root = rootAsServed();
    runIn({}, {});
    expect(root.textContent).toBe(tooOld);
    expect(root.querySelector(".for-agents")).toBeNull();
  });

  it("is told so where it has findLast but reads no class static block, as Safari 15.4 to 16.3", () => {
    const root = rootAsServed();
    runIn({ findLast: present }, {});
    expect(root.textContent).toBe(tooOld);
  });

  it("is left alone where it reads the client's script", () => {
    const root = rootAsServed();
    runIn({ findLast: present }, { isWellFormed: present });
    expect(root.querySelector(".for-agents")).not.toBeNull();
    expect(root.textContent).not.toContain("too old");
  });
});

describe("the page's too-old script", () => {
  it("is loaded by the page as a classic script, before the client's module", () => {
    const classic = page.indexOf('<script src="/too-old.js"></script>');
    expect(classic).toBeGreaterThan(page.indexOf('<div id="root">'));
    expect(classic).toBeLessThan(page.indexOf('<script type="module"'));
  });

  it("is written in ES5 alone, which every browser reads", () => {
    const code = script.replaceAll(/\/\*[\s\S]*?\*\/|\/\/.*$/gmu, "");
    expect(code).not.toMatch(/=>|`|\b(?:let|const|class|async|await)\b|\?\.|\?\?|\.\.\./u);
  });
});
