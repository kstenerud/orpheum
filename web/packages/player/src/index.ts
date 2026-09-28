// SPDX-License-Identifier: MPL-2.0

/**
 * The public entry point of the Orpheum player: the only module other packages and apps import
 * from this package (`exports` in package.json forbids deeper paths).
 *
 * The player wraps hls.js and JASSUB, the subtitle layer (plans/cut-a.md §3).
 */

export { canSetVolume, gainAfterVolume } from "./gain.ts";
export type { Gain } from "./gain.ts";
export { hlsSupport, hlsWay, playHls, playNative } from "./hls.ts";
export type { HlsSupport, HlsWay, NativeTarget, Playback } from "./hls.ts";
export { PlayReporter, reportEveryMs, seekTo } from "./report.ts";
export type { PlayEvent, PlayEventKind, PlayReport, PlaySpan } from "./report.ts";
export { drawSubtitles, jassubTimeOffset } from "./subtitles.ts";
export type { SubtitleLayer } from "./subtitles.ts";

/** The npm name of this package, exported so the workspace wiring has something to resolve. */
export const playerPackageName = "@orpheum/player";
