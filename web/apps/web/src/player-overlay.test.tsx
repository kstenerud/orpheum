// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, createRef, useEffect, type JSX } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { commandEventName } from "./input-commands.ts";
import type { OfferPiece, OfferPlace } from "./convenience-area.tsx";
import { PlayUpwardContext } from "./play-up.tsx";
import type { SequenceMoves } from "./player-controls.tsx";
import { PlayerOverlay } from "./player-overlay.tsx";
import { doubleTapWaitMs } from "./player-overlay-state.ts";
import { PlayerTracksContext, type PlayerTracks } from "./player-tracks.tsx";
import { seekAskedEvent, seekRestMs, soughtByOf } from "./seek-ahead.ts";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { clockText } from "./timeline.tsx";

/** The full-screen button, found by its name as a viewer finds it (it takes no class of its own). */
const FULL_SCREEN = 'button:is([aria-label="Full screen"], [aria-label="Leave full screen"])';

/** A Fire TV's Silk, which the overlay takes for a television's remote. */
const silk =
  "Mozilla/5.0 (Linux; Android 9; AFTSSS) AppleWebKit/537.36 (KHTML, like Gecko) Silk/138.19.9 like Chrome/138.0.7204.244 Safari/537.36";

/** Sends a command, as the input layer does, at target; returns whether something took it. */
function press(command: string, target: EventTarget = document.body): boolean {
  const event = new CustomEvent(commandEventName, {
    detail: { command, source: "key" },
    bubbles: true,
    cancelable: true,
  });
  act(() => {
    target.dispatchEvent(event);
  });
  return event.defaultPrevented;
}

/** An offer that shows at once, telling the overlay so, and notes each press of it in presses. */
function offerPiece(presses: string[]): OfferPiece {
  return function Offer(props: OfferPlace): JSX.Element {
    const { onOffered } = props;
    useEffect(() => {
      onOffered?.(true);
      return () => {
        onOffered?.(false);
      };
    }, [onOffered]);
    return (
      <button
        type="button"
        className="offer"
        onClick={() => {
          presses.push("pressed");
        }}
      >
        Up next
      </button>
    );
  };
}

/** A video that says it plays, with play and pause that change what it says. */
function playingVideo(): HTMLVideoElement {
  const video = document.createElement("video");
  let isPaused = false;
  Object.defineProperty(video, "paused", { get: () => isPaused });
  video.pause = () => {
    isPaused = true;
    video.dispatchEvent(new Event("pause"));
  };
  video.play = () => {
    isPaused = false;
    video.dispatchEvent(new Event("play"));
    return Promise.resolve();
  };
  return video;
}

describe("the film's times", () => {
  it("are said as a player says them", () => {
    expect(clockText(0)).toBe("0:00");
    expect(clockText(1_466_000)).toBe("24:26");
    expect(clockText(3_725_000)).toBe("1:02:05");
  });
});

