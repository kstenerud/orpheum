// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, createRef, type JSX } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { Command } from "./input-buttons.ts";
import { commandEventName } from "./input-commands.ts";
import { mount, servedBy, settleUntil, type Heard, type Mounted } from "./test-dom.tsx";
import { forgetAutoplays, stillWatchingAfterAutoplays } from "./still-watching.tsx";
import { testServices, withServices } from "./test-services.tsx";
import {
  offerState,
  upNextCountdownMs,
  upNextLeadMs,
  UpNextOffer,
  useUpNextOffered,
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

describe("what the Play page asks for what plays next", () => {
  it("asks once its video has played, not beside the film's first chunks (the efficiency audit, finding 4)", async () => {
    const heard: Heard[] = [];
    const services = servedBy({ "GET /files/3/up-next": [200, { upNext: bag }] }, heard);
    const video = document.createElement("video");
    let isPaused = true;
    Object.defineProperty(video, "paused", { get: () => isPaused });
    function Offered(): JSX.Element {
      const offered = useUpNextOffered(3, video, { isAutoplayOn: true, receiving: undefined });
      return <p>{offered.upNext?.episode.name ?? "nothing yet"}</p>;
    }
    const page = await mount(withServices(<Offered />, services));
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 20));
    });
    expect(heard).toEqual([]);
    isPaused = false;
    act(() => {
      video.dispatchEvent(new Event("playing"));
    });
    await settleUntil(() => page.container.textContent === "Bag");
    expect(heard.map((request) => request.methodAndPath)).toEqual(["GET /files/3/up-next"]);
    page.unmount();
  });
});

describe("the Up next offer over the film", () => {
  let mounted: Mounted | undefined;
  let played = 0;
  let video: ReturnType<typeof fakeVideo>;

  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    played = 0;
    video = fakeVideo();
    forgetAutoplays();
  });

  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.useRealTimers();
  });

  async function offer(
    isAutoplayOn: boolean,
    isPressable = true,
    services = testServices(),
  ): Promise<HTMLElement> {
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", video);
    mounted = await mount(
      withServices(
        <UpNextOffer
          upNext={bag}
          isAutoplayOn={isAutoplayOn}
          isPressable={isPressable}
          videoRef={videoRef}
          onPlay={() => {
            played += 1;
          }}
        />,
        services,
      ),
    );
    return mounted.container;
  }

  /** Plays an episode to its end and lets autoplay play the next, as count runs. */
  async function autoplayThrough(count: number): Promise<void> {
    for (let each = 0; each < count; each += 1) {
      mounted?.unmount();
      video = fakeVideo();
      await offer(true);
      video.stand(1200, true);
      waitMs(upNextCountdownMs);
    }
    mounted?.unmount();
    mounted = undefined;
    video = fakeVideo();
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

  it("after three episodes autoplayed with no press, asks Still watching? instead of counting (D339)", async () => {
    await autoplayThrough(stillWatchingAfterAutoplays);
    expect(played).toBe(3);
    const page = await offer(true);
    video.stand(1200, true);
    expect(page.textContent).toContain("Still watching?");
    expect(page.textContent).not.toContain("Plays in");
    waitMs(upNextCountdownMs * 2);
    expect(played).toBe(3);
    expect(document.activeElement?.textContent).toBe("Continue");
    act(() => {
      (document.activeElement as HTMLButtonElement).click();
    });
    expect(played).toBe(4);
  });

  it("starts the run of autoplays again at any press while an episode plays (D339)", async () => {
    await autoplayThrough(2);
    await offer(true);
    video.stand(600);
    press("down");
    video.stand(1200, true);
    waitMs(upNextCountdownMs);
    expect(played).toBe(3);
    mounted?.unmount();
    // Two autoplays since the press, one short of the run that asks.
    await autoplayThrough(1);
    const page = await offer(true);
    video.stand(1200, true);
    expect(page.textContent).toContain("Plays in 10 s");
  });

  it("leaves the film at Still watching?'s Stop, playing nothing (D339)", async () => {
    await autoplayThrough(stillWatchingAfterAutoplays);
    const services = testServices();
    const back = vi.spyOn(services.navigation, "back");
    const page = await offer(true, true, services);
    video.stand(1200, true);
    act(() => {
      [...page.querySelectorAll("button")].find((button) => button.textContent === "Stop")?.click();
    });
    expect(back).toHaveBeenCalledOnce();
    expect(played).toBe(3);
    expect(page.textContent).toBe("");
  });

  it("on a cast's receiver, shows and counts with nothing to press, its presses left to take-back", async () => {
    const page = await offer(true, false);
    video.stand(1200, true);
    expect(page.textContent).toContain("Up next: S1E3 · Bag");
    expect(page.querySelector("button")).toBeNull();
    press("down");
    press("back");
    expect(page.textContent).toContain("Plays in 10 s");
    waitMs(upNextCountdownMs);
    expect(played).toBe(1);
  });
});
