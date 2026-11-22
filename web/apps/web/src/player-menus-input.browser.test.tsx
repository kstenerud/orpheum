// SPDX-License-Identifier: MPL-2.0

// The Sound menu and ⁝'s Quality driven as each device drives them (D538 §8, §10; brief-common's
// browser tests): a mouse, a finger, the keyboard and a TV remote, on a phone upright and on its
// side, a computer's screen and a television's. Focus shows its ring and never lands on nothing.

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { tellOfferedRungs } from "@orpheum/player";
import type { RefObject } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { dataCapsKey, deviceDataCaps } from "./data-cap.ts";
import { startInput } from "./input-commands.ts";
import { PlayerMenus, PlayerMenusContext, type PlayerMenusWays } from "./player-menus.tsx";
import { PlayerTracksContext, type PlayerTracks } from "./player-tracks.tsx";
import { forgetSoundOffsetSetInThisTab } from "./sound-delay.tsx";
import { mount, type Mounted } from "./test-dom.tsx";
import { focusedName, isRingShown, tapOn } from "./test-parts.ts";
import { withServices } from "./test-services.tsx";

/** The screens the menus are driven on: a phone upright and on its side, a computer's, a television's. */
const screens = [
  { width: 360, height: 740 },
  { width: 740, height: 360 },
  { width: 1280, height: 800 },
  { width: 1920, height: 1080 },
] as const;

/** What the menus' presses asked of the film: the audio tracks chosen, levelling's switch. */
interface Noted {
  readonly audio: number[];
  readonly levelling: boolean[];
}

/** The page's ways: a file whose own timing is in step, so This file sits below This device. */
const ways: PlayerMenusWays = {
  fileId: 3,
  soundOffsetMs: 0,
  fileSoundOffset: { everyonesMs: 0, soundOffsetMs: 0 },
  onSubtitleFetched: () => undefined,
};

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  deviceDataCaps().choose(undefined);
  window.localStorage.removeItem(dataCapsKey);
});

/** A film of two audio tracks and no subtitles, so ⁝ holds Start over, Quality and Report a problem. */
function tracksNoting(noted: Noted): PlayerTracks {
  return {
    subtitles: {
      tracks: [],
      chosenTrackKey: undefined,
      choose: () => undefined,
      offsetMs: 0,
      shiftOffset: () => undefined,
      failure: undefined,
    },
    audio: {
      tracks: [
        { streamId: 80, audioTrackIndex: 0, language: "en", kind: "plain" },
        { streamId: 81, audioTrackIndex: 1, language: "fr", kind: "plain" },
      ],
      chosenStreamId: 80,
      choose: (track) => {
        noted.audio.push(track.streamId);
      },
      problem: undefined,
    },
    levelling: {
      isOn: false,
      switchTo: (isOn) => {
        noted.levelling.push(isOn);
      },
      problem: undefined,
    },
    level: { levelling: { isMeasured: true, gainDb: -2 }, carriedGainDb: 0 },
    measuring: undefined,
    boost: undefined,
  };
}

/** A video on the core's default rungs, told as hls.js tells them once it reads the master playlist. */
function videoOnTheLadder(): RefObject<HTMLVideoElement | null> {
  const video = document.createElement("video");
  const levels = [400, 700, 1200, 2000, 3000, 6000].map((kbps) => ({
    bitrate: kbps * 1500,
    averageBitrate: (kbps + 96) * 1000,
    uri: `q${String(kbps)}k/stream.m3u8`,
  }));
  let parsed: (() => void) | undefined = undefined;
  tellOfferedRungs({ levels, on: (_, listener) => (parsed = listener) }, video);
  (parsed as (() => void) | undefined)?.();
  return { current: video };
}

