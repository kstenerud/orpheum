// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { idempotent, newIdempotencyKey } from "./idempotent.ts";

/** Every source file of the app, as text, tests left out, by its path. */
const sources = import.meta.glob<string>(["./**/*.{ts,tsx}", "!./**/*.test.{ts,tsx}"], {
  query: "?raw",
  import: "default",
  eager: true,
});

/**
 * The POSTs no click sends, which carry no key: a film's page and the play page ask how a file
 * plays when they open (D258 §1), and the player reports where it is on its own, each report with
 * its own sequence (D217 §7); a Watch Together viewer's page asks how the group's film plays each
 * time it changes (D246 §3); and a tab on the WebRTC path offers its connections once signed in,
 * each offer a new connection, so a repeat makes another rather than doing one twice (D273 §8); and
 * a device showing its code asks for it, and how its pairing stands, on its own, asking again giving
 * the same code (D288 §1); a page tells the core of a failure it met on its own, each a report of
 * its own (failure-reports.ts); and the client origin redeems a hand-off's code as it loads, a code
 * the core takes once (D308 §4).
 */
const notSentByAClick = new Set([
  "./watching.ts",
  "./play-state.ts",
  "./together-playback.ts",
  "./transport-choice.ts",
  "./pairing.ts",
  "./failure-reports.ts",
  "./loader-at-client-origin.tsx",
]);

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

  it("makes a click's key where the page is not a secure context, as plain HTTP on a LAN name is", () => {
    const randomUUID = crypto.randomUUID.bind(crypto);
    Object.defineProperty(crypto, "randomUUID", { value: undefined, configurable: true });
    try {
      const first = newIdempotencyKey();
      expect(first).toMatch(/^[0-9a-f]{32}$/u);
      expect(newIdempotencyKey()).not.toBe(first);
    } finally {
      Object.defineProperty(crypto, "randomUUID", { value: randomUUID, configurable: true });
    }
  });

  it("no source makes a key with crypto.randomUUID, which a page on plain HTTP has not", () => {
    const users = Object.entries(sources)
      .filter(([, source]) => source.includes("randomUUID("))
      .map(([path]) => path);
    expect(users).toEqual([]);
  });

  it("every exception it makes is still a file that posts", () => {
    for (const path of notSentByAClick) {
      expect(sources[path] ?? "").toContain(".POST(");
    }
  });
});
