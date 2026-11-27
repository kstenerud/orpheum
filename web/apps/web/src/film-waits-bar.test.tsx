// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { LinkWait, LinkWaitWatch } from "@orpheum/player";
import { act, useRef, type JSX } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  FilmWaitsSpinner,
  linkWaitBarAfterMs,
  linkWaitShare,
  spinnerAfterMs,
} from "./film-waits.tsx";
import { Stage } from "./film-player.tsx";
import type { LadderOffer } from "./moving-down.ts";
import { mount, type Mounted } from "./test-dom.tsx";

/**
 * The bar under the spinner shows how much of a wait's chunks has come over the link, once one has
 * been coming for 3 seconds (the owner, 2026-11-17): none while the server is still making a chunk,
 * whose head has not come, and none once all of it has come.
 */

describe("how much of the spinner's bar is filled", () => {
  const coming = (arrivedBytes: number, linksForMs = linkWaitBarAfterMs): LinkWait => ({
    arrivedBytes,
    neededBytes: 400_000,
    linksForMs,
  });

  it("is no bar while the wait says nothing: a chunk the server is still making", () => {
    expect(linkWaitShare(undefined)).toBeUndefined();
  });

  it("is no bar before the wait has been the link's for 3 seconds; then at its fullest once all of it has come (D506 §4)", () => {
    expect(linkWaitShare(coming(200_000, linkWaitBarAfterMs - 1))).toBeUndefined();
    expect(linkWaitShare(coming(400_000))).toBe(0.9);
  });

  // No answer come: the core answers at once, so the link holds it (the owner, 2026-11-17).
  it("is at its least while no head has said how much is coming (D506 §4)", () => {
    const unmeasured: LinkWait = {
      arrivedBytes: 0,
      neededBytes: 0,
      linksForMs: linkWaitBarAfterMs,
    };
    expect(linkWaitShare(unmeasured)).toBe(0.1);
  });

  it("stays once shown though a chunk's head came since, as a chunk asked again's does (D506 §1)", () => {
    expect(linkWaitShare(coming(200_000, 0), true)).toBe(0.5);
    expect(linkWaitShare(undefined, true)).toBeUndefined();
    // All of it come, the next chunk not yet asked: the film still waits on the link.
    expect(linkWaitShare(coming(400_000), true)).toBe(0.9);
  });

  it("is the bytes come against those needed, between a tenth and nine tenths", () => {
    expect(linkWaitBarAfterMs).toBe(3000);
    expect(linkWaitShare(coming(200_000))).toBe(0.5);
    expect(linkWaitShare(coming(0))).toBe(0.1);
    expect(linkWaitShare(coming(399_000))).toBe(0.9);
  });
});

/** A watch the test says the wait's chunks to, and what the spinner asked of it. */
interface FakeWatching {
  wait: LinkWait | undefined;
  begun: number;
  stopped: number;
  readonly watchLinkWait: () => LinkWaitWatch;
}

function fakeWatching(): FakeWatching {
  const watching: FakeWatching = {
    wait: undefined,
    begun: 0,
    stopped: 0,
    watchLinkWait: () => {
      watching.begun += 1;
      return {
        read: () => watching.wait,
        stop: () => {
          watching.stopped += 1;
        },
      };
    },
  };
  return watching;
}

function WaitingFilm(props: { readonly watching: FakeWatching }): JSX.Element {
  const videoRef = useRef<HTMLVideoElement>(null);
  return (
    <div>
      <video ref={videoRef}>
        <track kind="captions" />
      </video>
      <FilmWaitsSpinner
        videoRef={videoRef}
        isFailed={false}
        watchLinkWait={props.watching.watchLinkWait}
      />
    </div>
  );
}

let mounted: Mounted | undefined;

afterEach(() => {
  vi.useRealTimers();
  mounted?.unmount();
  mounted = undefined;
});

/** A film's offer of its ladder, as the stage's ladder carries it. */
const offer: LadderOffer = {
  url: "/api/v1/bytes/media/chunks/t/master.m3u8",
  startAtBps: 1_896_000,
  qualityKbps: 1200,
  height: 480,
  isBelowTheLowest: false,
  isStartHeldByNodeSpeed: false,
  soundCarriedGainDb: 0,
  isReady: false,
  filmPictureKbps: 9_900,
  linkSharePercent: 70,
  steadiness: 0,
};

