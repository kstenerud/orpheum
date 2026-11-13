// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { playerSkippedEvent } from "@orpheum/player";
import { act, createRef, useEffect, type JSX } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { OfferPiece, OfferPlace } from "./convenience-area.tsx";
import { setFillsWindow } from "./film-fullscreen.ts";
import { arrowsAreCommandsAttribute } from "./focus-engine.ts";
import {
  commandEventName,
  commandReleaseEventName,
  type CommandRelease,
} from "./input-commands.ts";
import { OverTheFilm, PageControls } from "./page-controls.tsx";
import { PlayUpwardContext } from "./play-up.tsx";
import {
  answerPlayerCommand,
  markPlayersControls,
  playerControlsAttribute,
} from "./player-commands.ts";
import { seekRestMs, seekStepMs } from "./seek-ahead.ts";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import type { Upward } from "./upward.ts";

/** A Fire TV's Silk, which the player takes for a television's remote. */
const silk =
  "Mozilla/5.0 (Linux; Android 9; AFTSSS) AppleWebKit/537.36 (KHTML, like Gecko) Silk/138.19.9 like Chrome/138.0.7204.244 Safari/537.36";

/** A Play page's place: under the home page, which its Up goes to. */
const upward: Upward = {
  crumbs: [{ label: "Home", to: { screen: "home" }, isIt: () => false }],
  here: "Film",
};

/** A video, playing or paused as isPlaying says, and its ref. */
function stagedVideo(isPlaying: () => boolean): {
  readonly video: HTMLVideoElement;
  readonly videoRef: React.RefObject<HTMLVideoElement | null>;
} {
  const video = document.createElement("video");
  Object.defineProperty(video, "paused", { configurable: true, get: () => !isPlaying() });
  const videoRef = createRef<HTMLVideoElement>();
  Reflect.set(videoRef, "current", video);
  return { video, videoRef };
}

