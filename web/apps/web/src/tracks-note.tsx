// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { rateText } from "./rate-text.ts";

type Tracks = components["schemas"]["TracksBody"];
type PictureTrack = components["schemas"]["PictureTrackBody"];
type SoundTrack = components["schemas"]["SoundTrackBody"];
type FilePicture = components["schemas"]["FilePictureBody"];
type FileSound = components["schemas"]["FileSoundBody"];

/** Which of a file's sounds plays: its audio track, and whether its mix with the dialogue boost. */
export interface PlayingSound {
  readonly audioTrackIndex: number;
  readonly isBoosted: boolean;
}

/** What a codec is called, by ffprobe's name for it; one not named here, in capitals. */
const codecNames: Readonly<Record<string, string>> = {
  hevc: "HEVC",
  h264: "H.264",
  av1: "AV1",
  vp9: "VP9",
  vp8: "VP8",
  mpeg2video: "MPEG-2",
  mpeg4: "MPEG-4",
  vc1: "VC-1",
  eac3: "E-AC-3",
  ac3: "AC-3",
  aac: "AAC",
  truehd: "TrueHD",
  dts: "DTS",
  flac: "FLAC",
  opus: "Opus",
  mp3: "MP3",
  vorbis: "Vorbis",
};

/** A codec's name as a person reads it. */
export function codecName(codec: string): string {
  return codecNames[codec] ?? (codec.startsWith("pcm_") ? "PCM" : codec.toUpperCase());
}

/** A count of channels as a person reads it: mono, stereo, 5.1, 7.1, or so many channels. */
function channelsText(t: TFunction, count: number): string {
  const named: Readonly<Record<number, string>> = {
    1: "mono",
    2: "stereo",
    6: "surround51",
    8: "surround71",
  };
  return t(`play.tracks.channels.${named[count] ?? "other"}`, { count });
}

/** A picture as its line names it: codec, size and rate, as far as each is known. */
function pictureWhat(picture: Pick<PictureTrack, "codec" | "width" | "height" | "kbps">): string {
  const size =
    picture.width > 0 && picture.height > 0
      ? ` ${String(picture.width)}×${String(picture.height)}`
      : "";
  const rate = picture.kbps > 0 ? `, ${rateText(picture.kbps)}` : "";
  return `${codecName(picture.codec)}${size}${rate}`;
}

/** A sound as its line names it: codec, channels and rate, as far as each is known. */
function soundWhat(
  t: TFunction,
  sound: Pick<SoundTrack, "codec" | "channelCount" | "kbps">,
): string {
  const rate = sound.kbps > 0 ? `, ${String(sound.kbps)} kb/s` : "";
  return `${codecName(sound.codec)} ${channelsText(t, sound.channelCount)}${rate}`;
}

/** A picture's line: as it plays, and whether it is the original or transcoded from the file's. */
export function pictureLine(t: TFunction, picture: PictureTrack, file: FilePicture): string {
  const what = pictureWhat(picture);
  return picture.isOriginal
    ? t("play.tracks.original", { what })
    : t("play.tracks.transcoded", { what, from: pictureWhat({ ...file, kbps: 0 }) });
}

/** A sound's line: as it plays, and whether it is the original or transcoded from the file's track. */
export function soundLine(t: TFunction, sound: SoundTrack, file: FileSound | undefined): string {
  const what = soundWhat(t, sound);
  if (sound.isOriginal || file === undefined) {
    return t("play.tracks.original", { what });
  }
  const from = soundWhat(t, { ...file, kbps: 0 });
  return t(sound.isBoosted ? "play.tracks.transcodedBoosted" : "play.tracks.transcoded", {
    what,
    from,
  });
}

/**
 * The picture playing: on a film's ladder, the one at the height its player's level plays, or none
 * when none is; off it, the one there is.
 */
export function playingPicture(
  tracks: Tracks,
  height: number | undefined,
): PictureTrack | undefined {
  const pictures = tracks.pictures ?? [];
  return height === undefined ? pictures[0] : pictures.find((picture) => picture.height === height);
}

/**
 * The sound playing: the track chosen, its boosted mix when the boost is on and it has one; among the
 * sounds the picture playing plays of its own where it names them, the top of a ladder its tracks'
 * own copied (D368 §1), else among the playback's.
 */
export function playingSound(
  tracks: Tracks,
  chosen: PlayingSound,
  picture?: PictureTrack,
): SoundTrack | undefined {
  const sounds = picture?.sounds ?? tracks.sounds ?? [];
  const ofTrack = sounds.filter((sound) => sound.audioTrackIndex === chosen.audioTrackIndex);
  return ofTrack.find((sound) => sound.isBoosted === chosen.isBoosted) ?? ofTrack[0] ?? sounds[0];
}

/**
 * The tracks a film plays, each as it plays and whether it is the file's own or transcoded, and from
 * what (the owner, 2026-10-20: "show which tracks are original, and which are transcoded, and from
 * what").
 */
export function TracksNote(props: {
  readonly tracks: Tracks | undefined;
  /** The height the player's level plays at, on a film's ladder. */
  readonly height?: number | undefined;
  readonly sound: PlayingSound;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { tracks } = props;
  if (tracks === undefined) {
    return null;
  }
  const picture = playingPicture(tracks, props.height);
  const sound = playingSound(tracks, props.sound, picture);
  const file = (tracks.fileSounds ?? []).find(
    (each) => each.audioTrackIndex === sound?.audioTrackIndex,
  );
  return (
    <div className="play__tracks">
      <p>{t("play.tracks.heading")}</p>
      <ul>
        {picture === undefined ? null : (
          <li>{t("play.tracks.video", { line: pictureLine(t, picture, tracks.filePicture) })}</li>
        )}
        {sound === undefined ? null : (
          <li>{t("play.tracks.audio", { line: soundLine(t, sound, file) })}</li>
        )}
      </ul>
    </div>
  );
}
