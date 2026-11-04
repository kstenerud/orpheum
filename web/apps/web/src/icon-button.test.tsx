// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { createRef } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";

import { FullScreenButton } from "./full-screen-button.tsx";
import { IconButton } from "./icon-button.tsx";
import { PlayIcon } from "./player-icons.tsx";
import { CastSignOverTheFilmContext } from "./cast-sign-over-the-film.tsx";
import { PlayerOverlay } from "./player-overlay.tsx";
import { castSignOverTheFilm } from "./status-tray.tsx";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

describe("an icon button (D369 §1)", () => {
  it("is a button named by its label, drawing its icon by Icon, over a film round", () => {
    const markup = renderToStaticMarkup(
      <IconButton label="Play" icon={<PlayIcon />} onClick={() => undefined} isOverVideo />,
    );
    expect(markup).toMatch(
      /^<button type="button" class="icon-button icon-button--over-video" aria-label="Play"><svg class="icon player-icons__icon"/u,
    );
  });

  it("says whether what it opens is open", () => {
    const markup = renderToStaticMarkup(
      <IconButton
        label="Watch Together"
        icon={<PlayIcon />}
        onClick={() => undefined}
        isExpanded={false}
      />,
    );
    expect(markup).toContain('class="icon-button"');
    expect(markup).toContain('aria-expanded="false"');
  });

  it("is marked running or applied, or on a card over a film, by a named option, and takes no class", () => {
    const draw = (extra: Partial<Parameters<typeof IconButton>[0]>): string =>
      renderToStaticMarkup(
        <IconButton label="Play" icon={<PlayIcon />} onClick={() => undefined} {...extra} />,
      );
    expect(draw({ mark: "running" })).toContain('class="icon-button icon-button--running"');
    expect(draw({ mark: "applied" })).toContain('class="icon-button icon-button--applied"');
    expect(draw({ isOverVideo: true, isOnCard: true })).toContain(
      'class="icon-button icon-button--over-video icon-button--on-card"',
    );
    expect(draw({})).toContain('class="icon-button"');
  });
});

describe("the player's buttons, icons in place of characters a television's font may lack", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("draws the TV overlay's steps, play, full screen and Cast (D422) as icon buttons, with no text", async () => {
    vi.spyOn(Navigator.prototype, "userAgent", "get").mockReturnValue(
      "Mozilla/5.0 (Linux; Android 9; AFTSSS) AppleWebKit/537.36 (KHTML, like Gecko) Silk/138.19.9 like Chrome/138.0.7204.244 Safari/537.36",
    );
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", document.createElement("video"));
    mounted = await mount(
      withServices(
        // Cast's sign as the signed-in app hands it to the overlay (app.tsx).
        <CastSignOverTheFilmContext value={castSignOverTheFilm}>
          <PlayerOverlay videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />
        </CastSignOverTheFilmContext>,
      ),
    );
    const buttons = [...mounted.container.querySelectorAll("button")];
    vi.restoreAllMocks();
    expect(buttons.map((button) => button.getAttribute("aria-label"))).toEqual([
      "Cast to a device",
      "Back 10 s",
      "Play",
      "Forward 10 s",
      "Full screen",
    ]);
    for (const button of buttons) {
      expect(button.classList.contains("icon-button")).toBe(true);
      expect(button.querySelector("svg.icon")).not.toBeNull();
      expect(button.textContent).toBe("");
    }
  });

  it("draws full screen by its icon, and leaving it by another", () => {
    const markup = renderToStaticMarkup(withServices(<FullScreenButton />));
    expect(markup).toContain('aria-label="Full screen"');
    expect(markup).toContain("<svg");
    expect(markup).not.toMatch(/[⤢⤡]/u);
  });
});
