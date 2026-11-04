// SPDX-License-Identifier: MPL-2.0

/**
 * The player's widgets as a real browser lays them out (D369): what only their stylesheets decide,
 * in Chromium and Firefox, with no server.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./cast.css";
import "./status-tray.css";
import "./title.css";
import "./up-next.css";
import "./i18n.ts";

import { act, createRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { page, userEvent } from "vitest/browser";

import { commandEventName } from "./input-commands.ts";
import { IconButton } from "./icon-button.tsx";
import {
  AudioIcon,
  BackIcon,
  CloseIcon,
  SettingsIcon,
  NextIcon,
  PreviousIcon,
  SubtitlesIcon,
} from "./player-icons.tsx";
import { PlayUpwardContext } from "./play-up.tsx";
import { PlayerOverlay } from "./player-overlay.tsx";
import { doubleTapWaitMs } from "./player-overlay-state.ts";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { UiGallery } from "./ui-gallery.tsx";
import type { Upward } from "./upward.ts";

/** Runs check over what tree draws, then unmounts it. */
async function drawn(
  tree: Parameters<typeof mount>[0],
  check: (mounted: Mounted) => Promise<void> | void,
): Promise<void> {
  const mounted = await mount(withServices(tree));
  try {
    await check(mounted);
  } finally {
    mounted.unmount();
  }
}

/** The element selector names in container; it fails the test when there is none. */
function found(container: ParentNode, selector: string): HTMLElement {
  const element = container.querySelector<HTMLElement>(selector);
  if (element === null) {
    throw new Error(`nothing drawn is ${selector}`);
  }
  return element;
}

/** The colour --color-video-card names, as the browser computes a background of it. */
const videoCard = "rgba(0, 0, 0, 0.8)";

/** An episode's place, for the overlay's way up: its series, a page of its own. */
const episodeUpward: Upward = {
  crumbs: [{ label: "Gus Doyle", to: { screen: "home" }, isIt: () => false }],
  here: "S2E3",
};

/**
 * Puts the overlay's time in the test's hands, as player-overlay.test.tsx does: its look whether its
 * time is up (an interval) runs only as the test moves the time on, and performance.now, which it
 * reads, says the test's time. A key the browser delivers late, a machine under load taking more than
 * the overlay's 3 s for a round trip, then hides nothing. Returns what moves the time on by ms.
 */
function overlayTimeHeld(): (ms: number) => void {
  vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
  let pageNowMs = 0;
  vi.spyOn(performance, "now").mockImplementation(() => pageNowMs);
  return (ms) => {
    pageNowMs += ms;
    act(() => {
      vi.advanceTimersByTime(ms);
    });
  };
}

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

/**
 * Shows the overlay, hidden as the player opens (ui-audit-2026-11-04.md §3, the player), as a mouse
 * moved over the page does (player-taps.ts).
 */
function shownByAMouse(): void {
  for (const screenX of [5, 50]) {
    act(() => {
      document.body.dispatchEvent(
        new PointerEvent("pointermove", {
          bubbles: true,
          pointerType: "mouse",
          screenX,
          screenY: 5,
        }),
      );
    });
  }
}