/** A command offered at target, as the input layer offers one; whether something took it. */
function offer(target: Element, command: string): boolean {
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

/** An offer that is on as soon as it is drawn, a button named "Up next", as up-next.tsx's is. */
function OfferedAtOnce(props: OfferPlace): JSX.Element {
  const { onOffered } = props;
  useEffect(() => {
    onOffered?.(true);
  }, [onOffered]);
  return (
    <button type="button" className="up-next">
      Up next
    </button>
  );
}

/** The names of the buttons in element, in order. */
function buttonNames(element: Element | null | undefined): (string | null)[] {
  return [...(element?.querySelectorAll('button, [role="button"]') ?? [])].map(
    (each) => each.getAttribute("aria-label") ?? each.textContent,
  );
}

describe("the player's controls under the film outside full screen (D369 §2, D407 §9)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  /**
   * The Play page's pieces as the film player draws them: what is over the film in its stage, then
   * the controls under it; the stage holding the video as the player's does.
   */
  async function drawn(
    video: HTMLVideoElement,
    videoRef: React.RefObject<HTMLVideoElement | null>,
    offered?: OfferPiece,
  ): Promise<HTMLElement> {
    const film = { videoRef, readLinkKbps: () => undefined, failure: undefined };
    mounted = await mount(
      withServices(
        <PlayUpwardContext value={upward}>
          <div className="film-player__stage">
            <OverTheFilm {...film} hasPressesOnTheFilm offer={offered} />
          </div>
          <PageControls {...film} offer={offered} />
        </PlayUpwardContext>,
      ),
    );
    const stage = mounted.container.querySelector<HTMLElement>(".film-player__stage");
    if (stage === null) {
      throw new Error("no stage");
    }
    stage.prepend(video);
    return stage;
  }

  /** The bar under the film. */
  function bar(): HTMLElement | null | undefined {
    return mounted?.container.querySelector<HTMLElement>(".page-controls");
  }

  it("is the player's bar under the film, nothing over it: the timeline, ⏪ ▶ ⏩, sound and full screen, a stop for the arrows", async () => {
    const { video, videoRef } = stagedVideo(() => false);
    const stage = await drawn(video, videoRef);
    expect([...stage.children]).toEqual([video]);
    expect(stage.contains(bar() ?? null)).toBe(false);
    expect(bar()?.hasAttribute(playerControlsAttribute)).toBe(true);
    expect(buttonNames(bar())).toEqual([
      "Back 10 s",
      "Play",
      "Forward 10 s",
      "Volume",
      "Full screen",
    ]);
    const timeline = bar()?.querySelector('input[aria-label="Where the film stands"]');
    expect(timeline?.hasAttribute(arrowsAreCommandsAttribute)).toBe(true);
    expect(video.tabIndex).toBe(0);
    expect(bar()?.querySelector(".player-overlay__up")).toBeNull(); // a computer's trail is the page's
  });

  it("puts the video's stage full screen by its ⤢, though the bar lies outside the stage", async () => {
    const { video, videoRef } = stagedVideo(() => false);
    const stage = await drawn(video, videoRef);
    const asked = vi.fn(() => Promise.resolve());
    stage.requestFullscreen = asked;
    act(() => {
      bar()?.querySelector<HTMLButtonElement>('button[aria-label="Full screen"]')?.click();
    });
    expect(asked).toHaveBeenCalledOnce();
  });

  // A held Right seeked at each repeat, and the marker moved only as a seek landed, after a click
  // too (the owner, 2026-10-27 and 2026-10-29).
  it("moves its marker and time at once, for a held key without seeking until it is released, and keeps them there until the seek lands", async () => {
    const { video, videoRef } = stagedVideo(() => false);
    let positionS = 30;
    const seeks: number[] = [];
    Object.defineProperty(video, "duration", { configurable: true, value: 600 });
    Object.defineProperty(video, "currentTime", {
      configurable: true,
      get: () => positionS,
      set: (toS: number) => {
        seeks.push(toS); // a seek on its way: the film stays where it was until it lands
      },
    });
    await drawn(video, videoRef);
    const timeline = bar()?.querySelector<HTMLInputElement>(".timeline__range");
    const shown = (): string | undefined => bar()?.querySelector(".timeline__times")?.textContent;
    expect(shown()).toBe("0:30 / 10:00");
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout", "performance"] });
    act(() => {
      // The press's 10 s, then a second held at 20 s a second (seek-ahead.ts's heldSeekSpeeds).
      for (let repeat = 0; repeat < 3; repeat += 1) {
        answerPlayerCommand(video, "right", {
          source: "key",
          hold: repeat === 0 ? "start" : "continue",
        });
        vi.advanceTimersByTime(repeat < 2 ? 500 : 0);
      }
    });
    expect(shown()).toBe("1:00 / 10:00");
    expect(timeline?.value).toBe("60000");
    expect(seeks).toEqual([]);
    act(() => {
      const detail: CommandRelease = { command: "right", source: "key" };
      window.dispatchEvent(new CustomEvent(commandReleaseEventName, { detail }));
      vi.advanceTimersByTime(seekRestMs);
    });
    expect(seeks).toEqual([60]); // once, to the place shown
    act(() => {
      video.dispatchEvent(new Event("timeupdate"));
    });
    expect(shown()).toBe("1:00 / 10:00");
    act(() => {
      positionS = 60;
      video.dispatchEvent(new Event("seeked"));
    });
    expect(shown()).toBe("1:00 / 10:00");
    act(() => {
      if (timeline !== null && timeline !== undefined) {
        Reflect.set(HTMLInputElement.prototype, "value", "300000", timeline);
        timeline.dispatchEvent(new Event("input", { bubbles: true }));
      }
    });
    expect(seeks).toEqual([60, 300]); // a click seeks at once
    expect(shown()).toBe("5:00 / 10:00");
    act(() => {
      video.dispatchEvent(new Event("timeupdate"));
    });
    expect(shown()).toBe("5:00 / 10:00"); // not back to 1:00 while the seek loads
  });

  it("plays or pauses at a mouse's click on the film, and puts it full screen at a double click", async () => {
    const { video, videoRef } = stagedVideo(() => false);
    const stage = await drawn(video, videoRef);
    const play = vi.spyOn(video, "play").mockResolvedValue();
    const asked = vi.fn(() => Promise.resolve());
    stage.requestFullscreen = asked;
    act(() => {
      video.dispatchEvent(
        new PointerEvent("click", { bubbles: true, detail: 1, pointerType: "mouse" }),
      );
    });
    expect(play).toHaveBeenCalledOnce();
    act(() => {
      video.dispatchEvent(
        new PointerEvent("click", { bubbles: true, detail: 1, pointerType: "touch" }),
      );
      video.dispatchEvent(new MouseEvent("click", { bubbles: true, detail: 0 })); // a key's
    });
    expect(play).toHaveBeenCalledOnce(); // a tap is not a mouse's click, and a key's click is the key's
    act(() => {
      video.dispatchEvent(new MouseEvent("dblclick", { bubbles: true, detail: 2 }));
    });
    expect(asked).toHaveBeenCalledOnce();
  });

  // D407 §9 replaces the bar over the film that hid after its time (D369 §2): under the film it
  // hides nothing, so it stays and reads where the film stands as it plays.
  it("stays while the film plays, long after anyone was seen, its time following the film", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    const { video, videoRef } = stagedVideo(() => true);
    let atS = 10;
    Object.defineProperty(video, "currentTime", { get: () => atS });
    Object.defineProperty(video, "duration", { get: () => 3600 });
    await drawn(video, videoRef);
    vi.spyOn(performance, "now").mockReturnValue(1_000_000); // long after anyone was seen
    act(() => {
      video.dispatchEvent(new Event("play"));
      vi.advanceTimersByTime(10_000);
    });
    atS = 80;
    act(() => {
      video.dispatchEvent(new Event("timeupdate"));
    });
    expect(bar()?.hasAttribute("data-shown")).toBe(false);
    expect(
      bar()?.querySelector<HTMLInputElement>(".timeline__range")?.getAttribute("aria-valuetext"),
    ).toBe("1:20");
  });

  // D407 §1, §5, §9: in full screen the controls are the overlay over the film, and leaving full
  // screen, unasked as a Fire TV's Back or a computer's Escape leaves it, brings the bar back under
  // the film with focus on play. A stage filling the window counts.
  it("is the full-screen overlay in the stage while full screen, and the bar under the film after, focus on play", async () => {
    const { video, videoRef } = stagedVideo(() => true);
    const stage = await drawn(video, videoRef);
    act(() => {
      setFillsWindow(stage, true);
    });
    expect(stage.querySelector(".player-overlay")).not.toBeNull();
    expect(bar()).toBeNull();
    act(() => {
      setFillsWindow(stage, false);
    });
    expect([...stage.children]).toEqual([video]);
    expect(document.activeElement).toBe(bar()?.querySelector('button[aria-label="Pause"]'));
  });

  /** The player's word that it stepped over the film from fromMs to toMs (skip-unmade.ts). */
  function skipped(video: HTMLVideoElement, fromMs: number, toMs: number): void {
    act(() => {
      video.dispatchEvent(new CustomEvent(playerSkippedEvent, { detail: { fromMs, toMs } }));
    });
  }

  it("put the warning sign in the bar once a stretch the server could not make is skipped, and keep it (toolkit.md item 12)", async () => {
    const { video, videoRef } = stagedVideo(() => true);
    await drawn(video, videoRef);
    const sign = (): Element | null | undefined =>
      bar()?.querySelector('button[aria-label="What is wrong: Part of the film was skipped"]');
    expect(sign()).toBeNull();
    skipped(video, 2_530_000, 2_532_300);
    expect(sign()).not.toBeNull();
    expect(document.querySelector("dialog")).toBeNull();
    act(() => {
      video.dispatchEvent(new Event("timeupdate"));
    });
    expect(sign()).not.toBeNull();
  });

  it("tell a stretch longer than 15 s in a pop-up too, in full screen as well, once", async () => {
    const showModal = vi.spyOn(HTMLDialogElement.prototype, "showModal");
    const { video, videoRef } = stagedVideo(() => true);
    const stage = await drawn(video, videoRef);
    act(() => {
      setFillsWindow(stage, true);
    });
    skipped(video, 2_530_000, 2_544_000);
    expect(stage.querySelector("dialog")).toBeNull();
    skipped(video, 2_530_000, 2_548_600);
    const dialog = stage.querySelector("dialog");
    expect(dialog?.getAttribute("role")).toBe("alertdialog");
    expect(dialog?.textContent).toContain("42:10–42:28");
    expect(showModal).toHaveBeenCalled();
    act(() => {
      dialog?.querySelector<HTMLButtonElement>('button[aria-label="Close"]')?.click();
    });
    skipped(video, 2_530_000, 2_550_900);
    expect(stage.querySelector("dialog")).toBeNull();
    act(() => {
      video.dispatchEvent(
        new CustomEvent(playerSkippedEvent, {
          detail: { fromMs: 2_530_000, toMs: 2_651_900, isWhereItStops: true },
        }),
      );
    });
    expect(stage.querySelector("dialog")?.getAttribute("aria-label")).toBe("The film cannot go on");
  });

  it("tell in a pop-up that the film cannot go on, where the player could not step over what the server could not make", async () => {
    const { video, videoRef } = stagedVideo(() => true);
    const stage = await drawn(video, videoRef);
    act(() => {
      video.dispatchEvent(
        new CustomEvent(playerSkippedEvent, {
          detail: { fromMs: 2_530_000, toMs: 2_532_300, isWhereItStops: true },
        }),
      );
    });
    const dialog = stage.querySelector("dialog");
    expect(dialog?.getAttribute("aria-label")).toBe("The film cannot go on");
    expect(dialog?.textContent).toContain("from 42:10 on");
  });

  it("takes Down on the film into the bar, to play, and leaves Down elsewhere to the page", async () => {
    const { video, videoRef } = stagedVideo(() => false);
    await drawn(video, videoRef);
    const playButton = bar()?.querySelector<HTMLButtonElement>('button[aria-label="Play"]');
    act(() => {
      video.focus();
    });
    expect(offer(video, "down")).toBe(true);
    expect(document.activeElement).toBe(playButton);
    expect(offer(playButton ?? document.body, "down")).toBe(false);
  });

  it("draws an offer among its buttons, and when the film ends while nothing is on offer", async () => {
    const { video, videoRef } = stagedVideo(() => true);
    const stage = await drawn(video, videoRef, OfferedAtOnce);
    expect([...stage.children]).toEqual([video]);
    expect(buttonNames(bar()?.querySelector(".page-controls__offers"))).toEqual(["Up next"]);
    expect(bar()?.querySelector(".convenience-area__ends")).toBeNull();
    mounted?.unmount();
    const other = stagedVideo(() => true);
    await drawn(other.video, other.videoRef);
    expect(bar()?.querySelector(".page-controls__offers .convenience-area__ends")).not.toBeNull();
  });

  it("carries a television's ‹, its one way up, among the offers", async () => {
    vi.spyOn(Navigator.prototype, "userAgent", "get").mockReturnValue(silk);
    const { video, videoRef } = stagedVideo(() => false);
    await drawn(video, videoRef);
    expect(bar()?.querySelector(".page-controls__offers .player-overlay__up")).not.toBeNull();
  });
});

