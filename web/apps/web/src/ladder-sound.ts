// SPDX-License-Identifier: MPL-2.0

import {
  ownAudioTracksOf,
  switchOwnTrack,
  type ChunkedPlayback,
  type SoundChoiceCause,
} from "@orpheum/player";
import { useState, type RefObject } from "react";

import { startingAudioTrack, type AudioTrack } from "./audio-tracks.tsx";
import { sendInterest } from "./interest.ts";
import { soundLevelOf, type SoundLevel } from "./levelling.tsx";
import { useServices } from "./services.tsx";
import type { PlayingSound } from "./tracks-note.tsx";
import type { PlaybackDecision } from "./watching.ts";

// A film's sounds as the Play page plays and switches them: each audio rendition of its ladder by its
// place, what levels it, and the one way a sound changes as the film plays, the dialogue boost turned
// on or off or another audio track chosen (audio-switch-and-retiming-2026-11-01.md §1.2).

/**
 * The audio rendition of a film's ladder that plays sound: its place among the ladder's sounds, which
 * its tracks list in its master playlist's order, each track's standard mix then its boosted one,
 * both made by renditions.AudioRenditionsOf; undefined where the ladder has no such mix, or the
 * tracks are the file's own.
 */
export function ladderAudioRenditionIndex(
  tracks: PlaybackDecision["tracks"] | undefined,
  sound: PlayingSound,
): number | undefined {
  const index = (tracks?.sounds ?? []).findIndex(
    (each) =>
      !each.isOriginal &&
      each.audioTrackIndex === sound.audioTrackIndex &&
      each.isBoosted === sound.isBoosted,
  );
  return index < 0 ? undefined : index;
}

/**
 * The level of a sound on a film's ladder, as its tracks say it: its own track's levelling and the
 * gain its chunks carry (D226 §4; audio-switch-and-retiming-2026-11-01.md §1.2); undefined where the
 * tracks name no such sound.
 */
export function ladderSoundLevelOf(
  tracks: PlaybackDecision["tracks"] | undefined,
  sound: PlayingSound | undefined,
): SoundLevel | undefined {
  const index = sound === undefined ? undefined : ladderAudioRenditionIndex(tracks, sound);
  const named = index === undefined ? undefined : tracks?.sounds?.[index];
  return named === undefined
    ? undefined
    : { levelling: named.levelling, carriedGainDb: named.carriedGainDb };
}

/**
 * Chooses sound in place in playback for cause, and tells heard once it is heard: false, choosing
 * nothing, where the ladder names no such sound (index undefined) or playback cannot switch in place.
 */
export function chooseSoundInPlace(
  playback: Pick<ChunkedPlayback, "setAudioRendition" | "canSwitchSoundInPlace"> | undefined,
  index: number | undefined,
  choice: { readonly cause: SoundChoiceCause; readonly heard: () => void },
): boolean {
  if (playback === undefined || index === undefined || !playback.canSwitchSoundInPlace) {
    return false;
  }
  playback.setAudioRendition(index, choice.cause, choice.heard);
  return true;
}

/**
 * Why the ladder's sound changes as it plays, for its player's note, where the boost is turned on or
 * off; another track chosen is "trackChosen" (LadderUnder).
 */
export function soundChoiceCause(chosen: PlayingSound): SoundChoiceCause {
  return chosen.isBoosted ? "boostOn" : "boostOff";
}

/**
 * The audio rendition a film's ladder starts with, for its sound: the track's boosted mix when the
 * boost is on and the ladder has it; else undefined, its master playlist's default, the track's
 * standard mix (chunks.MasterPlaylist).
 */
export function ladderStartingAudio(
  tracks: PlaybackDecision["tracks"] | undefined,
  sound: PlayingSound | undefined,
): number | undefined {
  return sound?.isBoosted === true ? ladderAudioRenditionIndex(tracks, sound) : undefined;
}