/** Draws the Sound menu and ⁝ at the screen's bottom right, as the overlay's row ends there. */
async function drawnOn(screen: (typeof screens)[number]): Promise<Noted> {
  await page.viewport(screen.width, screen.height);
  forgetSoundOffsetSetInThisTab();
  const noted: Noted = { audio: [], levelling: [] };
  mounted = await mount(
    withServices(
      <div style={{ position: "fixed", right: "16px", bottom: "16px", display: "flex" }}>
        <PlayerTracksContext value={tracksNoting(noted)}>
          <PlayerMenusContext value={ways}>
            <PlayerMenus
              videoRef={videoOnTheLadder()}
              shown={["audio", "more"]}
              onOpenChange={() => undefined}
            />
          </PlayerMenusContext>
        </PlayerTracksContext>
      </div>,
    ),
  );
  return noted;
}

/** The overlay's button that opens the menu named name. */
function opener(name: "Sound" | "Settings"): HTMLElement {
  return page.getByRole("button", { name, exact: true }).element() as HTMLElement;
}

/** The open menu's row whose text starts with text: its button. */
function row(text: string): HTMLElement {
  const found = [
    ...document.querySelectorAll<HTMLElement>("dialog[open] [role='button'], dialog[open] button"),
  ].find((each) => each.checkVisibility() && each.textContent.startsWith(text));
  if (found === undefined) {
    throw new Error(`no row starting ${text} in the open menu`);
  }
  return found;
}

/** Whether the open menu lies wholly on the screen. */
function isMenuOnScreen(): boolean {
  const box = document.querySelector("dialog[open]")?.getBoundingClientRect();
  return (
    box !== undefined &&
    box.left >= 0 &&
    box.top >= 0 &&
    box.right <= window.innerWidth + 1 &&
    box.bottom <= window.innerHeight + 1
  );
}

/** The cap the device keeps, in kb/s, for whichever network the browser says it is on. */
function capsKept(): number[] {
  const kept = window.localStorage.getItem(dataCapsKey);
  return kept === null ? [] : Object.values(JSON.parse(kept) as Record<string, number>);
}

/** Waits for the menu to close, and for focus to be back on its opener. */
async function closedOnto(name: "Sound" | "Settings"): Promise<void> {
  await expect.poll(() => document.querySelector("dialog[open]")).toBeNull();
  await expect.poll(() => document.activeElement).toBe(opener(name));
}

