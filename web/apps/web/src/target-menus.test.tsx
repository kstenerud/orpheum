// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { createRef } from "react";
import { afterEach, describe, expect, it } from "vitest";

import type { PlaybackTarget, PlayerSound, TargetChoices } from "./playback-target.ts";
import { PlayerMenusContext } from "./player-menus.tsx";
import { PlayerTracksContext } from "./player-tracks.tsx";
import { TargetMenus } from "./target-menus.tsx";
import { buttonNamed, mount, type Mounted } from "./test-dom.tsx";
import { tracksNoting, ways } from "./test-player-menus.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const sound: PlayerSound = {
  volume: 0.5,
  isMuted: false,
  canSetVolume: true,
  setVolume: () => undefined,
  setMuted: () => undefined,
};

/** A target playing at a minute, its sound sound, its choices choices. */
function targetOf(choices: TargetChoices): PlaybackTarget {
  return {
    atMs: 60_000,
    reportedAtMs: 60_000,
    durationMs: 600_000,
    isPaused: false,
    isReady: true,
    playOrPause: () => undefined,
    seekTo: () => undefined,
    sound,
    kind: choices.kind,
    choices,
  };
}

/** The menus over target, the film's tracks around them as on the Play page. */
async function drawn(target: PlaybackTarget): Promise<HTMLElement> {
  mounted = await mount(
    withServices(
      <PlayerTracksContext value={tracksNoting({ subtitles: [], audio: [] })}>
        <PlayerMenusContext value={ways}>
          <TargetMenus target={target} shown={["sound", "subtitles", "audio", "more"]} />
        </PlayerMenusContext>
      </PlayerTracksContext>,
    ),
  );
  return mounted.container;
}

describe("the menus over the playback target (D555 §8, §10)", () => {
  it("draws the video's menus over the video here, 🔈 its sound and its subtitle tracks", async () => {
    const menus = await drawn(targetOf({ kind: "here", videoRef: createRef() }));

    expect(buttonNamed(menus, "Volume")).toBeDefined();
    expect(menus.querySelector('[aria-label^="Subtitles"]')).not.toBeNull();
  });

  it("draws over a castee only what it says, no 🔈 and none of the video's tracks", async () => {
    const menus = await drawn(
      targetOf({
        kind: "castee",
        playback: { delivery: "direct" },
        connection: undefined,
        report: undefined,
      }),
    );

    expect(buttonNamed(menus, "Volume")).toBeUndefined();
    expect(menus.querySelector('[aria-label^="Subtitles"]')).toBeNull();
    expect(buttonNamed(menus, "Settings")).toBeDefined();
  });
});