describe("the player's widgets, laid out by a real browser (D369 §1)", () => {
  it("draws every icon button, the status tray's signs too, a fingertip square: 2.75rem", async () => {
    // From a tablet up: on a phone the banner's signs stand a quarter closer, by the owner's word
    // (2026-10-28), which app-shell.browser.test.tsx measures.
    await page.viewport(1280, 720);
    await drawn(<UiGallery />, (mounted) => {
      const sizes = [
        ...mounted.container.querySelectorAll(".icon-button, .status-tray__button--warning"),
      ].map((each) => {
        const box = each.getBoundingClientRect();
        return `${String(Math.round(box.width))}×${String(Math.round(box.height))}`;
      });
      expect(sizes.length).toBeGreaterThan(5);
      expect(new Set(sizes)).toEqual(new Set(["44×44"]));
    });
  });

  it("draws the overlay's icons on a fingertip square, 2.75rem, their shapes inside it (D407)", async () => {
    const icons = {
      previous: <PreviousIcon />,
      next: <NextIcon />,
      subtitles: <SubtitlesIcon />,
      audio: <AudioIcon />,
      settings: <SettingsIcon />,
      back: <BackIcon />,
    };
    await page.viewport(1280, 720);
    await drawn(
      <div className="ui-gallery__film">
        {Object.entries(icons).map(([name, icon]) => (
          <IconButton key={name} isOverVideo label={name} icon={icon} onClick={() => undefined} />
        ))}
      </div>,
      (mounted) => {
        const buttons = [...mounted.container.querySelectorAll(".icon-button")];
        expect(buttons).toHaveLength(Object.keys(icons).length);
        for (const button of buttons) {
          const box = button.getBoundingClientRect();
          expect(`${String(Math.round(box.width))}×${String(Math.round(box.height))}`).toBe(
            "44×44",
          );
          const drawing = found(button, ".player-icons__icon").getBoundingClientRect();
          expect(drawing.width).toBeGreaterThan(0);
          expect(drawing.left).toBeGreaterThanOrEqual(box.left);
          expect(drawing.right).toBeLessThanOrEqual(box.right);
          // Drawn, never as text a television's font may lack.
          expect(button.textContent).toBe("");
        }
      },
    );
  });

  it("draws a running icon button in the status blue, pulsing, and an applied one in the accent, still", async () => {
    const colourOf = (container: ParentNode, token: string): string => {
      const probe = document.createElement("span");
      probe.style.color = `var(${token})`;
      container.appendChild(probe);
      const colour = getComputedStyle(probe).color;
      probe.remove();
      return colour;
    };
    await drawn(
      <>
        <IconButton label="Plain" icon={<CloseIcon />} onClick={() => undefined} />
        <IconButton label="Running" mark="running" icon={<CloseIcon />} onClick={() => undefined} />
        <IconButton label="Applied" mark="applied" icon={<CloseIcon />} onClick={() => undefined} />
      </>,
      ({ container }) => {
        const button = (label: string): HTMLElement => found(container, `[aria-label="${label}"]`);
        const pulse = (label: string): string =>
          getComputedStyle(found(button(label), ".icon")).animationName;
        expect(getComputedStyle(button("Running")).color).toBe(
          colourOf(container, "--color-status"),
        );
        expect(getComputedStyle(button("Applied")).color).toBe(
          colourOf(container, "--color-accent"),
        );
        expect(getComputedStyle(button("Plain")).color).toBe(colourOf(container, "--color-text"));
        expect(pulse("Running")).not.toBe("none");
        expect(pulse("Applied")).toBe("none");
        expect(pulse("Plain")).toBe("none");
      },
    );
  });

  it("draws Up next's × on the card's ground, as its card", async () => {
    await drawn(
      <div className="up-next">
        <IconButton
          isOverVideo
          isOnCard
          label="Dismiss"
          icon={<CloseIcon />}
          onClick={() => undefined}
        />
      </div>,
      (mounted) => {
        expect(
          getComputedStyle(found(mounted.container, ".up-next .icon-button")).backgroundColor,
        ).toBe(videoCard);
      },
    );
  });

  it("draws the cast receiver's lines on the card's ground, in the colour over a film", async () => {
    await drawn(
      <div className="cast-receiver">
        <output className="cast-receiver__note">{"·"}</output>
        <output className="cast-receiver__sound-on">{"·"}</output>
      </div>,
      (mounted) => {
        for (const selector of [".cast-receiver__note", ".cast-receiver__sound-on"]) {
          const style = getComputedStyle(found(mounted.container, selector));
          expect(style.backgroundColor).toBe(videoCard);
          expect(style.color).toBe("rgb(255, 255, 255)");
        }
      },
    );
  });

  it("hides the overlay's way up and buttons after a seek, the timeline alone showing", async () => {
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", document.createElement("video"));
    await drawn(
      <div className="film-player__stage">
        <PlayerOverlay videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />
      </div>,
      (mounted) => {
        act(() => {
          document.body.dispatchEvent(
            new CustomEvent(commandEventName, {
              detail: { command: "back", source: "key" },
              bubbles: true,
              cancelable: true,
            }),
          );
        });
        act(() => {
          document.body.dispatchEvent(
            new CustomEvent(commandEventName, {
              detail: { command: "right", source: "key" },
              bubbles: true,
              cancelable: true,
            }),
          );
        });
        const overlay = found(mounted.container, ".player-overlay");
        expect(overlay.getAttribute("data-shown")).toBe("scrub");
        expect(getComputedStyle(found(overlay, ".player-controls__buttons")).visibility).toBe(
          "hidden",
        );
        expect(getComputedStyle(found(overlay, ".player-overlay__top")).visibility).toBe("hidden");
        expect(getComputedStyle(found(overlay, ".timeline")).visibility).toBe("visible");
      },
    );
  });

  // The owner, 2026-11-03: "A user will expect that leaving it idle will cause it to fade out soon",
  // a key's focus in it holding it no longer (D411, amending D409).
  it("hides a playing film's overlay its time after the last press, a key's focus in it or not (D411)", async () => {
    const video = document.createElement("video");
    Object.defineProperty(video, "paused", { get: () => false });
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", video);
    const elapse = overlayTimeHeld();
    await drawn(
      <div className="film-player__stage">
        <PlayerOverlay videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />
      </div>,
      async (mounted) => {
        const overlay = found(mounted.container, ".player-overlay");
        mounted.container.append(video);
        video.tabIndex = -1;
        video.focus();
        await userEvent.keyboard("{ArrowDown}");
        await expect.poll(() => overlay.contains(document.activeElement)).toBe(true);
        expect(document.activeElement?.matches(":focus-visible")).toBe(true);
        expect(overlay.getAttribute("data-shown")).toBe("full");
        elapse(2900);
        expect(overlay.getAttribute("data-shown")).toBe("full");
        elapse(200);
        expect(overlay.getAttribute("data-shown")).toBe("hidden");
        expect(overlay.contains(document.activeElement)).toBe(false);
      },
    );
  });

  // The owner, 2026-11-03: the overlay up by a mouse, "the next up/down key from the keyboard should
  // navigate away from the scrub bar, as if focus had already been there", the first key not
  // swallowed by taking focus in (D411).
  it("moves focus on the video by Down to the buttons, and by Up to the way up, as if it were on the scrub bar, while a pointer shows all of the overlay (D411)", async () => {
    const video = document.createElement("video");
    Object.defineProperty(video, "paused", { get: () => false });
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", video);
    overlayTimeHeld(); // a playing film's overlay, which would hide as the keys come late
    await drawn(
      <PlayUpwardContext value={episodeUpward}>
        <div className="film-player__stage">
          <PlayerOverlay videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />
        </div>
      </PlayUpwardContext>,
      async (mounted) => {
        const overlay = found(mounted.container, ".player-overlay");
        mounted.container.append(video);
        video.tabIndex = -1;
        video.focus();
        shownByAMouse();
        expect(overlay.getAttribute("data-shown")).toBe("full");
        await userEvent.keyboard("{ArrowDown}");
        await expect
          .poll(() => document.activeElement?.getAttribute("aria-label"))
          .toBe("Back 10 s");
        video.focus();
        await userEvent.keyboard("{ArrowUp}");
        await expect
          .poll(() => document.activeElement?.classList.contains("player-overlay__up"))
          .toBe(true);
        expect(overlay.contains(document.activeElement)).toBe(true);
        expect(overlay.getAttribute("data-shown")).toBe("full");
      },
    );
  });

  // The owner, 2026-11-03: focused, the scrub bar looks as it always does, its ring "looks terrible";
  // a key's focus moved off it fades it a little, a mouse's never (D410).
  it("draws the focused scrub bar with no ring, and fades it a little while a key's focus is on a button", async () => {
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", document.createElement("video"));
    await drawn(
      <div className="film-player__stage">
        <PlayerOverlay videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />
      </div>,
      async (mounted) => {
        const overlay = found(mounted.container, ".player-overlay");
        const scrub = found(overlay, '[aria-label="Where the film stands"]');
        const forward = found(overlay, '[aria-label="Forward 10 s"]');
        shownByAMouse();
        await userEvent.click(forward);
        expect(getComputedStyle(scrub).opacity).toBe("1");
        while (document.activeElement !== scrub) {
          await userEvent.keyboard("{Shift>}{Tab}{/Shift}");
        }
        expect(scrub.matches(":focus-visible")).toBe(true);
        expect(getComputedStyle(scrub).outlineStyle).toBe("none");
        expect(getComputedStyle(scrub).boxShadow).toBe("none");
        await expect.poll(() => getComputedStyle(scrub).opacity).toBe("1");
        await userEvent.keyboard("{Tab}");
        expect(overlay.contains(document.activeElement)).toBe(true);
        // Under make check's load Firefox once left it at 1 (2026-11-04), passing alone; the message says
        // where focus was and whether it counted as a key's, so a failure names its cause.
        const focusSaid = (): string => {
          const focused = document.activeElement;
          return `focus on ${focused?.outerHTML.slice(0, 120) ?? "nothing"}, :focus-visible ${String(
            focused?.matches(":focus-visible"),
          )}, the overlay ${overlay.getAttribute("data-shown") ?? "?"}, the scrub bar in the page ${String(
            scrub.isConnected,
          )}`;
        };
        try {
          await expect.poll(() => getComputedStyle(scrub).opacity).toBe("0.6");
        } catch (error) {
          throw new Error(`${String(error)}; ${focusSaid()}`, { cause: error });
        }
      },
    );
  });

  it("moves JASSUB's subtitles up while the overlay's buttons show, and back as it hides (D407 §8)", async () => {
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", document.createElement("video"));
    await drawn(
      <div className="film-player__stage">
        <canvas className="JASSUB" />
        <PlayerOverlay videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />
      </div>,
      (mounted) => {
        const subtitles = found(mounted.container, ".JASSUB");
        shownByAMouse();
        expect(found(mounted.container, ".player-overlay").getAttribute("data-shown")).toBe("full");
        expect(getComputedStyle(subtitles).translate).not.toBe("none");
        act(() => {
          document.body.dispatchEvent(
            new CustomEvent(commandEventName, {
              detail: { command: "back", source: "key" },
              bubbles: true,
              cancelable: true,
            }),
          );
        });
        expect(found(mounted.container, ".player-overlay").getAttribute("data-shown")).toBe(
          "hidden",
        );
        expect(getComputedStyle(subtitles).translate).toBe("none");
      },
    );
  });

  it("draws nothing of the page under a stage filling the window, nor a pulse beside a film, and shows the page as it was after (the efficiency audit, finding 3)", async () => {
    await drawn(
      <>
        <header className="app-shell__banner">
          <button type="button" className="icon-button icon-button--running">
            <svg className="icon" aria-label="Watch Together" />
          </button>
        </header>
        <main>
          <input aria-label="kept" defaultValue="typed" />
          <div className="film-player__stage">
            <div className="film-player__video" />
          </div>
        </main>
        <div className="cast-receiver">
          <output className="cast-receiver__note">{"·"}</output>
        </div>
      </>,
      (mounted) => {
        const visibilityOf = (selector: string): string =>
          getComputedStyle(found(mounted.container, selector)).visibility;
        expect(
          getComputedStyle(found(mounted.container, ".icon-button--running > .icon")).animationName,
        ).toBe("none");
        expect(visibilityOf(".app-shell__banner")).toBe("visible");
        const stage = found(mounted.container, ".film-player__stage");
        stage.classList.add("film-player__stage--fills-window");
        expect(visibilityOf(".app-shell__banner")).toBe("hidden");
        expect(visibilityOf("input")).toBe("hidden");
        expect(visibilityOf(".film-player__video")).toBe("visible");
        expect(visibilityOf(".cast-receiver__note")).toBe("visible");
        stage.classList.remove("film-player__stage--fills-window");
        expect(visibilityOf(".app-shell__banner")).toBe("visible");
        expect(found(mounted.container, "input")).toHaveProperty("value", "typed");
      },
    );
  });

  it("draws the gallery's film black, the bar over it along its bottom", async () => {
    await drawn(<UiGallery />, (mounted) => {
      const film = found(mounted.container, ".ui-gallery__film:not(.ui-gallery__film--strip)");
      expect(getComputedStyle(film).backgroundColor).toBe("rgb(0, 0, 0)");
      const bar = found(film, ".ui-gallery__bar").getBoundingClientRect();
      const box = film.getBoundingClientRect();
      expect(Math.round(bar.bottom)).toBe(Math.round(box.bottom));
      expect(box.height).toBeGreaterThan(bar.height);
    });
  });
});

