// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { CastFrame, castFrameEveryMs, castFrameUrl } from "./cast-frame.tsx";

/** Each image the component loads apart, which a test loads when it says. */
const loading: FakeImage[] = [];

class FakeImage {
  onload: (() => void) | null = null;
  src = "";
  constructor() {
    loading.push(this);
  }
  load(): void {
    act(() => this.onload?.());
  }
}

let visibility: DocumentVisibilityState = "visible";
let root: Root | null = null;

/** Renders tree, and renders it again as each rerender's. */
function render(tree: ReactNode): { rerender: (next: ReactNode) => void } {
  Reflect.set(globalThis, "IS_REACT_ACT_ENVIRONMENT", true);
  const container = document.createElement("div");
  document.body.append(container);
  const mounted = createRoot(container);
  root = mounted;
  act(() => {
    mounted.render(tree);
  });
  return {
    rerender: (next) => {
      act(() => {
        mounted.render(next);
      });
    },
  };
}

/** Loads the index'th image the component asked for. */
function loadedAt(index: number): void {
  const image = loading[index];
  if (image === undefined) {
    throw new Error(`no image ${String(index)} was asked for`);
  }
  image.load();
}

const shows = (text: string): boolean => document.body.textContent.includes(text);

beforeEach(() => {
  loading.length = 0;
  visibility = "visible";
  vi.useFakeTimers();
  vi.stubGlobal("Image", FakeImage);
  vi.spyOn(document, "visibilityState", "get").mockImplementation(() => visibility);
});

afterEach(() => {
  act(() => root?.unmount());
  root = null;
  document.body.replaceChildren();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const asked = (): string[] => loading.map((image) => image.src);

const tick = (): void => {
  act(() => {
    vi.advanceTimersByTime(castFrameEveryMs);
  });
};

describe("CastFrame (D545)", () => {
  it("shows the still until the first frame loads, then the frame, swapped only once each has loaded", () => {
    const { rerender } = render(<CastFrame fileId={7} atMs={60_000} fallback={<p>the still</p>} />);
    expect(asked()).toEqual([castFrameUrl(7, 60_000)]);
    expect(shows("the still")).toBe(true);
    expect(document.querySelector("img")).toBeNull();
    loadedAt(0);
    expect(shows("the still")).toBe(false);
    expect(document.querySelector("img")?.getAttribute("src")).toBe(castFrameUrl(7, 60_000));
    rerender(<CastFrame fileId={7} atMs={63_000} fallback={<p>the still</p>} />);
    tick();
    expect(asked()).toEqual([castFrameUrl(7, 60_000), castFrameUrl(7, 63_000)]);
    // Not loaded yet: the frame before stays.
    expect(document.querySelector("img")?.getAttribute("src")).toBe(castFrameUrl(7, 60_000));
    loadedAt(1);
    expect(document.querySelector("img")?.getAttribute("src")).toBe(castFrameUrl(7, 63_000));
  });

  it("asks once each 3 s while the place moves, and nothing while it is paused", () => {
    const { rerender } = render(<CastFrame fileId={7} atMs={1_000} fallback={null} />);
    tick();
    tick();
    expect(asked()).toHaveLength(1);
    for (const at of [4_000, 7_000, 10_000]) {
      rerender(<CastFrame fileId={7} atMs={at} fallback={null} />);
      tick();
    }
    expect(asked()).toEqual([1_000, 4_000, 7_000, 10_000].map((at) => castFrameUrl(7, at)));
    rerender(<CastFrame fileId={7} atMs={10_500} fallback={null} />);
    act(() => {
      vi.advanceTimersByTime(castFrameEveryMs - 1);
    });
    expect(asked()).toHaveLength(4);
  });

  it("asks nothing while the page is hidden, and asks again once it is shown", () => {
    const { rerender } = render(<CastFrame fileId={7} atMs={1_000} fallback={null} />);
    visibility = "hidden";
    rerender(<CastFrame fileId={7} atMs={4_000} fallback={null} />);
    tick();
    tick();
    expect(asked()).toEqual([castFrameUrl(7, 1_000)]);
    visibility = "visible";
    tick();
    expect(asked()).toEqual([castFrameUrl(7, 1_000), castFrameUrl(7, 4_000)]);
  });
});
