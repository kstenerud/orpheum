// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { createRef } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";

import { FullScreenButton } from "./full-screen-button.tsx";
import { IconButton } from "./icon-button.tsx";
import { PlayIcon } from "./player-icons.tsx";
import { PlayerOverlay } from "./player-overlay.tsx";
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

  it("says whether what it opens is open, and takes the class of where it is", () => {
    const markup = renderToStaticMarkup(
      <IconButton
        label="Watch Together"
        icon={<PlayIcon />}
        onClick={() => undefined}
        isExpanded={false}
        className="status-tray__button"
      />,
    );
    expect(markup).toContain('class="icon-button status-tray__button"');
    expect(markup).toContain('aria-expanded="false"');
  });
});

describe("the player's buttons, icons in place of characters a television's font may lack", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("draws the TV overlay's steps, play and full screen as icon buttons, with no text", async () => {
    vi.spyOn(Navigator.prototype, "userAgent", "get").mockReturnValue(
      "Mozilla/5.0 (Linux; Android 9; AFTSSS) AppleWebKit/537.36 (KHTML, like Gecko) Silk/138.19.9 like Chrome/138.0.7204.244 Safari/537.36",
    );
    const videoRef = createRef<HTMLVideoElement>();
    Reflect.set(videoRef, "current", document.createElement("video"));
    mounted = await mount(
      withServices(
        <PlayerOverlay videoRef={videoRef} readLinkKbps={() => undefined} failure={undefined} />,
      ),
    );
    const buttons = [...mounted.container.querySelectorAll("button")];
    vi.restoreAllMocks();
    expect(buttons.length).toBe(4);
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
