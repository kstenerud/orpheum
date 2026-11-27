// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { act, type ReactNode } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { CastFrame } from "./cast-frame.tsx";
import type { ScrubPictures } from "./scrub-pictures.tsx";

/** Each sheet the component loads apart, which a test loads when it says. */
const loading: FakeImage[] = [];

/** A sheet of 10 × 10 pictures of 320 × 180. */
const sheetWidth = 3200;

class FakeImage {
  onload: (() => void) | null = null;
  src = "";
  naturalWidth = 0;
  constructor() {
    loading.push(this);
  }
  load(): void {
    this.naturalWidth = sheetWidth;
    act(() => this.onload?.());
  }
}

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

/** Loads the sheet at url, which the component must have asked for. */
function loaded(url: string): void {
  const image = loading.find((each) => each.src === url);
  if (image === undefined) {
    throw new Error(`no sheet ${url} was asked for`);
  }
  image.load();
}

/** A film of 250 pictures every 10 s in sheets of 10 × 10: three sheets, the given ones made. */
function made(...indexes: number[]): ScrubPictures {
  return {
    intervalMs: 10_000,
    pictureWidth: 320,
    pictureHeight: 180,
    columns: 10,
    rows: 10,
    pictureCount: 250,
    sheets: indexes.map((index) => ({ index, url: sheetUrl(index) })),
  };
}

const sheetUrl = (index: number): string => `/api/v1/bytes/trickplay/7/${String(index)}.webp`;

/** The picture drawn: its sheet, and its cut as the sheet's width and offsets in boxes. */
function drawn(): { url: string; width: string; left: string; top: string } | null {
  const image = document.querySelector<HTMLImageElement>(".cast-frame-image");
  if (image === null) {
    return null;
  }
  return {
    url: image.getAttribute("src") ?? "",
    width: image.style.width,
    left: image.style.left,
    top: image.style.top,
  };
}

const shows = (text: string): boolean => document.body.textContent.includes(text);

const asked = (): string[] => loading.map((image) => image.src);

beforeEach(() => {
  loading.length = 0;
  vi.useFakeTimers();
  vi.stubGlobal("Image", FakeImage);
});

afterEach(() => {
  act(() => root?.unmount());
  root = null;
  document.body.replaceChildren();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe("CastFrame (D571)", () => {
  it("shows the still until the place's sheet loads, then the place's picture cut from it", () => {
    // 20:34 is picture 123: sheet 1, its 24th, the 4th across in the 3rd row.
    render(<CastFrame pictures={made(0, 1, 2)} atMs={1_234_000} fallback={<p>the still</p>} />);
    expect(asked()).toEqual([sheetUrl(1)]);
    expect(shows("the still")).toBe(true);
    expect(drawn()).toBeNull();
    loaded(sheetUrl(1));
    expect(shows("the still")).toBe(false);
    expect(drawn()).toEqual({ url: sheetUrl(1), width: "1000%", left: "-300%", top: "-200%" });
  });

  it("moves to the next picture as the place crosses into it, the one before staying while the next sheet loads", () => {
    const pictures = made(0, 1, 2);
    const { rerender } = render(<CastFrame pictures={pictures} atMs={980_000} fallback={null} />);
    loaded(sheetUrl(0));
    expect(drawn()).toEqual({ url: sheetUrl(0), width: "1000%", left: "-800%", top: "-900%" });
    rerender(<CastFrame pictures={pictures} atMs={989_999} fallback={null} />);
    expect(drawn()?.left).toBe("-800%"); // still picture 98
    rerender(<CastFrame pictures={pictures} atMs={990_000} fallback={null} />);
    expect(drawn()).toEqual({ url: sheetUrl(0), width: "1000%", left: "-900%", top: "-900%" });
    rerender(<CastFrame pictures={pictures} atMs={1_000_000} fallback={null} />);
    // Picture 100 is the first of sheet 1, not loaded yet: picture 99 stays.
    expect(drawn()).toEqual({ url: sheetUrl(0), width: "1000%", left: "-900%", top: "-900%" });
    loaded(sheetUrl(1));
    expect(drawn()).toEqual({ url: sheetUrl(1), width: "1000%", left: "0%", top: "0%" });
    expect(asked()).toEqual([sheetUrl(0), sheetUrl(1)]);
  });

  it("shows the still while the film has no pictures, or none made for the place, and the picture once they are", () => {
    const { rerender } = render(
      <CastFrame pictures={undefined} atMs={1_234_000} fallback={<p>the still</p>} />,
    );
    expect(asked()).toEqual([]);
    expect(shows("the still")).toBe(true);
    rerender(<CastFrame pictures={made(0)} atMs={1_234_000} fallback={<p>the still</p>} />);
    expect(asked()).toEqual([]); // sheet 1 not made
    expect(shows("the still")).toBe(true);
    rerender(<CastFrame pictures={made(0, 1)} atMs={1_234_000} fallback={<p>the still</p>} />);
    loaded(sheetUrl(1));
    expect(shows("the still")).toBe(false);
    expect(drawn()?.url).toBe(sheetUrl(1));
  });

  it("asks for nothing but the pictures' sheets, never the key frame the node once made", () => {
    const { rerender } = render(<CastFrame pictures={made(0, 1)} atMs={1_000} fallback={null} />);
    loaded(sheetUrl(0));
    for (const at of [4_000, 70_000, 1_100_000]) {
      rerender(<CastFrame pictures={made(0, 1)} atMs={at} fallback={null} />);
      act(() => {
        vi.advanceTimersByTime(3000);
      });
    }
    expect(asked()).toEqual([sheetUrl(0), sheetUrl(1)]);
    expect(asked().some((url) => url.includes("/frame"))).toBe(false);
  });
});