describe("the player's full-screen overlay on a television (D407)", () => {
  let mounted: Mounted | undefined;
  beforeEach(() => {
    vi.spyOn(Navigator.prototype, "userAgent", "get").mockReturnValue(silk);
  });
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  /** Draws the overlay over video in a stage, with offer in its convenience area where given. */
  async function drawn(
    video: HTMLVideoElement,
    offer?: OfferPiece,
    moves?: SequenceMoves,
  ): Promise<Mounted> {
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", video);
    mounted = await mount(
      withServices(
        <div className="film-player__stage">
          <PlayerOverlay
            videoRef={videoRef}
            readLinkKbps={() => undefined}
            failure={undefined}
            offer={offer}
            moves={moves}
          />
        </div>,
      ),
    );
    mounted.container.querySelector(".film-player__stage")?.prepend(video); // where the player draws it
    return mounted;
  }

  /** The offer drawn, its button. */
  function offered(): HTMLButtonElement | null | undefined {
    return mounted?.container.querySelector<HTMLButtonElement>(".offer");
  }

  /** Lets the overlay's time pass while the film plays, which hides it. */
  function hiddenByTime(video: HTMLVideoElement): void {
    act(() => {
      video.dispatchEvent(new Event("play"));
    });
    vi.spyOn(performance, "now").mockReturnValue(1_000_000); // long after the last press
    act(() => {
      vi.advanceTimersByTime(10_000); // well past its time, which hides it
    });
  }

  /** What the overlay shows. */
  function shown(): string | null | undefined {
    return mounted?.container.querySelector(".player-overlay")?.getAttribute("data-shown");
  }

  // D407 §1 reverses the bar that had no rewind or fast-forward (the owner, 2026-10-25), and the
  // on-screen Back goes (Q5).
  it("draws the scrub bar, then ⏪ ▶ ⏩ and full screen, with no Back of its own", async () => {
    await drawn(document.createElement("video"));
    const container = mounted?.container;
    expect(container?.querySelector(".player-overlay__scrub input[type='range']")).not.toBeNull();
    const buttons = [...(container?.querySelectorAll(".player-overlay__buttons button") ?? [])];
    expect(buttons.map((button) => button.getAttribute("aria-label"))).toEqual([
      "Back 10 s",
      "Play",
      "Forward 10 s",
      "Full screen",
    ]);
    expect(container?.querySelector(".back-button")).toBeNull();
    for (const row of container?.querySelectorAll(".player-overlay__row") ?? []) {
      expect(row.hasAttribute("data-focus-container")).toBe(true);
    }
  });

  /** The labels of the overlay's buttons' row, in order. */
  function buttonLabels(): (string | null)[] {
    const buttons = mounted?.container.querySelectorAll(".player-overlay__buttons button") ?? [];
    return [...buttons].map((button) => button.getAttribute("aria-label"));
  }

  // D407 §2: ⏮ and ⏭ only within a sequence, each only where there is one to go to.
  it("draws ⏮ and ⏭ around the steps only where the sequence has them, and plays them pressed", async () => {
    const moved: string[] = [];
    await drawn(document.createElement("video"), undefined, {
      previous: () => moved.push("previous"),
      next: () => moved.push("next"),
    });
    expect(buttonLabels()).toEqual([
      "Previous episode",
      "Back 10 s",
      "Play",
      "Forward 10 s",
      "Next episode",
      "Full screen",
    ]);
    const buttonOf = (label: string): HTMLButtonElement | null | undefined =>
      mounted?.container.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`);
    act(() => {
      buttonOf("Next episode")?.click();
      buttonOf("Previous episode")?.click();
    });
    expect(moved).toEqual(["next", "previous"]);
    mounted?.unmount();
    await drawn(document.createElement("video"), undefined, { next: () => moved.push("next") });
    expect(buttonLabels()).toEqual([
      "Back 10 s",
      "Play",
      "Forward 10 s",
      "Next episode",
      "Full screen",
    ]);
    mounted?.unmount();
    await drawn(document.createElement("video"), undefined, {});
    expect(buttonLabels()).toEqual(["Back 10 s", "Play", "Forward 10 s", "Full screen"]);
  });

  it("says a seek it makes for Left or Right was asked by that key, not by the seeking command it answers with", async () => {
    // A Fire TV's receiver stepped back 10 s no one meant, and the log could not say what asked it
    // (staging, 2026-10-29, file 5680): Right and the remote's fast-forward must read apart.
    const video = document.createElement("video");
    Object.defineProperty(video, "duration", { value: 600 });
    video.currentTime = 100;
    const asked: (string | undefined)[] = [];
    video.addEventListener(seekAskedEvent, (event) => {
      asked.push(soughtByOf(event));
    });
    await drawn(video);
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    press("back"); // hidden, where Left seeks
    expect(press("left")).toBe(true);
    expect(shown()).toBe("scrub");
    act(() => {
      window.dispatchEvent(new KeyboardEvent("keyup", { key: "ArrowLeft" }));
      vi.advanceTimersByTime(seekRestMs);
    });
    expect(asked).toEqual(["left-key"]);
    expect(video.currentTime).toBe(90);
  });

  it("puts the stage full screen from its full-screen button's press, and leaves from it in full screen", async () => {
    // A Fire TV whose player opened out of full screen offered no way into it (the owner, 2026-10-27).
    let fullscreenElement: Element | null = null;
    Object.defineProperty(document, "fullscreenElement", {
      configurable: true,
      get: () => fullscreenElement,
    });
    const left = vi.fn(() => Promise.resolve());
    Object.defineProperty(document, "exitFullscreen", { configurable: true, value: left });
    await drawn(document.createElement("video"));
    const stage = mounted?.container.querySelector<HTMLElement>(".film-player__stage");
    const asked = vi.fn(() => {
      fullscreenElement = stage ?? null;
      document.dispatchEvent(new Event("fullscreenchange"));
      return Promise.resolve();
    });
    if (stage !== null && stage !== undefined) {
      stage.requestFullscreen = asked;
    }
    const button = mounted?.container.querySelector<HTMLButtonElement>(FULL_SCREEN);
    act(() => {
      button?.click();
    });
    expect(asked).toHaveBeenCalledOnce();
    expect(button?.getAttribute("aria-label")).toBe("Leave full screen");
    act(() => {
      button?.click();
    });
    expect(left).toHaveBeenCalledOnce();
    Reflect.deleteProperty(document, "fullscreenElement");
    Reflect.deleteProperty(document, "exitFullscreen");
  });

  it("pauses by OK while hidden, showing the pause sign and the scrub bar, and plays and hides by OK again (D407 §4)", async () => {
    const video = playingVideo();
    await drawn(video);
    press("back");
    expect(shown()).toBe("hidden");
    const sign = mounted?.container.querySelector(".player-overlay-paused");
    expect(sign?.hasAttribute("hidden")).toBe(true);
    expect(press("select")).toBe(true);
    expect(video.paused).toBe(true);
    expect(shown()).toBe("paused");
    expect(sign?.hasAttribute("hidden")).toBe(false);
    expect(press("select")).toBe(true);
    expect(video.paused).toBe(false);
    expect(shown()).toBe("hidden");
  });

  it("hides the pause sign 3 s after the last press, the film still paused", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    let pageNowMs = 0;
    vi.spyOn(performance, "now").mockImplementation(() => pageNowMs);
    const video = playingVideo();
    await drawn(video);
    press("back");
    press("select");
    pageNowMs += 2900;
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(shown()).toBe("paused");
    pageNowMs += 200;
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    expect(shown()).toBe("hidden");
    expect(video.paused).toBe(true);
  });

  it("hides by the first Back while it all shows, focus leaving it for the video, and leaves the next Back to the page", async () => {
    const video = document.createElement("video");
    await drawn(video);
    act(() => {
      mounted?.container.querySelector<HTMLButtonElement>(FULL_SCREEN)?.focus();
    });
    expect(press("back")).toBe(true);
    expect(shown()).toBe("hidden");
    expect(document.activeElement).toBe(video);
    expect(press("back")).toBe(false);
  });

  it("shows all of it by Up while hidden, focus on the scrub bar (D410)", async () => {
    await drawn(document.createElement("video"));
    press("back");
    expect(press("up")).toBe(true);
    expect(shown()).toBe("full");
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Where the film stands");
  });

  // D407 §3 reverses the offer drawn beside the overlay, whose OK the overlay left alone: the offer
  // is the convenience area's, shown alone while the rest is hidden, and OK presses it from anywhere.
  it("shows an offer alone while hidden, its hiding leaving focus on it, and OK presses it with no guard (D407 §3)", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    const video = playingVideo();
    const presses: string[] = [];
    await drawn(video, offerPiece(presses));
    hiddenByTime(video);
    expect(shown()).toBe("convenience");
    expect(document.activeElement).toBe(offered());
    expect(press("select", offered() ?? undefined)).toBe(false); // the offer's own button takes it
    act(() => {
      video.focus();
    });
    expect(press("select", video)).toBe(true);
    expect(presses).toEqual(["pressed"]);
    expect(shown()).toBe("convenience");
  });

  // D407 §4 and Q3 reverse VQ69's answer (Still watching's Continue kept Right): with an offer
  // alone, Left and Right seek and show the scrub bar, the offer keeping focus; OK is the offer's.
  it("seeks by Left and Right with an offer alone, the offer staying and keeping focus, and Up opens it all (Q3)", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    const video = playingVideo();
    await drawn(video, offerPiece([]));
    hiddenByTime(video);
    expect(press("right", offered() ?? undefined)).toBe(true);
    expect(shown()).toBe("scrub");
    expect(document.activeElement).toBe(offered());
    expect(mounted?.container.querySelector(".player-overlay")?.hasAttribute("data-offer")).toBe(
      true,
    );
    expect(press("up", offered() ?? undefined)).toBe(true);
    expect(shown()).toBe("full");
  });

  it("says when the film ends only with all of it shown and nothing on offer", async () => {
    const video = document.createElement("video");
    Object.defineProperty(video, "currentTime", { get: () => 10 });
    Object.defineProperty(video, "duration", { get: () => 3600 });
    await drawn(video, offerPiece([]));
    act(() => {
      video.dispatchEvent(new Event("timeupdate"));
    });
    expect(shown()).toBe("full");
    expect(mounted?.container.textContent).not.toContain("Ends at");
  });

  it("reads where the film stands at each timeupdate only while it shows, at once as it opens, with one clock formatter (the efficiency audit, finding 7)", async () => {
    const formatted = vi.spyOn(Date.prototype, "toLocaleTimeString");
    const video = document.createElement("video");
    let atS = 10;
    Object.defineProperty(video, "currentTime", { get: () => atS });
    Object.defineProperty(video, "duration", { get: () => 3600 });
    await drawn(video);
    const scrub = (): string | null | undefined =>
      mounted?.container.querySelector('input[type="range"]')?.getAttribute("aria-valuetext");
    const timeUpdate = (toS: number): void => {
      atS = toS;
      act(() => {
        video.dispatchEvent(new Event("timeupdate"));
      });
    };
    timeUpdate(20);
    expect(scrub()).toBe("0:20");
    expect(mounted?.container.textContent).toContain("Ends at");
    press("back"); // hides it
    expect(shown()).toBe("hidden");
    timeUpdate(50);
    expect(scrub()).toBe("0:20");
    atS = 70;
    press("up"); // opens it, where the film stands now
    expect(scrub()).toBe("1:10");
    expect(formatted).not.toHaveBeenCalled();
  });

  it("opens at a mouse's move, a click or a tap, as a desktop player does, and hides after its time", async () => {
    // A cast's receiver on a computer opened no controls to its mouse (the owner, 2026-10-27).
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    const video = playingVideo();
    await drawn(video);
    let pageNowMs = 0;
    vi.spyOn(performance, "now").mockImplementation(() => pageNowMs);
    const hideByTime = (): void => {
      pageNowMs += 1_000_000;
      act(() => {
        video.dispatchEvent(new Event("play"));
      });
      act(() => {
        vi.advanceTimersByTime(10_000); // well past its time, which hides it
      });
    };
    const moveTo = (screenX: number, pointerType = "mouse"): void => {
      act(() => {
        document.body.dispatchEvent(
          new PointerEvent("pointermove", { bubbles: true, pointerType, screenX, screenY: 5 }),
        );
      });
    };
    const clickWith = (detail: number): void => {
      act(() => {
        document.body.dispatchEvent(new MouseEvent("click", { bubbles: true, detail }));
      });
    };
    hideByTime();
    expect(shown()).toBe("hidden");
    moveTo(10);
    moveTo(10); // the browser's own move, the pointer where it was
    moveTo(30, "touch");
    clickWith(0); // a key's click, which the overlay hears as the key
    expect(shown()).toBe("hidden");
    moveTo(11);
    expect(shown()).toBe("full");
    expect(document.activeElement).not.toBe(mounted?.container.querySelector("button"));
    hideByTime();
    expect(shown()).toBe("hidden");
    clickWith(1);
    expect(shown()).toBe("full");
  });
});

describe("the player's full-screen overlay on a computer (D407 §6)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  /** Draws the overlay over video in a stage, as the player does. */
  async function drawnOver(video: HTMLVideoElement): Promise<Mounted> {
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", video);
    mounted = await mount(
      withServices(
        <div className="film-player__stage">
          <PlayerOverlay videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />
        </div>,
      ),
    );
    mounted.container.querySelector(".film-player__stage")?.prepend(video);
    return mounted;
  }

  // D407 §10, Q11: the sound is 🔈 first of the trailing group, no mute and slider in the row.
  it("draws 🔈 before full screen, and no mute or slider of the bar's", async () => {
    const on = await drawnOver(playingVideo());
    const buttons = on.container.querySelectorAll(".player-overlay__buttons button");
    const labels = [...buttons].map((button) => button.getAttribute("aria-label"));
    expect(labels.slice(-2)).toEqual(["Volume", "Full screen"]);
    expect(labels).not.toContain("Mute");
    expect(on.container.querySelector(".player-overlay__buttons input[type=range]")).toBeNull();
  });

  it("shows the volume + or − leaves for 1.5 s after the last, leaving the press to the player", async () => {
    const video = playingVideo();
    const on = await drawnOver(video);
    act(() => {
      video.volume = 0.45; // as the player's answer to the press leaves it (player-commands.ts)
      video.dispatchEvent(new Event("volumechange"));
    });
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    const readout = (): HTMLOutputElement | null =>
      on.container.querySelector<HTMLOutputElement>(".player-overlay-volume");
    expect(readout()?.hidden).toBe(true);
    expect(press("volumeUp")).toBe(false);
    expect(readout()?.hidden).toBe(false);
    expect(readout()?.textContent).toBe("Volume 45 %");
    act(() => {
      vi.advanceTimersByTime(1000);
    });
    press("volumeDown");
    act(() => {
      vi.advanceTimersByTime(1499);
    });
    expect(readout()?.hidden).toBe(false);
    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(readout()?.hidden).toBe(true);
  });

  // Q14: focus left on play held a pointer-shown overlay up for good while the film played. The
  // press is play's click whatever made it: OK's, Space's (which a button keeps, player-commands.ts)
  // or a mouse's.
  it("moves focus to the video at a press of play that plays the film, and leaves it on pause's", async () => {
    const video = playingVideo();
    video.pause();
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", video);
    mounted = await mount(
      withServices(
        <div className="film-player__stage">
          <PlayerOverlay videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />
        </div>,
      ),
    );
    mounted.container.querySelector(".film-player__stage")?.prepend(video);
    const play = mounted.container.querySelector<HTMLButtonElement>(
      '.player-overlay__buttons [aria-label="Play"]',
    );
    expect(play).not.toBeNull();
    await act(async () => {
      play?.focus();
      play?.click();
      await Promise.resolve();
    });
    expect(video.paused).toBe(false);
    expect(document.activeElement).toBe(video);
    const pause = mounted.container.querySelector<HTMLButtonElement>(
      '.player-overlay__buttons [aria-label="Pause"]',
    );
    await act(async () => {
      pause?.focus();
      pause?.click();
      await Promise.resolve();
    });
    expect(video.paused).toBe(true);
    expect(document.activeElement).toBe(pause);
  });
});

describe("the overlay's menus on a television (D407 §6, §8)", () => {
  let mounted: Mounted | undefined;
  beforeEach(() => {
    vi.spyOn(Navigator.prototype, "userAgent", "get").mockReturnValue(silk);
  });
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.restoreAllMocks();
  });

  /** A film with no subtitles and one audio track, its switches off. */
  const tracks: PlayerTracks = {
    subtitles: {
      tracks: [],
      chosenTrackKey: undefined,
      choose: () => undefined,
      offsetMs: 0,
      shiftOffset: () => undefined,
      failure: undefined,
    },
    audio: { tracks: [], chosenStreamId: undefined, choose: () => undefined, problem: undefined },
    levelling: { isOn: false, switchTo: () => undefined, problem: undefined },
    level: { levelling: { isMeasured: true, gainDb: 0 }, carriedGainDb: 0 },
    measuring: undefined,
    boost: undefined,
  };

  /** Draws the overlay over a playing video on a Play page, with ⁝ open. */
  async function drawnWithMoreOpen(): Promise<Mounted> {
    const video = playingVideo();
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", video);
    mounted = await mount(
      withServices(
        <PlayUpwardContext value={{ crumbs: [], here: "Film" }}>
          <PlayerTracksContext value={tracks}>
            <div className="film-player__stage">
              <PlayerOverlay
                videoRef={videoRef}
                readLinkKbps={() => undefined}
                failure={undefined}
              />
            </div>
          </PlayerTracksContext>
        </PlayUpwardContext>,
      ),
    );
    mounted.container.querySelector(".film-player__stage")?.prepend(video);
    const more = mounted.container.querySelector("button[aria-label='More']");
    act(() => {
      more?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    return mounted;
  }

  it("draws 💬 and ⁝ before full screen, no 🎧 for one audio track", async () => {
    const on = await drawnWithMoreOpen();
    const names = [
      ...on.container.querySelectorAll(
        ".player-controls__trailing > * button, .player-controls__trailing > button",
      ),
    ]
      .filter((button) => button.closest("dialog") === null)
      .map((button) => button.getAttribute("aria-label"));
    expect(names).toEqual(["Subtitles: Off", "More", "Full screen"]);
  });

  it("leaves Back to the open menu, which closes, the overlay staying", async () => {
    const on = await drawnWithMoreOpen();
    expect(on.container.querySelector("dialog")).not.toBeNull();
    const isTaken = press("back", document.activeElement ?? document.body);
    expect(isTaken).toBe(true);
    expect(on.container.querySelector("dialog")).toBeNull();
    expect(on.container.querySelector(".player-overlay")?.getAttribute("data-shown")).toBe("full");
  });

  it("closes the menu as full screen changes, focus going on to the scrub bar", async () => {
    const on = await drawnWithMoreOpen();
    expect(on.container.querySelector("dialog")).not.toBeNull();
    act(() => {
      document.dispatchEvent(new Event("fullscreenchange"));
    });
    expect(on.container.querySelector("dialog")).toBeNull();
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Where the film stands");
  });
});

// The owner, 2026-11-04: "For the tap thing: agreed", and a shown overlay must stay up as paused
// playback always does, however it was hidden before (D415).
describe("a finger's taps on the full-screen overlay's picture (D415)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  /** Draws the overlay over video, then takes the clock, which mounting needs running. */
  async function drawnOver(video: HTMLVideoElement): Promise<void> {
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", video);
    mounted = await mount(
      withServices(
        <div className="film-player__stage">
          <PlayerOverlay videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />
        </div>,
      ),
    );
    mounted.container.querySelector(".film-player__stage")?.prepend(video);
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout", "setInterval", "clearInterval"] });
  }

  function shown(): string | null | undefined {
    return mounted?.container.querySelector(".player-overlay")?.getAttribute("data-shown");
  }

  const left = 100;
  const middle = 500;
  const right = 900;

  /** A finger's tap at x on target (the picture by default), the click the browser sends after. */
  function tap(x: number, target?: Element): void {
    const at = target ?? mounted?.container.querySelector("video") ?? document.body;
    const init = { bubbles: true, pointerType: "touch", pointerId: 7, clientX: x, clientY: 50 };
    act(() => {
      at.dispatchEvent(new PointerEvent("pointerdown", init));
      at.dispatchEvent(new PointerEvent("pointerup", init));
      at.dispatchEvent(new MouseEvent("click", { bubbles: true, detail: 1, clientX: x }));
    });
  }

  function wait(ms: number): void {
    act(() => {
      vi.advanceTimersByTime(ms);
    });
  }

  function pausedVideo(): HTMLVideoElement {
    const video = playingVideo();
    video.pause();
    return video;
  }

  it("hides the overlay and shows it again, a paused film's staying up after, with no wait in the middle", async () => {
    await drawnOver(pausedVideo());
    expect(shown()).toBe("full");
    tap(middle);
    expect(shown()).toBe("hidden");
    tap(middle);
    expect(shown()).toBe("full");
    wait(60_000); // a paused film's overlay never hides by itself (D407 §6)
    expect(shown()).toBe("full");
  });

  it("hides a playing film's overlay and shows it again, which hides on its time", async () => {
    await drawnOver(playingVideo());
    tap(middle);
    expect(shown()).toBe("hidden");
    tap(middle);
    expect(shown()).toBe("full");
    let pageNowMs = 0;
    vi.spyOn(performance, "now").mockImplementation(() => pageNowMs + 1000);
    pageNowMs = 1_000_000; // well past its time
    wait(10_000);
    expect(shown()).toBe("hidden");
  });

  it("leaves a tap on a control to the control", async () => {
    await drawnOver(pausedVideo());
    const button = mounted?.container.querySelector(".player-overlay button") ?? undefined;
    tap(middle, button);
    expect(shown()).toBe("full");
  });

  it("waits on a side for a second tap, then hides", async () => {
    await drawnOver(pausedVideo());
    tap(left);
    expect(shown()).toBe("full");
    wait(doubleTapWaitMs);
    expect(shown()).toBe("hidden");
  });

  it("seeks 10 s back or forward on a double tap, stacking, with no show or hide for its taps", async () => {
    const video = pausedVideo();
    Object.defineProperty(video, "duration", { get: () => 600 });
    let atS = 100;
    Object.defineProperty(video, "currentTime", {
      get: () => atS,
      set: (to: number) => {
        atS = to;
      },
    });
    await drawnOver(video);
    tap(right);
    tap(right);
    expect(atS).toBe(110);
    tap(right);
    expect(atS).toBe(120);
    wait(doubleTapWaitMs * 4);
    expect(shown()).toBe("full");
    tap(left);
    tap(left);
    expect(atS).toBe(110);
    wait(doubleTapWaitMs * 4);
    expect(shown()).toBe("full");
  });

  it("has no double-tap seek in the middle: two taps there hide and show", async () => {
    const video = pausedVideo();
    let atS = 100;
    Object.defineProperty(video, "currentTime", {
      get: () => atS,
      set: (to: number) => {
        atS = to;
      },
    });
    await drawnOver(video);
    tap(middle);
    tap(middle);
    expect(atS).toBe(100);
    expect(shown()).toBe("full");
  });

  it("leaves a mouse's click as it was: it shows the overlay and never hides it", async () => {
    await drawnOver(pausedVideo());
    press("back"); // a key hides it
    expect(shown()).toBe("hidden");
    const video = mounted?.container.querySelector("video");
    act(() => {
      video?.dispatchEvent(new MouseEvent("click", { bubbles: true, detail: 1 }));
    });
    expect(shown()).toBe("full");
  });
});