/**
 * Whether the track the original plays has a centre for the dialogue boost to raise: more than two
 * channels, as the server's boosted mix needs (D222, D336 §2).
 */
export function hasACentre(decision: PlaybackDecision): boolean {
  const index = startingAudioTrack(decision)?.audioTrackIndex ?? 0;
  const sound = (decision.tracks.fileSounds ?? []).find((each) => each.audioTrackIndex === index);
  return sound !== undefined && sound.channelCount > 2;
}

/**
 * Plays an audio track chosen on a film's ladder in place, the way the boost is switched: its boosted
 * mix where the person left the boost on and the track has one, else its standard mix. Where the
 * player cannot switch in place, the element's own HLS or a ladder a stalled player moved to, whose
 * sounds no decision named, the film is asked for again from where it stands, as before
 * (audio-switch-and-retiming-2026-11-01.md §1.2, §1.5 Q3).
 */
export function playTrackChosen(
  chosen: {
    readonly audioTrackIndex: number;
    readonly isBoostOn: boolean;
    readonly tracks: PlaybackDecision["tracks"];
  },
  ways: {
    readonly playSound: ((sound: PlayingSound, cause: SoundChoiceCause) => boolean) | undefined;
    readonly askAgain: () => void;
  },
): void {
  const { audioTrackIndex, isBoostOn, tracks } = chosen;
  const isBoosted =
    isBoostOn &&
    ladderAudioRenditionIndex(tracks, { audioTrackIndex, isBoosted: true }) !== undefined;
  if (ways.playSound?.({ audioTrackIndex, isBoosted }, "trackChosen") !== true) {
    ways.askAgain();
  }
}

/**
 * The level of a ladder's sound: the gain its chunks carry, levelled; or, for a sound that carries
 * none, the film's own levelling from the decision, so a person who plays without levelling, whose
 * sound is offered unlevelled, is told it plays as mixed and can turn levelling on (D226 §4).
 */
export function ladderSoundLevel(decision: PlaybackDecision, carriedGainDb: number): SoundLevel {
  return carriedGainDb === 0
    ? { levelling: decision.levelling, carriedGainDb: 0 }
    : { levelling: { isMeasured: true, gainDb: carriedGainDb }, carriedGainDb };
}

/**
 * Another audio track chosen in a file played as it is (DirectUnder): switched in place where the
 * element lists its file's tracks, as Safari's does, levelled by its own and said to the server as the
 * sound playing, whose stall's offer then starts it; else askAgain, the film moving onto its ladder
 * once, as Chrome and Firefox do (the owner's answers of 2026-10-31). Returns the level of the sound
 * playing and the handler for a choice.
 */
export function useOwnTrackChoice(
  decision: PlaybackDecision,
  videoRef: RefObject<HTMLVideoElement | null>,
  askAgain: () => void,
): { readonly level: SoundLevel; readonly onAudioChosen: (track: AudioTrack) => void } {
  const { api } = useServices();
  const [playingTrack, setPlayingTrack] = useState<number | undefined>(undefined);
  const sounds = decision.tracks.sounds ?? [];
  const playing = sounds.find((sound) => sound.audioTrackIndex === playingTrack);
  const onAudioChosen = (track: AudioTrack): void => {
    const video = videoRef.current;
    const chosen = sounds.find((sound) => sound.audioTrackIndex === track.audioTrackIndex);
    if (
      video === null ||
      chosen === undefined ||
      !switchOwnTrack(ownAudioTracksOf(video), track.audioTrackIndex)
    ) {
      askAgain();
      return;
    }
    setPlayingTrack(track.audioTrackIndex);
    sendInterest(api, decision.mediaToken, {
      soundStream: chosen.stream,
      positionMs: Math.round(video.currentTime * 1000),
      isPaused: video.paused,
    });
  };
  const level =
    playing === undefined
      ? soundLevelOf(decision)
      : { levelling: playing.levelling, carriedGainDb: playing.carriedGainDb };
  return { level, onAudioChosen };
}
