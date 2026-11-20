// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, createRef } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { UpNext } from "./around-file.ts";
import { seekAskedEvent } from "./seek-ahead.ts";
import {
  creditsStartOf,
  SegmentsContext,
  SkipOffer,
  skipShownAloneMs,
  SkipsCommercials,
  type MediaSegment,
} from "./segments.tsx";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { PlayersOffers, UpNextContext, type UpNextOffered } from "./up-next.tsx";

/** A video whose place a test sets (stand) and a seek moves, told by its events. */
function fakeVideo(): HTMLVideoElement & {
  stand: (atS: number) => void;
  readonly sought: number[];
  readonly asked: string[];
} {
  const video = document.createElement("video");
  let currentTime = 0;
  const sought: number[] = [];
  const asked: string[] = [];
  Object.defineProperties(video, {
    currentTime: {
      get: () => currentTime,
      set: (atS: number) => {
        currentTime = atS;
        sought.push(atS);
      },
    },
    duration: { get: () => 1200 },
    ended: { get: () => false },
  });
  video.addEventListener(seekAskedEvent, (event) => {
    asked.push((event as CustomEvent<string>).detail);
  });
  return Object.assign(video, {
    sought,
    asked,
    stand: (atS: number): void => {
      currentTime = atS;
      act(() => {
        video.dispatchEvent(new Event("timeupdate"));
      });
    },
  });
}

/** An episode's segments: a recap, an intro, a preview, credits and a commercial, in seconds. */
const segments: readonly MediaSegment[] = [
  { type: "recap", startMs: 0, endMs: 40_000 },
  { type: "intro", startMs: 60_000, endMs: 90_000 },
  { type: "commercial", startMs: 300_000, endMs: 330_000 },
  { type: "preview", startMs: 500_000, endMs: 520_000 },
  { type: "credits", startMs: 1_100_000, endMs: 1_200_000 },
];

/** The next episode, "Bag", which plays file 4. */
const bag: UpNext = {
  episode: {
    itemId: 23,
    name: "Bag",
    groupNumber: 1,
    numberInGroup: 3,
    fileId: 4,
    isPartway: false,
  },
  ordering: "aired",
};

describe("where a file's credits start (D139)", () => {
  it("is the last credits segment's start, and nowhere without one", () => {
    expect(creditsStartOf(segments)).toBe(1_100_000);
    const twice: MediaSegment[] = [
      { type: "credits", startMs: 1_150_000, endMs: 1_200_000 },
      { type: "credits", startMs: 10_000, endMs: 20_000 },
    ];
    expect(creditsStartOf(twice)).toBe(1_150_000);
    expect(creditsStartOf(segments.filter((segment) => segment.type !== "credits"))).toBe(
      undefined,
    );
    expect(creditsStartOf([])).toBe(undefined);
  });
});

