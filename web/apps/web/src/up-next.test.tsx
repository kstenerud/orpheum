// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, createRef } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Command } from "./input-buttons.ts";
import { commandEventName } from "./input-commands.ts";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import {
  offerState,
  upNextCountdownMs,
  upNextLeadMs,
  UpNextOffer,
  type UpNext,
} from "./up-next.tsx";

/** S1E3, "Bag", which plays file 4. */
const bag: UpNext = {
  episode: {
    titleId: 23,
    name: "Bag",
    groupNumber: 1,
    numberInGroup: 3,
    fileId: 4,
    isPartway: false,
  },
  ordering: "aired",
};

const noChoice = { isDismissed: false, isCancelled: false };

describe("when Up next shows and counts (plans/up-next.md)", () => {
  const durationMs = 1_200_000;

  it("shows within its lead of the end and on the end screen, and not before", () => {
    const at = (atMs: number, hasEnded = false): boolean =>
      offerState({ atMs, durationMs, hasEnded }, noChoice, true).isShown;
    expect(at(durationMs - upNextLeadMs - 1000)).toBe(false);
    expect(at(durationMs - upNextLeadMs + 1000)).toBe(true);
    expect(at(durationMs, true)).toBe(true);
    expect(offerState({ atMs: 0, durationMs: 0, hasEnded: false }, noChoice, true).isShown).toBe(
      false,
    );
  });

  it("counts only once the film has ended, with autoplay on, until a press stops it", () => {
    const nearTheEnd = { atMs: durationMs - 5000, durationMs, hasEnded: false };
    const ended = { atMs: durationMs, durationMs, hasEnded: true };
    expect(offerState(nearTheEnd, noChoice, true).isCounting).toBe(false);
    expect(offerState(ended, noChoice, true).isCounting).toBe(true);
    expect(offerState(ended, noChoice, false)).toMatchObject({ isShown: true, isCounting: false });
    expect(offerState(ended, { ...noChoice, isCancelled: true }, true)).toMatchObject({
      isShown: true,
      isCounting: false,
    });
    expect(offerState(ended, { isDismissed: true, isCancelled: true }, true).isShown).toBe(false);
  });
});

/** A video the offer reads: where it stands, its length and whether it ended, told by its events. */
function fakeVideo(): HTMLVideoElement & { stand: (atS: number, isAtTheEnd?: boolean) => void } {
  const video = document.createElement("video");
  let currentTime = 0;
  let hasEnded = false;
  Object.defineProperties(video, {
    currentTime: { get: () => currentTime },
    duration: { get: () => 1200 },
    ended: { get: () => hasEnded },
  });
  return Object.assign(video, {
    stand: (atS: number, isAtTheEnd = false): void => {
      currentTime = atS;
      hasEnded = isAtTheEnd;
      act(() => {
        video.dispatchEvent(new Event(isAtTheEnd ? "ended" : "timeupdate"));
      });
    },
  });
}

/** Offers command where focus is, as the input layer does. */
function press(command: Command): void {
  const at = document.activeElement ?? document.body;
  act(() => {
    at.dispatchEvent(
      new CustomEvent(commandEventName, {
        detail: { command, source: "key" },
        bubbles: true,
        cancelable: true,
      }),
    );
  });
}

describe("the Up next offer over the film", () => {
  let mounted: Mounted | undefined;
  let played = 0;
  let video: ReturnType<typeof fakeVideo>;

  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    played = 0;
    video = fakeVideo();
  });

  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.useRealTimers();
  });

  async function offer(isAutoplayOn: boolean): Promise<HTMLElement> {
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", video);
    mounted = await mount(
      withServices(
        <UpNextOffer
          upNext={bag}
          isAutoplayOn={isAutoplayOn}
          videoRef={videoRef}
          onPlay={() => {
            played += 1;
          }}
        />,
      ),
    );
    return mounted.container;
  }

  function waitMs(ms: number): void {
    act(() => {
      vi.advanceTimersByTime(ms);
    });
  }

  it("offers the next episode near the end, counts down at the end with autoplay, and plays it", async () => {
    const page = await offer(true);
    video.stand(600);
    expect(page.textContent).toBe("");
    video.stand(1185);
    expect(page.textContent).toContain("Up next: S1E3 · Bag");
    expect(page.textContent).not.toContain("Plays in");
    video.stand(1200, true);
    expect(page.textContent).toContain("Plays in 10 s");
    expect(document.activeElement?.className).toBe("up-next__play");
    press("select"); // OK on the offer is the browser's press of its button, no stop of the count
    waitMs(upNextCountdownMs - 1000);
    expect(page.textContent).toContain("Plays in 1 s");
    expect(played).toBe(0);
    waitMs(1000);
    expect(played).toBe(1);
  });

  it("with autoplay off, stays at the end without counting, focused, and plays the next pressed", async () => {
    const page = await offer(false);
    video.stand(1200, true);
    expect(page.textContent).toContain("Up next: S1E3 · Bag");
    waitMs(upNextCountdownMs * 2);
    expect(page.textContent).not.toContain("Plays in");
    expect(played).toBe(0);
    const focused = document.activeElement;
    expect(focused?.className).toBe("up-next__play");
    act(() => {
      (focused as HTMLButtonElement).click();
    });
    expect(played).toBe(1);
  });

  it("stops counting at any press, leaving the offer, and goes at Back", async () => {
    const page = await offer(true);
    video.stand(1200, true);
    press("down");
    waitMs(upNextCountdownMs * 2);
    expect(played).toBe(0);
    expect(page.textContent).toContain("Up next: S1E3 · Bag");
    expect(page.textContent).not.toContain("Plays in");
    press("back");
    expect(page.textContent).toBe("");
  });

  it("stops counting at a click elsewhere, and plays at a click on it", async () => {
    const page = await offer(true);
    video.stand(1200, true);
    act(() => {
      document.body.dispatchEvent(new Event("pointerdown", { bubbles: true }));
    });
    waitMs(upNextCountdownMs * 2);
    expect(played).toBe(0);
    act(() => {
      page.querySelector<HTMLButtonElement>(".up-next__play")?.click();
    });
    expect(played).toBe(1);
  });

  it("is dismissed by its ×, and comes again once the film is moved back from the end", async () => {
    const page = await offer(true);
    video.stand(1185);
    act(() => {
      page.querySelector<HTMLButtonElement>('button[aria-label="Dismiss Up next"]')?.click();
    });
    expect(page.textContent).toBe("");
    video.stand(600);
    video.stand(1190);
    expect(page.textContent).toContain("Up next: S1E3 · Bag");
  });
});
