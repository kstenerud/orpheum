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
import { describe, expect, it } from "vitest";
import { page } from "vitest/browser";

import { commandEventName } from "./input-commands.ts";
import { IconButton } from "./icon-button.tsx";
import {
  AudioIcon,
  BackIcon,
  CloseIcon,
  MoreIcon,
  NextIcon,
  PreviousIcon,
  SubtitlesIcon,
} from "./player-icons.tsx";
import { PlayerOverlay } from "./player-overlay.tsx";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { UiGallery } from "./ui-gallery.tsx";

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
      more: <MoreIcon />,
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

  it("draws Up next's × on the card's ground, as its card", async () => {
    await drawn(
      <div className="up-next">
        <IconButton
          isOverVideo
          className="up-next__dismiss"
          label="Dismiss"
          icon={<CloseIcon />}
          onClick={() => undefined}
        />
      </div>,
      (mounted) => {
        expect(
          getComputedStyle(found(mounted.container, ".up-next__dismiss")).backgroundColor,
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

  it("draws nothing of the page under a stage filling the window, nor a pulse beside a film, and shows the page as it was after (the efficiency audit, finding 3)", async () => {
    await drawn(
      <>
        <header className="app-shell__banner">
          <button type="button" className="status-tray__button status-tray__button--on">
            <svg className="status-tray__icon" aria-label="Watch Together" />
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
        expect(getComputedStyle(found(mounted.container, ".status-tray__icon")).animationName).toBe(
          "none",
        );
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
