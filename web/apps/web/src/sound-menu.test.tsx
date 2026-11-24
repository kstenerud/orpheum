// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, createRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { forgetFileSoundOffsetFailure } from "./file-sound-offset.tsx";
import { PlayerMenus, PlayerMenusContext, type PlayerMenusWays } from "./player-menus.tsx";
import { PlayerTracksContext, type PlayerTracks } from "./player-tracks.tsx";
import { forgetSoundOffsetSetInThisTab, soundOffsetKeptAfterMs } from "./sound-delay.tsx";
import { buttonNamed, fire, mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { IsAdminContext } from "./warning-sign.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  forgetSoundOffsetSetInThisTab();
  forgetFileSoundOffsetFailure();
  vi.useRealTimers();
});

const off = { isOn: false, switchTo: () => undefined, problem: undefined };

/** A film's tracks with one audio track and no subtitles. */
const tracks: PlayerTracks = {
  subtitles: {
    tracks: [],
    chosenTrackKey: undefined,
    choose: () => undefined,
    offsetMs: 0,
    shiftOffset: () => undefined,
    failure: undefined,
  },
  audio: {
    tracks: [{ streamId: 80, audioTrackIndex: 0, language: "en", kind: "plain" }],
    chosenStreamId: 80,
    choose: () => undefined,
    problem: undefined,
  },
  levelling: off,
  level: { levelling: { isMeasured: true, gainDb: 3 }, carriedGainDb: 0 },
  measuring: undefined,
  boost: undefined,
};

/** The ways of a playback of file 3, its own offset everyone's 0 and the person's ownMs. */
function waysOwning(ownMs: number | undefined): PlayerMenusWays {
  return {
    fileId: 3,
    soundOffsetMs: 0,
    fileSoundOffset: {
      everyonesMs: 0,
      soundOffsetMs: ownMs ?? 0,
      ...(ownMs === undefined ? {} : { ownMs }),
    },
    onSubtitleFetched: () => undefined,
  };
}

/** Draws the overlay's menus, the Sound menu open, as an admin or not, served routes. */
async function soundMenuOpen(
  ways: PlayerMenusWays,
  isAdmin: boolean,
  served: { readonly routes: Parameters<typeof servedBy>[0]; readonly heard: Heard[] },
): Promise<Mounted> {
  mounted = await mount(
    withServices(
      <IsAdminContext value={isAdmin}>
        <PlayerTracksContext value={tracks}>
          <PlayerMenusContext value={ways}>
            <PlayerMenus
              videoRef={createRef()}
              shown={["audio", "more"]}
              onOpenChange={() => undefined}
            />
          </PlayerMenusContext>
        </PlayerTracksContext>
      </IsAdminContext>,
      servedBy(served.routes, served.heard),
    ),
  );
  const opener = mounted.container.querySelector('[aria-label="Sound"]');
  await fire(opener, new MouseEvent("click", { bubbles: true }));
  return mounted;
}

function press(on: Mounted, name: string): void {
  const button = buttonNamed(on.container, name);
  if (button === undefined) {
    throw new Error(`no button ${name}`);
  }
  act(() => {
    button.click();
  });
}

/** The open menu's choice whose text is text, else undefined. */
function choiceNamed(on: Mounted, text: string): HTMLElement | undefined {
  return [...on.container.querySelectorAll<HTMLElement>('dialog .list-box [role="button"]')].find(
    (each) => each.textContent === text,
  );
}

describe("the Sound menu's timing (D537, D538 §8)", () => {
  it("shows This device and This file apart, and keeps a step of This file as the person's own", async () => {
    const heard: Heard[] = [];
    const on = await soundMenuOpen(waysOwning(undefined), false, {
      routes: { "PUT /files/3/preferences": [204, undefined] },
      heard,
    });
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    // Its heading names what it times (the owner, 2026-11-23), over This device and This file.
    expect(on.container.querySelector(".sound-menu__heading")?.textContent).toBe("Sound timing");
    press(on, "This file later");
    press(on, "This file later");
    expect(on.container.textContent).toContain("Sound plays 100 ms later in this file");
    expect(on.container.textContent).not.toContain("100 ms later on this device");
    await act(async () => {
      await vi.advanceTimersByTimeAsync(soundOffsetKeptAfterMs);
    });
    expect(heard).toEqual([
      { methodAndPath: "PUT /files/3/preferences", body: { soundOffsetMs: 100 } },
    ]);
  });

  it("offers anyone but an admin Tell the admin, a report suggesting their own, where it is not everyone's", async () => {
    const on = await soundMenuOpen(waysOwning(-150), false, { routes: {}, heard: [] });
    expect(choiceNamed(on, "Make it everyone's")).toBeUndefined();
    await fire(choiceNamed(on, "Tell the admin"), new MouseEvent("click", { bubbles: true }));
    expect(on.container.textContent).toContain(
      "It suggests this file's sound play 150 ms earlier for everyone.",
    );
  });

  it("offers an admin Make it everyone's, which sets every person's to theirs and forgets their own", async () => {
    const heard: Heard[] = [];
    const on = await soundMenuOpen(waysOwning(200), true, {
      routes: {
        "PUT /files/3/sound-offset": [204, undefined],
        "PUT /files/3/preferences": [204, undefined],
      },
      heard,
    });
    expect(choiceNamed(on, "Tell the admin")).toBeUndefined();
    await fire(choiceNamed(on, "Make it everyone's"), new MouseEvent("click", { bubbles: true }));
    await act(async () => {
      await Promise.resolve();
    });
    expect(heard).toEqual([
      { methodAndPath: "PUT /files/3/sound-offset", body: { soundOffsetMs: 200 } },
      { methodAndPath: "PUT /files/3/preferences", body: { isSoundOffsetEveryones: true } },
    ]);
  });

  it("offers neither where the person's own is every person's", async () => {
    const on = await soundMenuOpen(waysOwning(undefined), true, { routes: {}, heard: [] });
    expect(choiceNamed(on, "Make it everyone's")).toBeUndefined();
    expect(choiceNamed(on, "Tell the admin")).toBeUndefined();
  });
});
