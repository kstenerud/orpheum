// SPDX-License-Identifier: MPL-2.0

/**
 * The public entry point of the Orpheum player: the only module other packages and apps import
 * from this package (`exports` in package.json forbids deeper paths).
 *
 * The player wraps hls.js and JASSUB, the subtitle layer (plans/cut-a.md §3); it holds the gain
 * after the volume that levels a film's loudness (D226 §4), and the reporting of play state
 * (D217 §7).
 */

export { hlsErrorFailure, mediaErrorFailure, playbackFailures } from "./failure.ts";
export type { PlaybackFailure } from "./failure.ts";
export { diagnosisOf } from "./diagnosis.ts";
export type { PlaybackDiagnosis } from "./diagnosis.ts";
export { canSetVolume, gainAfterVolume } from "./gain.ts";
export type { Gain } from "./gain.ts";
export { canFollowAGroup, hlsSupport, hlsMethod, playHls, playNative } from "./hls.ts";
export type { HlsSupport, HlsMethod, HlsPlayback, NativeTarget } from "./hls.ts";
export { playChunked, playerFoundUnplayableEvent, unplayableOf } from "./chunked.ts";
export type { ChunkedPlayback, PlayedLevel, UnplayableQuality } from "./chunked.ts";
export { meterBufferGrowth } from "./link.ts";
export type { LinkMeter, MeteredMedia } from "./link.ts";
export { PlayReporter, ReportRefused, reportEveryMs, seekTo } from "./report.ts";
export {
  eventContextOf,
  mediaSessionActionLastsMs,
  skippedByOf,
  watchMediaSession,
} from "./event-context.ts";
export type { ActionSession, ContextPage, MediaSessionAction, SkippedBy } from "./event-context.ts";
export type {
  PlayEvent,
  PlayEventContext,
  PlayEventKind,
  PlayReport,
  PlaySpan,
  PlayStandstill,
  PlayUnplayable,
} from "./report.ts";
export { stallAfterMs, watchStalls } from "./stalls.ts";
export { standsStillReportAfterMs, watchStandingStill } from "./standstill.ts";
export type { Standstill, StandingMedia } from "./standstill.ts";
export type { Stall, WatchedMedia } from "./stalls.ts";
export { holdBeforeStepUpMs, nextCap, stepUpBufferMs, stepUpHeadroom } from "./step-up.ts";
export type { StepUpState } from "./step-up.ts";
export { drawSubtitles, jassubTimeOffset } from "./subtitles.ts";
export type { SubtitleLayer } from "./subtitles.ts";
export type { CaptionSource } from "./fullscreen-captions.ts";
export { keepScreenAwake } from "./wake.ts";
export { pauseWhenHidden } from "./hidden.ts";
export type { HiddenPage, PausedFilm } from "./hidden.ts";
export type { AwakeMedia, AwakePage, ScreenLock, ScreenLocks } from "./wake.ts";

/** The npm name of this package, exported so the workspace wiring has something to resolve. */
export const playerPackageName = "@orpheum/player";
