// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { SoundChoiceCause } from "@orpheum/player";
import { afterEach, describe, expect, it } from "vitest";

import { chooseSoundInPlace, ladderSoundLevelOf, playTrackChosen } from "./ladder-sound.ts";
import { PlayView } from "./play.tsx";
import { choose, mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { directDecision, unmeasuredSound } from "./test-fixtures.ts";
import { withServices } from "./test-services.tsx";
import type { PlayingSound } from "./tracks-note.tsx";
import type { PlaybackDecision } from "./watching.ts";

/**
 * A film's ladder's sounds: an English 5.1 track's standard mix and boosted one, levelled, carrying
 * the stereo mix's -3.5 dB; and a French stereo track's, levelled by its own -2 dB.
 */
function ladderTracks(): PlaybackDecision["tracks"] {
  const converted = { codec: "aac", channelCount: 2, kbps: 96, isOriginal: false };
  const english = { levelling: { isMeasured: true, gainDb: -3.5 }, carriedGainDb: -3.5 };
  return {
    ...directDecision().tracks,
    sounds: [
      { audioTrackIndex: 0, isBoosted: false, ...converted, stream: "sound0-96k", ...english },
      { audioTrackIndex: 0, isBoosted: true, ...converted, stream: "sound0-boost-96k", ...english },
      {
        audioTrackIndex: 1,
        isBoosted: false,
        ...converted,
        stream: "sound1-96k",
        levelling: { isMeasured: true, gainDb: -2 },
        carriedGainDb: -2,
      },
    ],
    fileSounds: [
      { audioTrackIndex: 0, codec: "eac3", channelCount: 6 },
      { audioTrackIndex: 1, codec: "aac", channelCount: 2 },
    ],
  };
}

describe("another audio track chosen on a film's ladder (audio-switch-and-retiming-2026-11-01.md §1.2)", () => {
  /** What playTrackChosen did: the sounds it played in place, with why, and whether it asked again. */
  function chosenWith(isInPlace: boolean): {
    readonly played: { sound: PlayingSound; cause: SoundChoiceCause }[];
    readonly asked: string[];
    readonly ways: Parameters<typeof playTrackChosen>[1];
  } {
    const played: { sound: PlayingSound; cause: SoundChoiceCause }[] = [];
    const asked: string[] = [];
    return {
      played,
      asked,
      ways: {
        playSound: (sound, cause) => {
          played.push({ sound, cause });
          return isInPlace;
        },
        askAgain: () => asked.push("again"),
      },
    };
  }

  it("is switched in place the boost's way, never asking for the film again, so no one's player reloads", () => {
    const { played, asked, ways } = chosenWith(true);
    playTrackChosen({ audioTrackIndex: 1, isBoostOn: false, tracks: ladderTracks() }, ways);
    expect(played).toEqual([
      { sound: { audioTrackIndex: 1, isBoosted: false }, cause: "trackChosen" },
    ]);
    expect(asked).toEqual([]);
  });

  it("plays the boosted mix where the person left the boost on and the track has one, the standard mix where it has none", () => {
    const { played, ways } = chosenWith(true);
    playTrackChosen({ audioTrackIndex: 0, isBoostOn: true, tracks: ladderTracks() }, ways);
    playTrackChosen({ audioTrackIndex: 1, isBoostOn: true, tracks: ladderTracks() }, ways);
    expect(played.map((each) => each.sound)).toEqual([
      { audioTrackIndex: 0, isBoosted: true },
      { audioTrackIndex: 1, isBoosted: false },
    ]);
  });

  it("asks for the film again where its player cannot switch in place: the element's own HLS, a ladder a stalled player moved to", () => {
    const { asked, ways } = chosenWith(false);
    playTrackChosen({ audioTrackIndex: 1, isBoostOn: false, tracks: ladderTracks() }, ways);
    expect(asked).toEqual(["again"]);
    const none = chosenWith(true);
    playTrackChosen(
      { audioTrackIndex: 1, isBoostOn: false, tracks: ladderTracks() },
      { ...none.ways, playSound: undefined },
    );
    expect(none.asked).toEqual(["again"]);
  });
});

describe("the gain of a sound switched to in place (audio-switch-and-retiming-2026-11-01.md §1.2, §1.5 Q4)", () => {
  it("is the heard sound's own track's levelling and carried gain, not the starting track's", () => {
    expect(ladderSoundLevelOf(ladderTracks(), { audioTrackIndex: 1, isBoosted: false })).toEqual({
      levelling: { isMeasured: true, gainDb: -2 },
      carriedGainDb: -2,
    });
    expect(ladderSoundLevelOf(ladderTracks(), { audioTrackIndex: 0, isBoosted: true })).toEqual({
      levelling: { isMeasured: true, gainDb: -3.5 },
      carriedGainDb: -3.5,
    });
    expect(ladderSoundLevelOf(undefined, { audioTrackIndex: 0, isBoosted: false })).toBeUndefined();
  });

  it("changes when the new sound is heard, never as it is chosen, so the old sound never plays at the new one's gain", () => {
    let onHeard: (() => void) | undefined;
    const choices: { index: number; cause: SoundChoiceCause }[] = [];
    const playback = {
      canSwitchSoundInPlace: true,
      setAudioRendition: (index: number, cause: SoundChoiceCause, heard?: () => void) => {
        choices.push({ index, cause });
        onHeard = heard;
      },
    };
    const heard: string[] = [];
    const isInPlace = chooseSoundInPlace(playback, 2, {
      cause: "trackChosen",
      heard: () => heard.push("French"),
    });
    expect(isInPlace).toBe(true);
    expect(choices).toEqual([{ index: 2, cause: "trackChosen" }]);
    expect(heard).toEqual([]);
    onHeard?.();
    expect(heard).toEqual(["French"]);
  });

  it("chooses nothing where the player cannot switch in place, or the ladder names no such sound", () => {
    const choices: number[] = [];
    const native = {
      canSwitchSoundInPlace: false,
      setAudioRendition: (index: number) => choices.push(index),
    };
    const choice = { cause: "trackChosen" as const, heard: () => undefined };
    expect(chooseSoundInPlace(native, 2, choice)).toBe(false);
    expect(chooseSoundInPlace({ ...native, canSwitchSoundInPlace: true }, undefined, choice)).toBe(
      false,
    );
    expect(chooseSoundInPlace(undefined, 2, choice)).toBe(false);
    expect(choices).toEqual([]);
  });
});

describe("another audio track chosen in a file played as it is (the owner's answers of 2026-10-31)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  /** The original with an English and a French track, each named by its stream and levelled by its own. */
  function twoOwnTracks(): PlaybackDecision {
    const plain = directDecision();
    const own = { codec: "aac", channelCount: 2, kbps: 0, isOriginal: true, isBoosted: false };
    return directDecision({
      audioTracks: [
        { streamId: 80, audioTrackIndex: 0, language: "en", kind: "plain" },
        { streamId: 81, audioTrackIndex: 1, language: "fr", kind: "plain" },
      ],
      startingAudioStreamId: 80,
      tracks: {
        ...plain.tracks,
        sounds: [
          { audioTrackIndex: 0, ...own, ...unmeasuredSound, stream: "sound0-own" },
          { audioTrackIndex: 1, ...own, ...unmeasuredSound, stream: "sound1-own" },
        ],
      },
    });
  }

  /** The routes a direct playback's page asks for. */
  const routes = {
    "PUT /files/2/preferences": [204, undefined],
    "PUT /playback/t/interest": [204, null],
    "GET /files/2/waits": [200, {}],
    "GET /files/2/up-next": [200, {}],
    "DELETE /playback/t": [204, null],
  } as const;

  it("switches the element's own track in place where it lists them, as Safari's does, and says the sound it plays", async () => {
    const heard: Heard[] = [];
    const asked: string[] = [];
    mounted = await mount(
      withServices(
        <PlayView
          fileId={2}
          decision={twoOwnTracks()}
          onDecideAgain={(cause) => asked.push(cause)}
        />,
        servedBy(routes, heard),
      ),
    );
    const video = mounted.container.querySelector("video");
    const tracks = { length: 2, 0: { enabled: true }, 1: { enabled: false } };
    Object.defineProperty(video, "audioTracks", { value: tracks, configurable: true });
    await choose(mounted.container.querySelector("select"), "81");
    expect(asked).toEqual([]);
    expect([tracks[0].enabled, tracks[1].enabled]).toEqual([false, true]);
    expect(heard).toContainEqual({
      methodAndPath: "PUT /playback/t/interest",
      body: { soundStream: "sound1-own", positionMs: 0, isPaused: true },
    });
  });

  it("asks for the film again, onto its ladder, where the element lists no tracks, as Chrome's and Firefox's do not", async () => {
    const asked: string[] = [];
    mounted = await mount(
      withServices(
        <PlayView
          fileId={2}
          decision={twoOwnTracks()}
          onDecideAgain={(cause) => asked.push(cause)}
        />,
        servedBy(routes),
      ),
    );
    await choose(mounted.container.querySelector("select"), "81");
    expect(asked).toEqual(["audioChosen"]);
  });
});