describe("Play, a button like every other (D369 §3)", () => {
  it("is a submit button's size and shape, in the accent", async () => {
    await drawn(
      <p>
        <a href="#play" className="title__play">
          {"Play"}
        </a>{" "}
        <button type="submit">{"Play"}</button>
      </p>,
      (mounted) => {
        const play = getComputedStyle(found(mounted.container, ".title__play"));
        const submit = getComputedStyle(found(mounted.container, "button"));
        for (const property of [
          "paddingLeft",
          "paddingRight",
          "borderTopWidth",
          "borderTopColor",
          "backgroundColor",
          "minHeight",
        ] as const) {
          expect(`${property}: ${play[property]}`).toBe(`${property}: ${submit[property]}`);
        }
      },
    );
  });
});

// The owner, 2026-11-04: "For the tap thing: agreed" (D415): the thirds are of the picture as the
// browser lays it out, and a finger's tap is a real pointer sequence.
describe("A finger's taps on the full-screen picture, as a real browser lays it out (D415)", () => {
  it("hides and shows a paused film's overlay by a tap in the middle, and seeks by a double tap on a side", async () => {
    const video = document.createElement("video");
    Object.defineProperty(video, "paused", { get: () => true });
    Object.defineProperty(video, "duration", { get: () => 600 });
    let atS = 100;
    Object.defineProperty(video, "currentTime", {
      get: () => atS,
      set: (to: number) => {
        atS = to;
      },
    });
    video.style.display = "block";
    video.style.width = "600px";
    video.style.height = "300px";
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", video);
    await drawn(
      <div className="film-player__stage">
        <PlayerOverlay videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />
      </div>,
      async (mounted) => {
        found(mounted.container, ".film-player__stage").prepend(video);
        const overlay = found(mounted.container, ".player-overlay");
        const box = video.getBoundingClientRect();
        const tapAt = (fraction: number): void => {
          const init = {
            bubbles: true,
            pointerType: "touch",
            pointerId: 3,
            clientX: box.left + box.width * fraction,
            clientY: box.top + 20,
          };
          act(() => {
            video.dispatchEvent(new PointerEvent("pointerdown", init));
            video.dispatchEvent(new PointerEvent("pointerup", init));
            video.dispatchEvent(new MouseEvent("click", { bubbles: true, detail: 1 }));
          });
        };
        shownByAMouse();
        expect(overlay.getAttribute("data-shown")).toBe("full");
        tapAt(0.5);
        expect(overlay.getAttribute("data-shown")).toBe("hidden");
        tapAt(0.5);
        expect(overlay.getAttribute("data-shown")).toBe("full");
        tapAt(0.9);
        tapAt(0.9);
        expect(atS).toBe(110);
        tapAt(0.1);
        tapAt(0.1);
        expect(atS).toBe(100);
        await new Promise((done) => setTimeout(done, doubleTapWaitMs * 2));
        expect(overlay.getAttribute("data-shown")).toBe("full");
        // The indicator stands in the stage's left third (a full-screen film fills it), catching no tap.
        const seeked = found(mounted.container, ".player-overlay-seeked");
        expect(seeked.textContent).toBe("\u221210");
        expect(getComputedStyle(seeked).pointerEvents).toBe("none");
        const seekedBox = seeked.getBoundingClientRect();
        const stageBox = found(mounted.container, ".film-player__stage").getBoundingClientRect();
        expect(seekedBox.left + seekedBox.width / 2).toBeLessThan(
          stageBox.left + stageBox.width / 3,
        );
        expect(seekedBox.left).toBeGreaterThanOrEqual(stageBox.left);
      },
    );
  });
});
