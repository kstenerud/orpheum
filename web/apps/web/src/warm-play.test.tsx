// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { act, createElement, type JSX } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  forgetPlayWarmed,
  useWarmsPlay,
  warmPlayAfterMs,
  warmPlayIdleWithinMs,
  type WarmWays,
} from "./warm-play.ts";

let root: Root | undefined;

beforeEach(() => {
  Reflect.set(globalThis, "IS_REACT_ACT_ENVIRONMENT", true);
  vi.useFakeTimers();
  forgetPlayWarmed();
});

afterEach(() => {
  act(() => {
    root?.unmount();
  });
  root = undefined;
  vi.useRealTimers();
});

/** A page that warms the Play page's code as the home page does. */
function Warming(props: { readonly ways: WarmWays }): JSX.Element {
  useWarmsPlay(props.ways);
  return createElement("p", null, "home");
}

/** Shows a page warming with ways, its Play code's loads counted; returns the count. */
function show(isUnderLoader: boolean): () => number {
  let loads = 0;
  const ways: WarmWays = {
    isUnderLoader: () => isUnderLoader,
    load: async () => {
      loads += 1;
      await Promise.resolve();
    },
  };
  const container = document.createElement("div");
  root = createRoot(container);
  act(() => {
    root?.render(createElement(Warming, { ways }));
  });
  return () => loads;
}

describe("the Play page's code warmed while the person browses (file 5671's first film after a deploy, 2026-10-28)", () => {
  it("is fetched once the page has settled and the browser is idle, under a loader, once per page load", () => {
    const loads = show(true);
    vi.advanceTimersByTime(warmPlayAfterMs - 1);
    expect(loads()).toBe(0); // not while the page's own first requests come
    vi.advanceTimersByTime(1 + warmPlayIdleWithinMs);
    expect(loads()).toBe(1);
    act(() => {
      root?.unmount();
    });
    const again = show(true); // the home page shown again on the same page load
    vi.advanceTimersByTime(warmPlayAfterMs + warmPlayIdleWithinMs);
    expect(again()).toBe(0);
  });

  it("is not fetched by a page reached straight, nor by one left before it settled", () => {
    const straight = show(false);
    vi.advanceTimersByTime(warmPlayAfterMs + warmPlayIdleWithinMs);
    expect(straight()).toBe(0);
    act(() => {
      root?.unmount();
    });
    const left = show(true);
    act(() => {
      root?.unmount();
    });
    root = undefined;
    vi.advanceTimersByTime(warmPlayAfterMs + warmPlayIdleWithinMs);
    expect(left()).toBe(0);
  });
});
