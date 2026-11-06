// SPDX-License-Identifier: MPL-2.0

/**
 * How a player's film sounds, as it tells its session, and a remote's choice of it (the server's
 * internal/core/together PlaybackSound and SoundChoice): a cast's remote shows the receiver's
 * dialogue boost, levelling and audio track and changes them, the receiver carrying each choice out
 * as its own press. A Fire TV's own controls are hidden while it is controlled, so its sound could
 * only be changed after taking control back (the owner, VQ116's run).
 */

/** One of a file's own audio tracks, as a remote names it. */
export interface SoundTrack {
  readonly streamId: number;
  /** BCP 47; empty where nothing names one. */
  readonly language: string;
  readonly containerTitle?: string | undefined;
  readonly kind: "plain" | "commentary" | "described";
}

/**
 * How a player's film sounds and what of it its person may choose, as its page offers: the dialogue
 * boost, undefined where the film has no boosted mix to switch to (D222, D336); levelling (D226);
 * the audio track playing, among the file's own (D230 §5); how far the device moves its sound from
 * its picture, later when positive, undefined where its page offers no such move (the owner,
 * 2026-11-06: "Put a control in watch together and cast").
 */
export interface PlaybackSound {
  readonly isBoostOn?: boolean | undefined;
  readonly isLevellingOn: boolean;
  readonly audioStreamId?: number | undefined;
  readonly audioTracks: readonly SoundTrack[];
  readonly soundOffsetMs?: number | undefined;
}

/** One choice of how a film sounds, made on a remote for its person's player to carry out. */
export type SoundChoice =
  | { readonly kind: "boost"; readonly isOn: boolean }
  | { readonly kind: "levelling"; readonly isOn: boolean }
  | { readonly kind: "audioTrack"; readonly streamId: number }
  | { readonly kind: "soundOffset"; readonly ms: number };

/**
 * A sound as a player sends it: the server takes isBoostOn's absence for no boost to switch, and
 * omits false and zero.
 */
export function wireSound(sound: PlaybackSound): PlaybackSound {
  return {
    ...(sound.isBoostOn === undefined ? {} : { isBoostOn: sound.isBoostOn }),
    isLevellingOn: sound.isLevellingOn,
    ...(sound.audioStreamId === undefined ? {} : { audioStreamId: sound.audioStreamId }),
    audioTracks: sound.audioTracks,
    ...(sound.soundOffsetMs === undefined ? {} : { soundOffsetMs: sound.soundOffsetMs }),
  };
}