/** A film on its ladder on the player's stage, its playback's waits watched by watching. */
function LadderStage(props: { readonly watching: FakeWatching }): JSX.Element {
  const videoRef = useRef<HTMLVideoElement>(null);
  return (
    <Stage
      videoRef={videoRef}
      readLinkKbps={() => undefined}
      failure={undefined}
      hasControls={false}
      isAutoPlay={false}
      ladder={{
        offer,
        positionMs: 0,
        playing: undefined,
        cannotKeepUp: undefined,
        isAheadFull: () => false,
        isAtLowestRung: () => false,
        watchLinkWait: props.watching.watchLinkWait,
        sound: undefined,
        heardLevel: undefined,
        playSound: () => false,
      }}
    />
  );
}

/** Stalls the video in mounted, playing with nothing loaded, on the faked timers. */
function stall(video: HTMLVideoElement): void {
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout", "setInterval", "clearInterval"] });
  Object.defineProperty(video, "paused", { value: false });
  Object.defineProperty(video, "readyState", { value: 0, configurable: true });
  act(() => {
    video.dispatchEvent(new Event("waiting"));
  });
}

/** The video in mounted. */
function videoIn(shown: Mounted): HTMLVideoElement {
  const video = shown.container.querySelector("video");
  if (video === null) {
    throw new Error("the page has no video");
  }
  return video;
}

/** Lets ms pass on the faked timers, and what they start settle. */
async function passMs(ms: number): Promise<void> {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

/** The bar's filler's width, undefined where no bar shows. */
function filled(): string | undefined {
  const fill = mounted?.container.querySelector<HTMLElement>(".film-waits__bar .film-waits__fill");
  return fill?.style.width;
}

describe("the bar under the spinner (the owner, 2026-11-17)", () => {
  it("shows only once a chunk waited on has come for 3 seconds with bytes still to come, and goes with the spinner", async () => {
    const watching = fakeWatching();
    mounted = await mount(<WaitingFilm watching={watching} />);
    const video = videoIn(mounted);
    stall(video);
    await passMs(spinnerAfterMs);
    expect(mounted.container.querySelector(".film-waits__spinner")).not.toBeNull();
    expect(watching.begun).toBe(1); // watched from when the spinner shows
    expect(filled()).toBeUndefined(); // the server is still making the chunk
    watching.wait = { arrivedBytes: 0, neededBytes: 400_000, linksForMs: 1000 };
    await passMs(250);
    expect(filled()).toBeUndefined(); // coming, but not yet for 3 seconds
    watching.wait = { arrivedBytes: 200_000, neededBytes: 400_000, linksForMs: 3000 };
    await passMs(250);
    expect(filled()).toBe("50%");
    expect(mounted.container.querySelector(".film-waits__bar")?.getAttribute("aria-hidden")).toBe(
      "true",
    );
    watching.wait = { arrivedBytes: 300_000, neededBytes: 400_000, linksForMs: 3250 };
    await passMs(250);
    expect(filled()).toBe("75%");
    Object.defineProperty(video, "readyState", { value: 4 });
    act(() => {
      video.dispatchEvent(new Event("playing"));
    });
    expect(filled()).toBeUndefined();
    expect(watching.stopped).toBe(1);
    expect(watching.begun).toBe(1);
  });

  it("once shown, stays as new heads come, and waits its 3 seconds again only after the server held it up (D506 §1)", async () => {
    const watching = fakeWatching();
    mounted = await mount(<WaitingFilm watching={watching} />);
    stall(videoIn(mounted));
    await passMs(spinnerAfterMs);
    watching.wait = { arrivedBytes: 100_000, neededBytes: 400_000, linksForMs: 3000 };
    await passMs(250);
    expect(filled()).toBe("25%");
    watching.wait = { arrivedBytes: 0, neededBytes: 400_000, linksForMs: 100 };
    await passMs(250);
    expect(filled()).toBe("10%"); // a new head since, its bar kept
    watching.wait = undefined; // a chunk the server makes
    await passMs(250);
    expect(filled()).toBeUndefined();
    watching.wait = { arrivedBytes: 0, neededBytes: 400_000, linksForMs: 100 };
    await passMs(250);
    expect(filled()).toBeUndefined(); // the link's again, but not yet for 3 seconds
  });

  it("is watched on the player's stage by its ladder's playback", async () => {
    const watching = fakeWatching();
    watching.wait = { arrivedBytes: 100_000, neededBytes: 400_000, linksForMs: 3000 };
    mounted = await mount(<LadderStage watching={watching} />);
    stall(videoIn(mounted));
    await passMs(spinnerAfterMs);
    // The bar's watch, and the too-slow dialog's, which counts only the link's wait (too-slow-at-lowest.ts).
    expect(watching.begun).toBe(2);
    await passMs(250);
    expect(filled()).toBe("25%");
  });
});