describe("Left and Right on the bar's timeline (D369 §2)", () => {
  afterEach(() => {
    document.body.replaceChildren();
    vi.useRealTimers();
  });

  /** A video and, beside its stage as the bar under the film is, controls with two ranges. */
  function laidOut(): {
    readonly video: HTMLVideoElement;
    readonly controls: HTMLElement;
    readonly timeline: HTMLInputElement;
    readonly volume: HTMLInputElement;
  } {
    const stage = document.createElement("div");
    const video = document.createElement("video");
    Object.defineProperty(video, "duration", { configurable: true, value: 600 });
    video.currentTime = 30;
    const controls = document.createElement("div");
    controls.setAttribute(playerControlsAttribute, "");
    const timeline = document.createElement("input");
    timeline.type = "range";
    timeline.setAttribute(arrowsAreCommandsAttribute, "");
    const volume = document.createElement("input");
    volume.type = "range";
    controls.append(timeline, volume);
    stage.append(video);
    document.body.append(stage, controls);
    return { video, controls, timeline, volume };
  }

  it("seek the film a step, as on the film itself, and nowhere else does a range's arrow", () => {
    const { video, controls, timeline, volume } = laidOut();
    markPlayersControls(controls, { current: video });
    timeline.focus();
    vi.useFakeTimers();
    expect(answerPlayerCommand(video, "right")).toBe(true);
    vi.advanceTimersByTime(seekRestMs);
    expect(video.currentTime).toBe(30 + seekStepMs / 1000);
    volume.focus();
    expect(answerPlayerCommand(video, "left")).toBe(false);
    vi.advanceTimersByTime(seekRestMs);
    expect(video.currentTime).toBe(30 + seekStepMs / 1000);
  });

  // plans/player-overlay.md §6: the bar under the film is outside the stage, so controls are the
  // player's by the video they name, not by where they lie.
  it("are the player's only on controls marked as its own for that video", () => {
    const { video, controls, timeline } = laidOut();
    timeline.focus();
    expect(answerPlayerCommand(video, "right")).toBe(false);
    const unmark = markPlayersControls(controls, { current: document.createElement("video") });
    expect(answerPlayerCommand(video, "right")).toBe(false);
    unmark();
    markPlayersControls(controls, { current: video });
    expect(answerPlayerCommand(video, "right")).toBe(true);
  });
});

