// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, useState, type JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { RemoteSkipOffer, useFileSegments } from "./cast-segments.tsx";
import type { MediaSegment } from "./segments.tsx";
import { mount, servedBy, settleUntil, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** An episode's segments: a recap, an intro, a commercial, a preview and credits. */
const segments: readonly MediaSegment[] = [
  { type: "recap", startMs: 0, endMs: 40_000 },
  { type: "intro", startMs: 60_000, endMs: 90_000 },
  { type: "commercial", startMs: 300_000, endMs: 330_000 },
  { type: "preview", startMs: 500_000, endMs: 520_000 },
  { type: "credits", startMs: 1_100_000, endMs: 1_200_000 },
];

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

describe("a cast's remote's segments (D546)", () => {
  function Shows(props: { readonly fileId: number }): JSX.Element {
    const read = useFileSegments(props.fileId);
    return <p>{read.map((segment) => `${segment.type}@${String(segment.startMs)}`).join(" ")}</p>;
  }

  it("are the file's, as GET /files/{id}/segments answers", async () => {
    const heard: Heard[] = [];
    const services = servedBy({ "GET /files/4/segments": [200, { segments }] }, heard);
    mounted = await mount(withServices(<Shows fileId={4} />, services));
    await settleUntil(() => mounted?.container.textContent !== "");
    expect(mounted.container.textContent).toBe(
      "recap@0 intro@60000 commercial@300000 preview@500000 credits@1100000",
    );
    expect(heard.map((each) => each.methodAndPath)).toEqual(["GET /files/4/segments"]);
  });

  it("are none where they cannot be read, so nothing is offered", async () => {
    const heard: Heard[] = [];
    const services = servedBy({ "GET /files/5/segments": [500, {}] }, heard);
    mounted = await mount(withServices(<Shows fileId={5} />, services));
    await settleUntil(() => heard.length > 0);
    expect(mounted.container.textContent).toBe("");
  });
});

describe("a cast's remote's skip (D546)", () => {
  /** Moves the mounted remote's position, as the cast's reports do. */
  let moveTo: (atMs: number) => void = () => undefined;

  /** The remote's skip, whose position a test moves (moveTo) in the same root, as a press is kept. */
  function Remote(props: {
    readonly fromMs: number;
    readonly isCreditsLeftToUpNext: boolean;
    readonly skipped: number[];
  }): JSX.Element | null {
    const [atMs, setAtMs] = useState(props.fromMs);
    moveTo = setAtMs;
    return (
      <RemoteSkipOffer
        segments={segments}
        atMs={atMs}
        isCreditsLeftToUpNext={props.isCreditsLeftToUpNext}
        onSkip={(endMs) => {
          props.skipped.push(endMs);
        }}
      />
    );
  }

  /** The remote's skip mounted at fromMs; the segment ends its presses skipped to. */
  async function remoteAt(fromMs: number, isCreditsLeftToUpNext = false): Promise<number[]> {
    const skipped: number[] = [];
    mounted = await mount(
      withServices(
        <Remote fromMs={fromMs} isCreditsLeftToUpNext={isCreditsLeftToUpNext} skipped={skipped} />,
      ),
    );
    return skipped;
  }

  function move(atMs: number): void {
    act(() => {
      moveTo(atMs);
    });
  }

  function button(): HTMLButtonElement | null {
    return mounted?.container.querySelector("button") ?? null;
  }

  it("offers the player's skip words while inside an intro, a recap, a preview or credits", async () => {
    await remoteAt(10_000);
    for (const [atMs, words] of [
      [10_000, "Skip recap"],
      [60_000, "Skip intro"],
      [519_999, "Skip preview"],
      [1_150_000, "Skip credits"],
    ] as const) {
      move(atMs);
      expect(button()?.textContent).toBe(words);
    }
    for (const atMs of [40_000, 59_999, 90_000, 310_000, 520_000]) {
      move(atMs);
      expect(button()).toBe(null);
    }
  });

  it("leaves credits to Up next where it takes their place, still offering the rest", async () => {
    await remoteAt(1_150_000, true);
    expect(button()).toBe(null);
    move(70_000);
    expect(button()?.textContent).toBe("Skip intro");
  });

  it("skips to the segment's end at its press, and hides for that segment until the next", async () => {
    const skipped = await remoteAt(65_000);
    act(() => {
      button()?.click();
    });
    expect(skipped).toEqual([90_000]);
    expect(button()).toBe(null);
    move(80_000);
    expect(button()).toBe(null);
    move(505_000);
    expect(button()?.textContent).toBe("Skip preview");
  });
});