describe("⁝'s Quality and the Sound menu, driven by each device (D538)", () => {
  for (const screen of screens) {
    const at = `${String(screen.width)}×${String(screen.height)}`;

    it(`chooses a Quality and an audio track by mouse at ${at}`, async () => {
      const noted = await drawnOn(screen);
      await userEvent.click(opener("Settings"));
      await userEvent.click(row("Quality: Automatic"));
      await expect.poll(() => focusedName()).toBe("Automatic");
      expect(isMenuOnScreen()).toBe(true);
      await userEvent.click(row("0.6 GB per hour"));
      await expect.poll(() => document.querySelector("dialog[open]")).toBeNull();
      expect(capsKept()).toEqual([1296]);
      await userEvent.click(opener("Sound"));
      expect(isMenuOnScreen()).toBe(true);
      await userEvent.click(row("Sound track: English"));
      await userEvent.click(row("French"));
      expect(noted.audio).toEqual([81]);
    });

    it(`chooses a Quality and switches levelling by touch at ${at}`, async () => {
      const noted = await drawnOn(screen);
      tapOn(opener("Settings"));
      await expect.poll(() => document.querySelector("dialog[open]")).not.toBeNull();
      tapOn(row("Quality: Automatic"));
      await expect.poll(() => focusedName()).toBe("Automatic");
      tapOn(row("2.7 GB per hour"));
      await expect.poll(() => document.querySelector("dialog[open]")).toBeNull();
      expect(capsKept()).toEqual([6096]);
      tapOn(opener("Sound"));
      await expect.poll(() => document.querySelector("dialog[open]")).not.toBeNull();
      expect(isMenuOnScreen()).toBe(true);
      const levelling = page.getByRole("switch", { name: /^Level loudness/u }).element();
      tapOn(levelling);
      expect(noted.levelling).toEqual([true]);
    });

    it(`walks ⁝'s Quality by the keyboard's Tab, Space, Enter and Escape at ${at}`, async () => {
      await drawnOn(screen);
      opener("Sound").focus();
      await userEvent.tab();
      expect(document.activeElement).toBe(opener("Settings"));
      expect(isRingShown()).toBe(true);
      await userEvent.keyboard("{Enter}");
      await expect.poll(() => focusedName()).toBe("Start over");
      expect(isRingShown()).toBe(true);
      await userEvent.tab();
      expect(focusedName()).toBe("Quality: Automatic›");
      expect(isRingShown()).toBe(true);
      await userEvent.keyboard(" ");
      await expect.poll(() => focusedName()).toBe("Automatic");
      expect(isRingShown()).toBe(true);
      await userEvent.tab();
      expect(focusedName()).toBe("2.7 GB per hour");
      await userEvent.keyboard("{Enter}");
      await closedOnto("Settings");
      expect(capsKept()).toEqual([6096]);
      expect(isRingShown()).toBe(true);
      await userEvent.keyboard("{Enter}");
      await expect.poll(() => focusedName()).toBe("Start over");
      await userEvent.keyboard("{Escape}");
      await closedOnto("Settings");
      expect(isRingShown()).toBe(true);
    });

    it(`walks ⁝'s Quality by a remote's arrows and OK, its Down leaving the row at ${at}`, async () => {
      await drawnOn(screen);
      const input = startInput({ goBack: () => false, isNavigating: () => true });
      try {
        opener("Settings").focus();
        await userEvent.keyboard("{Enter}");
        await expect.poll(() => focusedName()).toBe("Start over");
        await userEvent.keyboard("{ArrowDown}");
        expect(focusedName()).toBe("Quality: Automatic›");
        expect(isRingShown()).toBe(true);
        // Report a problem sits below, so Down has a row to leave Quality's for.
        await userEvent.keyboard("{ArrowDown}");
        expect(focusedName()).toBe("Report a problem");
        expect(isRingShown()).toBe(true);
        await userEvent.keyboard("{ArrowUp}");
        await userEvent.keyboard("{Enter}");
        await expect.poll(() => focusedName()).toBe("Automatic");
        expect(isRingShown()).toBe(true);
        // Left returns to the entry, ⁝ still open.
        await userEvent.keyboard("{ArrowLeft}");
        await expect.poll(() => focusedName()).toBe("Quality: Automatic›");
        expect(document.querySelector("dialog[open]")).not.toBeNull();
        await userEvent.keyboard("{ArrowRight}");
        await expect.poll(() => focusedName()).toBe("Automatic");
        await userEvent.keyboard("{ArrowDown}");
        await userEvent.keyboard("{ArrowDown}");
        expect(focusedName()).toBe("1.4 GB per hour");
        expect(isRingShown()).toBe(true);
        await userEvent.keyboard("{Enter}");
        await closedOnto("Settings");
        expect(capsKept()).toEqual([3096]);
        expect(isRingShown()).toBe(true);
      } finally {
        input.stop();
      }
    });

    it(`walks the Sound menu by a remote's arrows and OK, each row one stop, at ${at}`, async () => {
      const noted = await drawnOn(screen);
      const input = startInput({ goBack: () => false, isNavigating: () => true });
      try {
        opener("Sound").focus();
        await userEvent.keyboard("{Enter}");
        await expect.poll(() => focusedName()).toBe("Sound track: English›");
        expect(isRingShown()).toBe(true);
        await userEvent.keyboard("{ArrowDown}");
        expect(document.activeElement?.getAttribute("role")).toBe("switch");
        expect(isRingShown()).toBe(true);
        await userEvent.keyboard("{Enter}");
        expect(noted.levelling).toEqual([true]);
        // Down into This device's row, then out of it to This file's, the row below it.
        await userEvent.keyboard("{ArrowDown}");
        const device = document.activeElement?.closest("li");
        expect(device?.textContent).toContain("This device");
        expect(isRingShown()).toBe(true);
        await userEvent.keyboard("{ArrowDown}");
        const file = document.activeElement?.closest("li");
        expect(file?.textContent).toContain("This file");
        expect(file).not.toBe(device);
        expect(isRingShown()).toBe(true);
        expect(isMenuOnScreen()).toBe(true);
      } finally {
        input.stop();
      }
    });
  }
});
