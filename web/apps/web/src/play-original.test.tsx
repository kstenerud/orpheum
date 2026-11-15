// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import type { JSX } from "react";

import { dataCapsKey } from "./data-cap.ts";
import { DebugToolsContext } from "./debug-tools.tsx";
import { PlayView } from "./play.tsx";
import { fire, mount, servedBy, type Mounted } from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const onLadder = {
  url: "/api/v1/bytes/media/chunks/t/master.m3u8",
  startAtBps: 1_896_000,
  qualityKbps: 1200,
  height: 480,
  isBelowTheLowest: false,
  soundCarriedGainDb: 0,
  isReady: true,
  filmPictureKbps: 4000,
  linkSharePercent: 70,
  steadiness: 0,
};

/**
 * The Play page of a film on its ladder, its person's debug tools shown or not; given
 * isOriginalWaiting, the original its person chose waiting there for the link to carry it.
 */
async function onItsLadder(
  showsDebugTools: boolean,
  asked: string[] = [],
  isOriginalWaiting = false,
): Promise<Mounted> {
  const page: JSX.Element = (
    <PlayView
      fileId={2}
      decision={directDecision({
        onLadder,
        ...(isOriginalWaiting ? { isOriginalWaitingForTheLink: true } : {}),
      })}
      onDecideAgain={(cause) => asked.push(cause)}
    />
  );
  return await mount(
    withServices(
      <DebugToolsContext value={{ isShown: showsDebugTools, onChanged: () => undefined }}>
        {page}
      </DebugToolsContext>,
      servedBy({
        "GET /files/2/waits": [200, {}],
        "GET /files/2/availability": [200, {}],
        "GET /files/2/up-next": [200, {}],
        "POST /playback/t/report": [200, {}],
        "DELETE /playback/t": [204, null],
      }),
    ),
  );
}

describe("the original file, chosen over the ladder (web-client.md: playback is never a black box)", () => {
  it("is offered under a film on its ladder, and asks how the film plays again when pressed", async () => {
    const asked: string[] = [];
    mounted = await onItsLadder(true, asked);
    const button = [...mounted.container.querySelectorAll("button")].find(
      (each) => each.textContent === "Play the original file",
    );
    expect(button).toBeDefined();
    await fire(button ?? null, new MouseEvent("click", { bubbles: true }));
    expect(asked).toEqual(["originalChosen"]);
  });

  it("is not offered under the film on a device under a data cap, which plays a rung (D488)", async () => {
    window.localStorage.setItem(dataCapsKey, JSON.stringify({ unknown: 796 }));
    try {
      mounted = await onItsLadder(true);
      expect(mounted.container.textContent).not.toContain("Play the original file");
    } finally {
      window.localStorage.removeItem(dataCapsKey);
    }
  });

  it("is offered under the film where the device's cap is at or above its file's rate (D491)", async () => {
    window.localStorage.setItem(dataCapsKey, JSON.stringify({ unknown: 2400 })); // the file's own
    try {
      mounted = await onItsLadder(true);
      expect(mounted.container.textContent).toContain("Play the original file");
      // ⁝'s too, told the file's rate by the page.
      await fire(moreButton(mounted), new MouseEvent("click", { bubbles: true }));
      const inTheMenu = [...menuOpen(mounted).querySelectorAll('button, [role="button"]')].map(
        (each) => each.textContent,
      );
      expect(inTheMenu).toContain("Play the original file");
    } finally {
      window.localStorage.removeItem(dataCapsKey);
    }
  });

  it("is a debug tool, offered only to a person who shows them (the owner, 2026-10-29)", async () => {
    mounted = await onItsLadder(false);
    expect(mounted.container.textContent).not.toContain("Play the original file");
  });

  it("is not offered for a film already playing its own file, which may play in the bare player", async () => {
    mounted = await mount(
      withServices(
        <PlayView fileId={2} decision={directDecision()} onDecideAgain={() => undefined} />,
        servedBy({
          "GET /files/2/waits": [200, {}],
          "GET /files/2/up-next": [200, {}],
          "DELETE /playback/t": [204, null],
        }),
      ),
    );
    expect(mounted.container.textContent).not.toContain("Play the original file");
    const bare = [...mounted.container.querySelectorAll("button")].find(
      (each) => each.textContent === "Play in a bare player",
    );
    await fire(bare ?? null, new MouseEvent("click", { bubbles: true }));
    // The video alone, the browser's own controls, nothing over it (bare-player.tsx).
    const video = mounted.container.querySelector("video.bare-player");
    expect(video?.getAttribute("src")).toBe("/api/v1/bytes/media/direct/abc");
    expect(video?.hasAttribute("controls")).toBe(true);
    expect(mounted.container.querySelector(".film-player__stage")).toBeNull();
  });

  it("is the quality entry of the player's ⁝ (D418 §4)", async () => {
    const asked: string[] = [];
    mounted = await onItsLadder(true, asked);
    await fire(moreButton(mounted), new MouseEvent("click", { bubbles: true }));
    const original = [...menuOpen(mounted).querySelectorAll('button, [role="button"]')].find(
      (each) => each.textContent === "Play the original file",
    );
    await fire(original ?? null, new MouseEvent("click", { bubbles: true }));
    expect(asked).toEqual(["originalChosen"]);
  });
});

