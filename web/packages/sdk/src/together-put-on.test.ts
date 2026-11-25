// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { putOnCommands } from "./together-put-on.ts";

describe("a leading player putting its film on for its session (D555 §3)", () => {
  it("puts the film on at its position, as the title it was opened as, paused there where it waits paused", () => {
    expect(putOnCommands({ fileId: 1504 }, 185_000, false)).toEqual([
      { command: "changeItem", fileId: 1504, positionMs: 185_000 },
    ]);
    expect(putOnCommands({ fileId: 1504, itemId: 2020 }, 185_000, true)).toEqual([
      { command: "changeItem", fileId: 1504, itemId: 2020, positionMs: 185_000 },
      { command: "pause" },
    ]);
  });
});
