// SPDX-License-Identifier: MPL-2.0

/**
 * The film's own channel of a receiver's sound, as its cast's remote shows and sets it (D440 §2,
 * §5): the film's volume as the player's own slider sets it, and its mute, apart from the TV's own
 * volume; they last for the session and are not saved. Framework-free, so every receiver reads and
 * sets them alike (D555 §3); the web receiver's hook is a thin wrapper (cast-receiver-sound.ts).
 */

import type { SoundChannel } from "@orpheum/sdk";

import { heardVolumeOf, setHeardVolume } from "./heard-volume.ts";

/** The film's channel as it stands: the volume the person set and whether it is muted. */
export type FilmChannel = Pick<SoundChannel, "level" | "isMuted">;

/** The film's channel on video now: the volume the person set, not the element's (D436). */
export function filmChannelOf(video: HTMLMediaElement): FilmChannel {
  return { level: heardVolumeOf(video), isMuted: video.muted };
}

/**
 * Calls heard with the film's channel on video now, and again at each volumechange. Returns the
 * stopping.
 */
export function hearFilmChannel(
  video: HTMLMediaElement,
  heard: (channel: FilmChannel) => void,
): () => void {
  const read = (): void => {
    heard(filmChannelOf(video));
  };
  read();
  video.addEventListener("volumechange", read);
  return () => {
    video.removeEventListener("volumechange", read);
  };
}

/** Carries a remote's choice of the film's channel out on video: its level and its mute. */
export function chooseFilmChannel(video: HTMLMediaElement, choice: FilmChannel): void {
  setHeardVolume(video, choice.level);
  // eslint-disable-next-line no-param-reassign -- muting the film is setting its video's muted
  video.muted = choice.isMuted;
}
