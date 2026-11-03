// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { ListEntry, ListNote } from "./list-box.tsx";

describe("a list's entries (list-box.tsx)", () => {
  // The 2026-10-05 architecture research's R5: a look a list needs is a variant of its own.
  it("take no class from their user, which tsc refuses", () => {
    // @ts-expect-error -- an entry's look is its own: a variant, never a user's class
    const entry = <ListEntry className="restyled">{null}</ListEntry>;
    // @ts-expect-error -- a note's look beyond its width is its user's, inside it
    const note = <ListNote className="restyled">{null}</ListNote>;
    expect([entry.type, note.type]).toEqual([ListEntry, ListNote]);
  });
});