describe("a television's player opening full screen (D338 §5)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.restoreAllMocks();
  });

  // The owner's TCL TV went full screen as the player opened, which used up the press, and the start
  // gate's play 3 s later was refused (staging, 2026-11-06).
  it("plays its video, and pauses it, under the press before full screen uses the press up", async () => {
    const steps: string[] = [];
    vi.spyOn(HTMLMediaElement.prototype, "play").mockImplementation(() => {
      steps.push("play");
      return Promise.resolve();
    });
    vi.spyOn(HTMLMediaElement.prototype, "pause").mockImplementation(() => {
      steps.push("pause");
    });
    Object.defineProperty(HTMLElement.prototype, "requestFullscreen", {
      configurable: true,
      value: () => {
        steps.push("fullScreen");
        return Promise.resolve();
      },
    });
    const videoRef = createRef<HTMLVideoElement>();
    const film = { videoRef, readLinkKbps: () => undefined, failure: undefined };
    try {
      mounted = await mount(
        withServices(
          <PlayUpwardContext value={upward}>
            <div className="film-player__stage">
              <video ref={videoRef}>
                <track kind="captions" />
              </video>
              <OverTheFilm {...film} shouldOpenFullScreen />
            </div>
          </PlayUpwardContext>,
        ),
      );
      expect(steps).toEqual(["play", "pause", "fullScreen"]);
    } finally {
      Reflect.deleteProperty(HTMLElement.prototype, "requestFullscreen");
    }
  });
});
