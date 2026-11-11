// SPDX-License-Identifier: MPL-2.0

/**
 * The press kept for a film's start (keepThePressForTheStart) beside the start gate's own play, in a
 * real browser: the owner's next episode, played by Next or by Continue on "Are you still watching?"
 * in a full-screen Firefox on a Mac, opened paused at 0:00 with 35 s loaded, its start never made
 * (production and staging, 2026-11-11). The Play page's start gate waits to play as the video loads,
 * and, the browser full screen, the press is kept just after: its play, paused at once, was heard as
 * the person's own and gave the gate's wait up.
 */

import { startGateOf } from "@orpheum/sdk";
import { afterEach, describe, expect, it } from "vitest";
import { userEvent } from "vitest/browser";

import { toneWav } from "./lab-tone.ts";
import { keepThePressForTheStart } from "./start-refused.ts";

/**
 * Opens the film as the Play page does: its video's start gate waits to play as it loads
 * (film-player.tsx's useStartsWithEnoughAhead), and, the browser full screen, the press is kept for
 * its start just after (play.tsx's useFullscreenAsTheBrowserIs, an effect after the gate's).
 */
function opensPlaying(address: string): HTMLVideoElement {
  const film = document.createElement("video");
  film.muted = true;
  film.src = address;
  document.body.append(film);
  startGateOf(film).play(() => {
    film.play().catch(() => undefined);
  });
  keepThePressForTheStart(film);
  return film;
}

describe("a film whose press is kept for its start while its start gate waits", () => {
  let video: HTMLVideoElement | undefined;
  let url: string | undefined;
  afterEach(() => {
    video?.pause();
    video?.remove();
    video = undefined;
    if (url !== undefined) {
      URL.revokeObjectURL(url);
    }
    url = undefined;
  });

  it("still plays once enough is loaded (the owner's next episode, 2026-11-11)", async () => {
    const button = document.createElement("button");
    button.textContent = "Next";
    document.body.append(button);
    url = URL.createObjectURL(new Blob([toneWav(440, { seconds: 2 })], { type: "audio/wav" }));
    // The press that plays the next episode, live as its page opens.
    button.addEventListener("click", () => {
      video = opensPlaying(url ?? "");
    });
    await userEvent.click(button);
    button.remove();
    expect(video).toBeDefined();
    await expect.poll(() => video?.paused, { timeout: 5000 }).toBe(false);
  });
});
