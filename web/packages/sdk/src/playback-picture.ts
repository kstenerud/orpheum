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
 * by its page's key, undefined for none, among the tracks offered; and whether its page offers the
 * film's own file in place of its ladder (D418 §4), the one quality a person chooses.
 */
export interface PlaybackPicture {
  readonly subtitleKey?: string | undefined;
  readonly subtitles: readonly SubtitleOption[];
  readonly canPlayOriginal: boolean;
}

/** One choice of what a film shows, made on a remote for its person's player to carry out. */
export type PictureChoice =
  { readonly kind: "subtitles"; readonly key: string | undefined } | { readonly kind: "original" };

/** A picture as a player sends it: the server takes an absent key for none, and omits false. */
export function wirePicture(picture: PlaybackPicture): PlaybackPicture {
  return {
    ...(picture.subtitleKey === undefined ? {} : { subtitleKey: picture.subtitleKey }),
    subtitles: picture.subtitles,
    canPlayOriginal: picture.canPlayOriginal,
  };
}

/** A choice as a remote sends it: no key for subtitles off. */
export function wirePictureChoice(choice: PictureChoice): Readonly<Record<string, unknown>> {
  return choice.kind === "subtitles" && choice.key !== undefined
    ? { kind: choice.kind, key: choice.key }
    : { kind: choice.kind };
}