// The owner, 2026-11-15: the original chosen on a link too slow for it starts on a lower rung, and
// ⋯ says so.
describe("the original chosen, waiting on the ladder for a link short of it", () => {
  const waits = "Plays the original file once the link carries it";

  it("says so under the film and in ⁝, in place of offering it again", async () => {
    mounted = await onItsLadder(true, [], true);
    expect(mounted.container.textContent).toContain(waits);
    expect(mounted.container.textContent).not.toContain("Play the original file");
    await fire(moreButton(mounted), new MouseEvent("click", { bubbles: true }));
    const inTheMenu = menuOpen(mounted);
    expect(inTheMenu.querySelector(".list-box__note .play__original-waits")?.textContent).toBe(
      waits,
    );
    expect(inTheMenu.textContent).not.toContain("Play the original file");
  });

  it("is a debug tool, as the button it answers is", async () => {
    mounted = await onItsLadder(false, [], true);
    expect(mounted.container.textContent).not.toContain(waits);
  });
});

describe("the player's How it plays (D444)", () => {
  it("says in ⁝ that a film on its ladder plays converted, from this page's own player", async () => {
    mounted = await onItsLadder(false);
    await fire(moreButton(mounted), new MouseEvent("click", { bubbles: true }));
    expect(menuOpen(mounted).querySelector(".how-it-plays")?.textContent).toMatch(
      /^Plays it converted/u,
    );
  });
});

/** The player's ⁝ (player-menus.tsx). */
function moreButton(on: Mounted): Element | null {
  return on.container.querySelector('[aria-haspopup][aria-label="Settings"]');
}

/** The menu or dialog open over the film. */
function menuOpen(on: Mounted): Element {
  const open = on.container.querySelector("dialog[open]");
  if (open === null) {
    throw new Error("nothing open over the film");
  }
  return open;
}

// Item 9 of ui-audit-2026-11-04.md: the notes under the film, hidden once it fills the window
// (D418 §1), are ⁝'s For testers, still kept as it plays, nothing removed for testers.
describe("the player's For testers", () => {
  const quality = "Starting at 480p";

  it("keeps the notes the film hides out of view until ⁝'s For testers shows them, and again after", async () => {
    mounted = await onItsLadder(true);
    const note = (): Element | undefined =>
      [...mounted!.container.querySelectorAll(".for-testers p")].find((each) =>
        each.textContent.startsWith(quality),
      );
    expect(note()?.closest("[hidden]")).not.toBeNull();
    await fire(moreButton(mounted), new MouseEvent("click", { bubbles: true }));
    const entry = [...menuOpen(mounted).querySelectorAll(".list-box__option")].find(
      (each) => each.textContent === "For testers",
    );
    await fire(entry ?? null, new MouseEvent("click", { bubbles: true }));
    const dialog = menuOpen(mounted);
    expect(dialog.getAttribute("aria-label") ?? dialog.textContent).toContain("For testers");
    expect(note()?.closest("dialog")).toBe(dialog);
    expect(note()?.closest("[hidden]")).toBeNull();
    // Its tools too: the debug link's, kept beside the notes (play.tsx's UnderThePlayer).
    expect(dialog.querySelector(".for-testers")?.childElementCount).toBeGreaterThan(1);
    const close = [...dialog.querySelectorAll("button")].find(
      (each) => each.getAttribute("aria-label") === "Close" || each.textContent === "Close",
    );
    await fire(close ?? null, new MouseEvent("click", { bubbles: true }));
    expect(mounted.container.querySelector("dialog[open]")).toBeNull();
    expect(note()?.closest("[hidden]")).not.toBeNull();
  });
});
