// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { PosterGrid, type Entry } from "./browse.tsx";
import { bodyField, mount, servedBy, settleUntil, type Heard } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** An IntersectionObserver a test drives: it keeps what it watches, and the test says what shows. */
class HandObserver {
  static all: HandObserver[] = [];
  readonly watched = new Set<Element>();
  constructor(readonly tell: (entries: { target: Element; isIntersecting: boolean }[]) => void) {
    HandObserver.all.push(this);
  }
  observe(element: Element): void {
    this.watched.add(element);
  }
  unobserve(element: Element): void {
    this.watched.delete(element);
  }
  disconnect(): void {
    this.watched.clear();
  }
}

/** Films 1 to 4, each a tile of the grid. */
const films = [1, 2, 3, 4].map((id): Entry => ({
  id,
  kind: "movie",
  name: `Film ${String(id)}`,
  identificationState: "identified",
  addedAt: 1,
}));

/** What the grid said of its tiles: each PUT's item ids, and each DELETE. */
function tilesSaid(heard: readonly Heard[]): string[] {
  return heard
    .filter((request) => request.methodAndPath.endsWith("/tiles/interest"))
    .map((request) =>
      request.methodAndPath.startsWith("PUT")
        ? `PUT ${JSON.stringify(bodyField(request, "itemIds"))}`
        : request.methodAndPath,
    );
}

afterEach(() => {
  HandObserver.all = [];
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("a grid's tiles in view (D362 §4)", () => {
  it("says the tiles in view once they settle, those scrolled to in their place, and none when it goes", async () => {
    vi.stubGlobal("IntersectionObserver", HandObserver);
    vi.useFakeTimers({
      toFake: ["setTimeout", "clearTimeout", "setInterval", "clearInterval"],
      shouldAdvanceTime: true,
    });
    const heard: Heard[] = [];
    const services = servedBy(
      { "PUT /tiles/interest": [204, null], "DELETE /tiles/interest": [204, null] },
      heard,
    );
    const grid = await mount(withServices(<PosterGrid entries={films} />, services));
    const tile = (index: number): Element => {
      const found = grid.container.querySelectorAll(".poster-card")[index];
      if (found === undefined) {
        throw new Error(`no tile ${String(index)}`);
      }
      return found;
    };
    const observer = HandObserver.all.find((each) => each.watched.size === films.length);
    act(() => {
      observer?.tell([
        { target: tile(0), isIntersecting: true },
        { target: tile(1), isIntersecting: true },
      ]);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });
    await settleUntil(() => tilesSaid(heard).length === 1);
    expect(tilesSaid(heard)).toEqual(["PUT [1,2]"]);
    act(() => {
      observer?.tell([
        { target: tile(0), isIntersecting: false },
        { target: tile(2), isIntersecting: true },
      ]);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });
    await settleUntil(() => tilesSaid(heard).length === 2);
    expect(tilesSaid(heard)).toEqual(["PUT [1,2]", "PUT [2,3]"]);
    grid.unmount();
    await settleUntil(() => tilesSaid(heard).at(-1) === "DELETE /tiles/interest");
  });
});