describe("the skip over the film (b1.md Phase C)", () => {
  let mounted: Mounted | undefined;
  let video: ReturnType<typeof fakeVideo>;
  let offered: boolean[];

  beforeEach(() => {
    video = fakeVideo();
    offered = [];
  });

  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.useRealTimers();
  });

  async function skip(isCreditsLeftToUpNext = false): Promise<HTMLElement> {
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", video);
    mounted = await mount(
      <SkipOffer
        videoRef={videoRef}
        segments={segments}
        isCreditsLeftToUpNext={isCreditsLeftToUpNext}
        onOffered={(isOffered) => offered.push(isOffered)}
      />,
    );
    return mounted.container;
  }

  function button(container: HTMLElement): HTMLButtonElement | null {
    return container.querySelector("button.skip-segment");
  }

  it.each([
    ["recap", 20, "Skip recap"],
    ["intro", 60, "Skip intro"],
    ["preview", 519.9, "Skip preview"],
    ["credits", 1150, "Skip credits"],
  ])("shows inside %s, and not outside any skipped segment", async (_, atS, words) => {
    const container = await skip();
    video.stand(atS);
    expect(button(container)?.textContent).toBe(words);
    for (const outside of [40, 59.9, 90, 200, 310, 520, 1099]) {
      video.stand(outside);
      expect(button(container)).toBe(null);
    }
  });

  it("seeks to the segment's end at a press, and hides", async () => {
    const container = await skip();
    video.stand(65);
    act(() => {
      button(container)?.click();
    });
    expect(video.sought).toEqual([90]);
    expect(video.asked).toEqual(["skip-intro"]);
    expect(button(container)).toBe(null);
  });

  it("hides as a seek leaves the segment, before it or past its end, and shows again inside it", async () => {
    const container = await skip();
    video.stand(70);
    expect(button(container)).not.toBe(null);
    video.stand(95);
    expect(button(container)).toBe(null);
    video.stand(70);
    expect(button(container)).not.toBe(null);
    video.stand(50);
    expect(button(container)).toBe(null);
  });

  it("is offered alone for its first seconds, then left to show with the controls", async () => {
    const container = await skip();
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    video.stand(61);
    expect(offered).toEqual([true]);
    act(() => {
      vi.advanceTimersByTime(skipShownAloneMs - 100);
    });
    expect(offered).toEqual([true]);
    act(() => {
      vi.advanceTimersByTime(200);
    });
    expect(offered).toEqual([true, false]);
    expect(button(container)?.textContent).toBe("Skip intro");
    // The next segment is offered alone again.
    video.stand(501);
    expect(offered).toEqual([true, false, true]);
  });

  it("leaves credits to Up next on an episode with a next one", async () => {
    const container = await skip(true);
    video.stand(1150);
    expect(button(container)).toBe(null);
    video.stand(70);
    expect(button(container)?.textContent).toBe("Skip intro");
  });
});

describe("a commercial skipping itself (plans/media-segments.md)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("seeks past each commercial once in the playback: a seek back into it plays it", async () => {
    const video = fakeVideo();
    mounted = await mount(<SkipsCommercials video={video} segments={segments} />);
    video.stand(299);
    expect(video.sought).toEqual([]);
    video.stand(300);
    expect(video.sought).toEqual([330]);
    expect(video.asked).toEqual(["skip-commercial"]);
    video.stand(310);
    video.stand(320);
    expect(video.sought).toEqual([330]);
  });
});

describe("the Play page's offers, one at a time", () => {
  let mounted: Mounted | undefined;
  let video: ReturnType<typeof fakeVideo>;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  async function offers(upNext: UpNextOffered): Promise<HTMLElement> {
    video = fakeVideo();
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", video);
    mounted = await mount(
      withServices(
        <UpNextContext value={upNext}>
          <SegmentsContext value={segments}>
            <PlayersOffers videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />
          </SegmentsContext>
        </UpNextContext>,
      ),
    );
    return mounted.container;
  }

  it("shows Up next at the credits of an episode with a next one, and no Skip credits", async () => {
    const container = await offers({ upNext: bag, isAutoplayOn: false });
    video.stand(70);
    expect(container.textContent).toContain("Skip intro");
    video.stand(1100);
    expect(container.textContent).toContain("Up next");
    expect(container.textContent).not.toContain("Skip credits");
  });

  it("shows Skip credits where nothing is next", async () => {
    const container = await offers({ upNext: undefined, isAutoplayOn: false });
    video.stand(1100);
    expect(container.textContent).toContain("Skip credits");
    expect(container.textContent).not.toContain("Up next");
  });

  it("offers no skip on a cast's receiver", async () => {
    const container = await offers({
      upNext: undefined,
      isAutoplayOn: false,
      receiving: {
        cast: {
          castId: 5,
          membershipCredential: "receiver",
          fileId: 3,
          itemName: "Metropolis",
          positionMs: 61_000,
          fromDeviceId: 2,
          fromDeviceName: "Pixel",
        },
        onLeave: () => undefined,
      },
    });
    video.stand(70);
    expect(container.textContent).not.toContain("Skip");
  });
});
