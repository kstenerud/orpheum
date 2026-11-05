// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { allPages, pageLimitMax, type PageQuery } from "./pages.ts";

describe("allPages", () => {
  it("reads each page by the last one's nextCursor until a page has none (D416 §6)", async () => {
    const asked: PageQuery[] = [];
    const pages: Record<string, { entries: number[]; nextCursor?: string }> = {
      first: { entries: [1, 2], nextCursor: "b" },
      b: { entries: [3, 4], nextCursor: "c" },
      c: { entries: [5] },
    };
    const every = await allPages((query) => {
      asked.push(query);
      return Promise.resolve(pages[query.cursor ?? "first"]!);
    });
    expect(every).toEqual([1, 2, 3, 4, 5]);
    expect(asked).toEqual([
      { limit: pageLimitMax },
      { limit: pageLimitMax, cursor: "b" },
      { limit: pageLimitMax, cursor: "c" },
    ]);
  });

  it("ends with an error, not a loop, at a cursor the server gives twice", async () => {
    await expect(
      allPages(() => Promise.resolve({ entries: [1], nextCursor: "same" })),
    ).rejects.toThrow("twice");
  });
});
