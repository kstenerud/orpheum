// SPDX-License-Identifier: MPL-2.0
// @vitest-environment happy-dom

import { createApi, type PlaybackSound } from "@orpheum/sdk";
import { describe, expect, it } from "vitest";

import { ReceiverSoundPanel, type SoundDecision, type SwitchesSound } from "./sound-panel.ts";

/** One of the ladder's sounds, as the core names it. */
type LadderSound = NonNullable<NonNullable<SoundDecision["tracks"]>["sounds"]>[number];

/** One of the ladder's sounds, as the core names it, its levelling measured. */
function ladderSound(fields: {
  readonly audioTrackIndex: number;
  readonly isBoosted?: boolean;
  readonly isOriginal?: boolean;
  readonly carriedGainDb?: number;
  readonly gainDb?: number;
}): LadderSound {
  return {
    audioTrackIndex: fields.audioTrackIndex,
    isBoosted: fields.isBoosted ?? false,
    isOriginal: fields.isOriginal ?? false,
    carriedGainDb: fields.carriedGainDb ?? 0,
    levelling: { isMeasured: true, gainDb: fields.gainDb ?? 6 },
  } as unknown as LadderSound;
}

/**
 * A film of two audio tracks, English (stream 2) and German (stream 3); its ladder: the original's
 * sound, English, English boosted, German; the person's boost and levelling as given.
 */
function twoTracks(preferences: {
  isDialogueBoostOn: boolean;
  isLevellingOn: boolean;
}): SoundDecision {
  return {
    audioTracks: [
      { audioTrackIndex: 0, streamId: 2, language: "en", kind: "plain" },
      { audioTrackIndex: 1, streamId: 3, language: "de", kind: "plain", containerTitle: "Dub" },
    ],
    startingAudioStreamId: 2,
    preferences,
    tracks: {
      sounds: [
        ladderSound({ audioTrackIndex: 0, isOriginal: true }),
        ladderSound({ audioTrackIndex: 0 }),
        ladderSound({ audioTrackIndex: 0, isBoosted: true, carriedGainDb: 6 }),
        ladderSound({ audioTrackIndex: 1, gainDb: 3 }),
      ],
    },
  } as unknown as SoundDecision;
}

/** A panel on a playing film, with what it asked of the player, the gain, the offset and the core. */
function panelOn(decision: SoundDecision) {
  const switched: string[] = [];
  const gains: number[] = [];
  const offsets: number[] = [];
  const kept: string[] = [];
  const api = createApi({
    origin: "https://core.example",
    getToken: () => "token",
    fetch: async (request) => {
      kept.push(`${request.method} ${new URL(request.url).pathname} ${await request.text()}`);
      return Response.json({});
    },
  });
  const panel = new ReceiverSoundPanel(
    decision,
    { api, fileId: 7, video: document.createElement("video"), note: () => undefined },
    {
      gainOf: () => ({ setDb: (db) => gains.push(db) }),
      offsetOf: () => ({ setMs: (ms) => offsets.push(ms) }),
    },
  );
  const playback: SwitchesSound = {
    canSwitchSoundInPlace: true,
    setAudioRendition: (index, cause, onHeard) => {
      switched.push(`${String(index)} ${cause}`);
      onHeard?.();
    },
  };
  return { panel, playback, switched, gains, offsets, kept };
}

describe("a Chromecast's sound, as its cast's remote works it", () => {
  it("offers the file's tracks its ladder has, the boost and levelling, as the Play page's menu", () => {
    const { panel, playback } = panelOn(
      twoTracks({ isDialogueBoostOn: false, isLevellingOn: true }),
    );
    panel.plays(playback);
    expect(panel.read()).toEqual<PlaybackSound>({
      isBoostOn: false,
      isLevellingOn: true,
      audioStreamId: 2,
      audioTracks: [
        { streamId: 2, language: "en", kind: "plain" },
        { streamId: 3, language: "de", kind: "plain", containerTitle: "Dub" },
      ],
      soundOffsetMs: 0,
    });
  });

  it("starts with the boosted mix where the person's boost is on", () => {
    const boosted = panelOn(twoTracks({ isDialogueBoostOn: true, isLevellingOn: false }));
    expect(boosted.panel.startingSound()).toEqual({ startAudioRenditionIndex: 2 });
    const plain = panelOn(twoTracks({ isDialogueBoostOn: false, isLevellingOn: false }));
    expect(plain.panel.startingSound()).toEqual({});
  });

  it("switches track and boost in place, kept for the person, and offers no boost a track lacks", async () => {
    const { panel, playback, switched, kept } = panelOn(
      twoTracks({ isDialogueBoostOn: false, isLevellingOn: false }),
    );
    panel.plays(playback);
    panel.choose({ kind: "boost", isOn: true });
    panel.choose({ kind: "audioTrack", streamId: 3 });
    expect(switched).toEqual(["2 boostOn", "3 trackChosen"]);
    expect(panel.read()?.audioStreamId).toBe(3);
    expect(panel.read()?.isBoostOn).toBeUndefined();
    await expect
      .poll(() => kept)
      .toEqual([
        'PUT /api/v1/me/preferences {"isDialogueBoostOn":true}',
        'PUT /api/v1/files/7/preferences {"audioChoice":{"streamId":3}}',
      ]);
  });

  it("levels the sound by a gain after the volume, less what its mix carries, made only once needed", () => {
    const { panel, playback, gains } = panelOn(
      twoTracks({ isDialogueBoostOn: false, isLevellingOn: false }),
    );
    panel.plays(playback);
    expect(gains).toEqual([]);
    panel.choose({ kind: "levelling", isOn: true });
    expect(gains).toEqual([6]);
    // The boosted mix carries its 6 dB: levelling on adds nothing more, off takes it back out.
    panel.choose({ kind: "boost", isOn: true });
    panel.choose({ kind: "levelling", isOn: false });
    expect(gains).toEqual([6, 0, -6]);
  });

  it("moves its sound from the picture as the remote's timing says, told to the remote", () => {
    const { panel, playback, offsets } = panelOn(
      twoTracks({ isDialogueBoostOn: false, isLevellingOn: false }),
    );
    panel.plays(playback);
    let told = 0;
    panel.subscribe(() => {
      told += 1;
    });
    panel.choose({ kind: "soundOffset", ms: 250 });
    expect(offsets).toEqual([250]);
    expect(panel.read()?.soundOffsetMs).toBe(250);
    expect(told).toBe(1);
  });

  it("says no sound for a film the ladder has none of, as from an older core", () => {
    expect(panelOn({}).panel.read()).toBeUndefined();
  });
});
