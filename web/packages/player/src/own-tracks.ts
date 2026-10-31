// SPDX-License-Identifier: MPL-2.0

/** One of the element's audio tracks, as its list gives it: whether it plays (the DOM's `enabled`). */
export type OwnAudioTrack = Record<"enabled", boolean>;

/**
 * The element's own list of its file's audio tracks (`audioTracks`), in the file's order: Safari's;
 * Chrome and Firefox keep it behind a flag.
 */
export interface OwnAudioTracks {
  readonly length: number;
  readonly [index: number]: OwnAudioTrack | undefined;
}

/** Whether value is a list of tracks: an object with a length. */
function isTrackList(value: unknown): value is OwnAudioTracks {
  return (
    typeof value === "object" &&
    value !== null &&
    "length" in value &&
    typeof value.length === "number"
  );
}

/**
 * The element's own audio tracks where it lists them, else undefined: found by the feature, never the
 * browser's name.
 */
export function ownAudioTracksOf(video: HTMLMediaElement): OwnAudioTracks | undefined {
  const tracks: unknown = Reflect.get(video, "audioTracks");
  return isTrackList(tracks) ? tracks : undefined;
}

/**
 * Switches a file played as it is to its audio track at index, in place, where the element lists its
 * tracks: that one enabled, every other not, so the picture plays on and the new sound is heard at
 * once (the owner's answers of 2026-10-31). Returns whether it did; where it cannot, the page moves
 * the film onto its ladder, as a track chosen did before.
 */
export function switchOwnTrack(tracks: OwnAudioTracks | undefined, index: number): boolean {
  if (tracks === undefined || index < 0 || index >= tracks.length || tracks[index] === undefined) {
    return false;
  }
  for (let place = 0; place < tracks.length; place += 1) {
    const track = tracks[place];
    if (track !== undefined) {
      track.enabled = place === index;
    }
  }
  return true;
}
