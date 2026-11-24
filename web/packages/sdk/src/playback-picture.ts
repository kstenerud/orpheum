// SPDX-License-Identifier: MPL-2.0

/**
 * What a player shows of its film that its person may choose, as it tells its session, and a
 * remote's choice of it (the server's internal/core/together PlaybackPicture and PictureChoice): a
 * cast's remote shows the receiver's subtitles and its offer of the film's own file, and changes
 * them, the receiver carrying each choice out as its own press (D440 §5), as it does the sound's
 * (playback-sound.ts).
 */

/** One subtitle track a player offers, as its page keys and names it, in its person's language. */
export interface SubtitleOption {
  readonly key: string;
  readonly label: string;
}

/**
 * What a player shows that its person may choose, as its page offers: the subtitle track showing,
 * by its page's key, undefined for none, among the tracks offered; how far its subtitles are moved
 * from the film, later when positive, as its subtitle timing stands, undefined for in step; and
 * whether its page offers the film's own file in place of its ladder (D418 §4), the one quality a
 * person chooses.
 */
export interface PlaybackPicture {
  readonly subtitleKey?: string | undefined;
  readonly subtitleOffsetMs?: number | undefined;
  readonly subtitles: readonly SubtitleOption[];
  readonly canPlayOriginal: boolean;
}

/**
 * One choice of what a film shows, made on a remote for its person's player to carry out: a subtitle
 * track, or none; the subtitles' timing, ms from the film, later when positive, as the player's own
 * timing sets it; the original; and word that a subtitle was fetched for the film (Find subtitles,
 * D224), which its player reads its tracks again to offer.
 */
export type PictureChoice =
  | { readonly kind: "subtitles"; readonly key: string | undefined }
  | { readonly kind: "subtitleOffset"; readonly ms: number }
  | { readonly kind: "original" }
  | { readonly kind: "subtitleFetched" };

/** A picture as a player sends it: the server takes an absent key for none, and omits false. */
export function wirePicture(picture: PlaybackPicture): PlaybackPicture {
  return {
    ...(picture.subtitleKey === undefined ? {} : { subtitleKey: picture.subtitleKey }),
    ...(picture.subtitleOffsetMs === undefined || picture.subtitleOffsetMs === 0
      ? {}
      : { subtitleOffsetMs: picture.subtitleOffsetMs }),
    subtitles: picture.subtitles,
    canPlayOriginal: picture.canPlayOriginal,
  };
}

/** A choice as a remote sends it: no key for subtitles off, no ms for subtitles in step. */
export function wirePictureChoice(choice: PictureChoice): Readonly<Record<string, unknown>> {
  switch (choice.kind) {
    case "subtitles":
      return choice.key === undefined
        ? { kind: choice.kind }
        : { kind: choice.kind, key: choice.key };
    case "subtitleOffset":
      return choice.ms === 0 ? { kind: choice.kind } : { kind: choice.kind, ms: choice.ms };
    case "original":
    case "subtitleFetched":
      return { kind: choice.kind };
  }
}
