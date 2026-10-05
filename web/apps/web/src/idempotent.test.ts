// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { idempotent } from "./idempotent.ts";

/** Every source file of the app, as text, tests left out, by its path. */
const sources = import.meta.glob<string>(["./**/*.{ts,tsx}", "!./**/*.test.{ts,tsx}"], {
  query: "?raw",
  import: "default",
  eager: true,
});

/**
 * The POSTs no click sends, which carry no key: the play page asks how a file plays when it
 * opens, and the player reports where it is on its own, each report with its own sequence (D217 §7);
 * a Watch Together viewer's page asks how the group's film plays each time it changes (D246 §3).
 */
const notSentByAClick = new Set(["./play.tsx", "./play-state.ts", "./together-playback.ts"]);

/** The text of each call that begins at an index in source: from it to its closing parenthesis. */
function callAt(source: string, start: number): string {
  let depth = 0;
  for (let index = source.indexOf("(", start); index < source.length; index += 1) {
    if (source[index] === "(") {
      depth += 1;
    } else if (source[index] === ")") {
      depth -= 1;
      if (depth === 0) {
        return source.slice(start, index + 1);
      }
    }
  }
  return source.slice(start);
}

describe("a request a click sends is safe to repeat (D244 §4)", () => {
  it("names the click by its key in the header the server keeps its first answer by", () => {
    expect(idempotent("click-1")).toEqual({ headers: { "Idempotency-Key": "click-1" } });
  });

  it("every POST a click sends carries the click's key", () => {
    const unkeyed: string[] = [];
    for (const [path, source] of Object.entries(sources)) {
      if (notSentByAClick.has(path)) {
        continue;
      }
      for (const match of source.matchAll(/\.POST\(/gu)) {
        const call = callAt(source, match.index);
        if (!call.includes("idempotent(")) {
          unkeyed.push(`${path}: ${call.slice(0, 60)}`);
        }
      }
    }
    expect(unkeyed).toEqual([]);
  });

  it("every exception it makes is still a file that posts", () => {
    for (const path of notSentByAClick) {
      expect(sources[path] ?? "").toContain(".POST(");
    }
  });
});
