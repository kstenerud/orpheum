// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { TogetherVoice, VoiceSnapshot } from "@orpheum/sdk";
import { createRef } from "react";
import { afterEach, describe, expect, it } from "vitest";

import type { PlaybackTarget, PlayerSound, TargetChoices } from "./playback-target.ts";
import { PlayerMenusContext } from "./player-menus.tsx";
import { PlayerTracksContext } from "./player-tracks.tsx";
import { TargetMenus } from "./target-menus.tsx";
import { buttonNamed, click, enter, mount, type Mounted } from "./test-dom.tsx";
import { tracksNoting, ways } from "./test-player-menus.tsx";
import { withServices } from "./test-services.tsx";
import { SessionVoiceContext, type VoiceInUse } from "./together-voice.tsx";

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

/** The menus over target, the film's tracks around them as on the Play page, in voice's session. */
async function drawn(target: PlaybackTarget, voice?: VoiceInUse): Promise<HTMLElement> {
  mounted = await mount(
    withServices(
      <SessionVoiceContext value={voice}>
        <PlayerTracksContext value={tracksNoting({ subtitles: [], audio: [] })}>
          <PlayerMenusContext value={ways}>
            <TargetMenus target={target} shown={["sound", "subtitles", "audio", "more"]} />
          </PlayerMenusContext>
        </PlayerTracksContext>
      </SessionVoiceContext>,
    ),
  );
  return mounted.container;
}

/** A Watch Together session's voice with partners in it, each gain set noted in set. */
function voiceWith(
  partners: readonly { readonly memberId: number; readonly name: string }[],
  set: [number, number][] = [],
): VoiceInUse {
  const snapshot = { status: "off", partners, partnerGains: {} } as unknown as VoiceSnapshot;
  const voice = {
    setPartnerGain: (memberId: number, gain: number) => {
      set.push([memberId, gain]);
    },
  } as unknown as TogetherVoice;
  return { voice, snapshot, tap: { isWaiting: false, play: () => undefined } };
}

const castee: TargetChoices = {
  kind: "castee",
  playback: { delivery: "direct" },
  connection: undefined,
  report: undefined,
};

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

  it("draws 🔈 over a castee in a Watch Together session with others' voices, each person's volume in it (the owner, 2026-11-26)", async () => {
    const set: [number, number][] = [];
    const menus = await drawn(targetOf(castee), voiceWith([{ memberId: 2, name: "Dan" }], set));

    await click(buttonNamed(menus, "Volume"));
    await enter(document.querySelector('input[aria-label="Dan\'s voice"]'), "40");
    expect(set.at(-1)?.[0]).toBe(2);
    expect(set.at(-1)?.[1]).toBeGreaterThan(0);
  });

  it("draws no 🔈 over a castee in a session where no one else's voice is to be mixed", async () => {
    const menus = await drawn(targetOf(castee), voiceWith([]));

    expect(buttonNamed(menus, "Volume")).toBeUndefined();
  });
});
