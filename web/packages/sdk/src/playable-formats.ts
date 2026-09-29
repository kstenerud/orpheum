// SPDX-License-Identifier: MPL-2.0

import type { components } from "./gen/api.ts";

/** What a client can play, one list per axis of the menu, most preferred first (D21, D217 §1). */
export type PlayableFormats = components["schemas"]["PlayableFormatsBody"];

type VideoCodecName = NonNullable<PlayableFormats["videoCodecs"]>[number];
type AudioCodecName = NonNullable<PlayableFormats["audioCodecs"]>[number];
type ContainerName = NonNullable<PlayableFormats["containers"]>[number];

/** How a page asks what it can decode: a media element's canPlayType, and MSE's isTypeSupported. */
export interface MediaProbe {
  readonly canPlayType: (type: string) => string;
  readonly isTypeSupported: (type: string) => boolean;
}

// One representative codec string per menu entry: the entry is a named bundle (D21), so asking
// about its most common member is asking about the entry.
const videoCodecProbes: readonly (readonly [VideoCodecName, string])[] = [
  ["hevc-hdr10", 'video/mp4; codecs="hvc1.2.4.L153.B0"'],
  ["av1-hdr10", 'video/mp4; codecs="av01.0.08M.10"'],
  ["hevc-sdr", 'video/mp4; codecs="hvc1.1.6.L120.90"'],
  ["av1-sdr", 'video/mp4; codecs="av01.0.08M.08"'],
];
const audioCodecProbes: readonly (readonly [AudioCodecName, string])[] = [
  ["eac3", 'audio/mp4; codecs="ec-3"'],
  ["ac3", 'audio/mp4; codecs="ac-3"'],
  ["opus", 'audio/mp4; codecs="opus"'],
  ["flac", 'audio/mp4; codecs="flac"'],
];
const containerProbes: readonly (readonly [ContainerName, readonly string[]])[] = [
  ["mp4", ["video/mp4"]],
  ["mkv", ["video/x-matroska", "video/matroska"]],
  ["mpegts", ["video/mp2t"]],
];

/**
 * The formats a page can play, from what its media element and MSE say. The baselines, h264-sdr
 * and aac-stereo, are always listed: every client plays them (D21), and they come last among
 * the video entries, after what is more efficient. AAC with more than two channels is decoded
 * and downmixed by every browser, so aac-multi follows aac-stereo.
 */
export function playableFormatsFrom(probe: MediaProbe): PlayableFormats {
  const isPlayable = (type: string): boolean =>
    probe.isTypeSupported(type) || probe.canPlayType(type) !== "";
  return {
    videoCodecs: [
      ...videoCodecProbes.filter(([, type]) => isPlayable(type)).map(([name]) => name),
      "h264-sdr",
    ],
    audioCodecs: [
      "aac-stereo",
      "aac-multi",
      ...audioCodecProbes.filter(([, type]) => isPlayable(type)).map(([name]) => name),
    ],
    subtitleFormats: ["ass", "webvtt"],
    containers: containerProbes
      .filter(([, types]) => types.some((type) => probe.canPlayType(type) !== ""))
      .map(([name]) => name),
  };
}

/** The formats the browser this runs in can play. */
export function browserPlayableFormats(): PlayableFormats {
  const element = document.createElement("video");
  // Safari on iOS has ManagedMediaSource instead; canPlayType answers for it there.
  const hasMediaSource = "MediaSource" in globalThis;
  return playableFormatsFrom({
    canPlayType: (type) => element.canPlayType(type),
    isTypeSupported: (type) => hasMediaSource && MediaSource.isTypeSupported(type),
  });
}
