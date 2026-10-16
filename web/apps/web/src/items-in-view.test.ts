// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { act, createElement } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useItemsInView } from "./items-in-view.ts";
import { mount } from "./test-dom.tsx";

/** An IntersectionObserver a test drives: it keeps what it watches, and the test says what shows. */
class HandObserver {
  static latest: HandObserver | undefined;
  readonly watched = new Set<Element>();
  constructor(readonly tell: (entries: { target: Element; isIntersecting: boolean }[]) => void) {
    HandObserver.latest = this;
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

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("useItemsInView", () => {
  it("tells the first and last item in view as they scroll in and out (D306 §7)", async () => {
    vi.stubGlobal("IntersectionObserver", HandObserver);
    const ranges: [number, number][] = [];
    const showRange = (first: number, last: number): void => {
      ranges.push([first, last]);
    };
    function List(): ReturnType<typeof createElement> {
      const itemRef = useItemsInView(showRange);
      return createElement(
        "ol",
        null,
        ["A", "B", "C", "D"].map((name, index) =>
          createElement("li", { key: name, ref: itemRef(index), "data-name": name }, name),
        ),
      );
    }
    const mounted = await mount(createElement(List));
    const observer = HandObserver.latest;
    expect(observer?.watched.size).toBe(4);
    const item = (name: string): Element => {
      const found = mounted.container.querySelector(`[data-name="${name}"]`);
      if (found === null) {
        throw new Error(`no ${name}`);
      }
      return found;
    };
    act(() => {
      observer?.tell([
        { target: item("B"), isIntersecting: true },
        { target: item("C"), isIntersecting: true },
      ]);
    });
    act(() => {
      observer?.tell([
        { target: item("B"), isIntersecting: false },
        { target: item("D"), isIntersecting: true },
      ]);
    });
    expect(ranges).toEqual([
      [1, 2],
      [2, 3],
    ]);
    mounted.unmount();
  });
});
