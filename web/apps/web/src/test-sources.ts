// SPDX-License-Identifier: MPL-2.0

/**
 * The sources of this directory, web/apps/web/src/, for the tests that read them as text: every
 * stylesheet held to a rule, every component held to the widget set.
 */

import { readdirSync, readFileSync } from "node:fs";

/** This directory, whose sources are read. */
const here = new URL("./", import.meta.url);

/** Each source whose name ends in ending, tests left out, its name and its text. */
export function sourcesHere(ending: string): readonly (readonly [string, string])[] {
  return readdirSync(here)
    .filter((name) => name.endsWith(ending) && !name.includes(".test."))
    .map((name) => [name, readFileSync(new URL(name, here), "utf8")] as const);
}

/** The text of one source of this directory. */
export function sourceHere(name: string): string {
  return readFileSync(new URL(name, here), "utf8");
}

/** A declaration in a stylesheet: its rule's selector, and the at-rules it stands in, outermost first. */
export interface Declaration {
  readonly text: string;
  readonly selector: string;
  readonly within: readonly string[];
}

/** Every declaration of a stylesheet with the at-rules around it; its comments are let go. */
export function declarationsOf(sheet: string): Declaration[] {
  const text = sheet.replaceAll(/\/\*[\s\S]*?\*\//gu, "");
  const found: Declaration[] = [];
  const open: string[] = [];
  let pending = "";
  const declare = (): void => {
    if (pending.trim() !== "") {
      found.push({
        text: pending.trim(),
        selector: open.at(-1)?.replaceAll(/\s+/gu, " ") ?? "",
        within: open.filter((at) => at.startsWith("@")),
      });
    }
    pending = "";
  };
  for (const character of text) {
    if (character === "{") {
      open.push(pending.trim());
      pending = "";
    } else if (character === "}") {
      declare();
      open.pop();
    } else if (character === ";") {
      declare();
    } else {
      pending += character;
    }
  }
  return found;
}
