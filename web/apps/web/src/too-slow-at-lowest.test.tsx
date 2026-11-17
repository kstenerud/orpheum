// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, useRef, type JSX } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { spinnerAfterMs } from "./film-waits.tsx";
import { Stage } from "./film-player.tsx";
import type { LadderOffer } from "./moving-down.ts";
import { currentNotice, dismissNotice } from "./notice.ts";
import { mount, type Mounted } from "./test-dom.tsx";
import { tooSlowAtLowestAfterMs } from "./too-slow-at-lowest.ts";

/**
 * A film on its ladder that waits for data 10 s on at its lowest quality is told, once in its
 * playback, that it probably will not play on this link, and goes on trying (D506 §3).
 */

/** What the film's words say, the owner's to choose (D506 §3). */
const told =
  "This film probably won't play on this connection: even at its lowest quality, it can't load fast enough. It will keep trying.";

/** A film's offer of its ladder, as the stage's ladder carries it. */
const offer: LadderOffer = {
  url: "/api/v1/bytes/media/chunks/t/master.m3u8",
  startAtBps: 1_896_000,
  qualityKbps: 1200,
  height: 480,
  isBelowTheLowest: false,
  soundCarriedGainDb: 0,
  isReady: false,
  filmPictureKbps: 9_900,
  linkSharePercent: 70,
  steadiness: 0,
};

/** Whether the film's playback loads its lowest quality, as the test says. */
const rung = { isLowest: true };

/** The player's stage, a film on its ladder in it, or, isOwnFile, a film playing its own file. */
function FilmStage(props: { readonly isOwnFile?: boolean }): JSX.Element {
  const videoRef = useRef<HTMLVideoElement>(null);
  const ladder = {
    offer,
    positionMs: 0,
    playing: undefined,
    cannotKeepUp: undefined,
    isAheadFull: () => false,
    isAtLowestRung: () => rung.isLowest,
    watchLinkWait: () => undefined,
    sound: undefined,
    heardLevel: undefined,
    playSound: () => false,
  };
  return (
    <Stage
      videoRef={videoRef}
      readLinkKbps={() => undefined}
      failure={undefined}
      hasControls={false}
      isAutoPlay={false}
      {...(props.isOwnFile === true ? {} : { ladder })}
    />
  );
}

let mounted: Mounted | undefined;

afterEach(() => {
  vi.useRealTimers();
  mounted?.unmount();
  mounted = undefined;
  rung.isLowest = true;
  const notice = currentNotice();
  if (notice !== undefined) {
    dismissNotice(notice);
  }
});

/** The film's stage mounted, its video stalled, playing with nothing loaded, on the faked timers. */
async function stalled(isOwnFile = false): Promise<HTMLVideoElement> {
  mounted = await mount(<FilmStage isOwnFile={isOwnFile} />);
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout", "setInterval", "clearInterval"] });
  const video = mounted.container.querySelector("video");
  if (video === null) {
    throw new Error("the stage has no video");
  }
  Object.defineProperty(video, "paused", { value: false, configurable: true });
  Object.defineProperty(video, "readyState", { value: 0, configurable: true });
  act(() => {
    video.dispatchEvent(new Event("waiting"));
  });
  await passMs(spinnerAfterMs);
  return video;
}

/** Lets ms pass on the faked timers, and what they start settle. */
async function passMs(ms: number): Promise<void> {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

/** The video plays on, enough loaded. */
function plays(video: HTMLVideoElement): void {
  Object.defineProperty(video, "readyState", { value: 4, configurable: true });
  act(() => {
    video.dispatchEvent(new Event("playing"));
  });
}

describe("a film the lowest quality cannot carry (D506 §3)", () => {
  it("is told, once it has waited 10 s at its lowest quality, that it probably won't play here, and goes on trying", async () => {
    expect(tooSlowAtLowestAfterMs).toBe(10_000);
    const video = await stalled();
    await passMs(tooSlowAtLowestAfterMs - 500);
    expect(currentNotice()).toBeUndefined();
    await passMs(500);
    expect(currentNotice()?.text).toBe(told);
    // Nothing stops: the film is still meant to play, its spinner over it.
    expect(video.paused).toBe(false);
    expect(mounted?.container.querySelector(".film-waits__spinner")).not.toBeNull();
  });

  it("counts from nothing while a quality above the lowest loads, and while it plays", async () => {
    rung.isLowest = false;
    const video = await stalled();
    await passMs(2 * tooSlowAtLowestAfterMs);
    expect(currentNotice()).toBeUndefined(); // a lower quality is still to try
    rung.isLowest = true;
    await passMs(tooSlowAtLowestAfterMs - 2000);
    rung.isLowest = false; // a step up, its chunk asked for, then down again
    await passMs(500);
    rung.isLowest = true;
    await passMs(tooSlowAtLowestAfterMs - 2000);
    expect(currentNotice()).toBeUndefined();
    plays(video);
    await passMs(tooSlowAtLowestAfterMs);
    expect(currentNotice()).toBeUndefined(); // it played: the wait ended
    Object.defineProperty(video, "readyState", { value: 0, configurable: true });
    act(() => {
      video.dispatchEvent(new Event("waiting"));
    });
    await passMs(spinnerAfterMs); // the spinner again
    await passMs(tooSlowAtLowestAfterMs - 500);
    expect(currentNotice()).toBeUndefined();
    await passMs(500);
    expect(currentNotice()?.text).toBe(told);
  });

  it("is told once in a playback: closed with its OK, a later wait at the lowest tells nothing", async () => {
    const video = await stalled();
    await passMs(tooSlowAtLowestAfterMs);
    const notice = currentNotice();
    expect(notice?.text).toBe(told);
    if (notice !== undefined) {
      dismissNotice(notice); // its OK (notice-note.tsx)
    }
    plays(video);
    Object.defineProperty(video, "readyState", { value: 0, configurable: true });
    act(() => {
      video.dispatchEvent(new Event("waiting"));
    });
    await passMs(spinnerAfterMs); // the spinner again
    await passMs(3 * tooSlowAtLowestAfterMs);
    expect(currentNotice()).toBeUndefined();
  });

  it("is never told of a film playing its own file, which has no qualities to step down", async () => {
    await stalled(true);
    await passMs(3 * tooSlowAtLowestAfterMs);
    expect(currentNotice()).toBeUndefined();
  });
});
