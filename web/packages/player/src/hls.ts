// SPDX-License-Identifier: MPL-2.0

import {
  catchUpSeekEvent,
  holdLoadingEvent,
  loadingRunwayEvent,
  loadingRunwayOf,
  loadingHoldOf,
  playerLoadedChunkEvent,
  playerMovedItselfEvent,
  playerSwitchedQualityEvent,
  type ChunkLoaded,
  type QualitySwitched,
} from "@orpheum/sdk";
import Hls, { PlaylistLevelType, type AudioTrackSwitchingData, type ErrorData } from "hls.js";

import { isRecovered } from "./abr-controller.ts";
import type { ChoiceWhy } from "./abr-options.ts";
import { recoverFromDecodingErrors } from "./decode-recovery.ts";
import { failureText } from "./diagnosis.ts";
import { hlsErrorFailure, unreachableStatus, type PlaybackFailed } from "./failure.ts";
import type { ForwardBuffer } from "./forward-buffer.ts";
import { linkNoter } from "./link-estimate.ts";

/** Why the person's sound changes as it plays: the dialogue boost turned on or off (D217 §3). */
export type SoundChoiceCause = "boostOn" | "boostOff";

/** A prepared rendition playing in a video element. */
export interface HlsPlayback {
  /**
   * Plays the audio rendition at index, in the master playlist's order, for cause: the sound loaded
   * plays on until the new one's chunk at the playhead has loaded (keepAudioRendition).
   */
  readonly setAudioRendition: (index: number, cause: SoundChoiceCause) => void;
  /** Stops playing and lets go of the element. */
  readonly destroy: () => void;
  /** The link's speed in kb/s as hls.js measures its segments, or undefined where it cannot say. */
  readonly estimateKbps: () => number | undefined;
}

/** What the page offers for HLS: hls.js over MSE, or the element's own HLS, as Safari's. */
export interface HlsSupport {
  readonly hasMse: boolean;
  readonly canPlayNatively: boolean;
}

/** Whether this page can play HLS, and how. */
export function hlsSupport(video: HTMLVideoElement): HlsSupport {
  return {
    hasMse: Hls.isSupported(),
    canPlayNatively: video.canPlayType("application/vnd.apple.mpegurl") !== "",
  };
}

/** How a page plays HLS: hls.js, the element's own HLS, or not at all. */
export type HlsMethod = "hls.js" | "native" | "none";

/** The method to play HLS by: hls.js wherever MSE is, since it switches audio renditions itself. */
export function hlsMethod(support: HlsSupport): HlsMethod {
  if (support.hasMse) {
    return "hls.js";
  }
  return support.canPlayNatively ? "native" : "none";
}

/** The part of a media element the element's own HLS needs. */
export interface NativeTarget {
  readonly getAttribute: (name: string) => string | null;
  readonly setAttribute: (name: string, value: string) => void;
  readonly removeAttribute: (name: string) => void;
  readonly load: () => void;
}

/** Plays a playlist with the element's own HLS, as Safari's. */
export function playNative(target: NativeTarget, url: string): HlsPlayback {
  target.setAttribute("src", url);
  return {
    // Safari switches its own tracks from its controls; choosing one is left to them.
    setAudioRendition: () => undefined,
    // Safari measures its link for itself, and says nothing of it.
    estimateKbps: () => undefined,
    // A page's video kept for its next film may hold that film's source already: it is left be.
    destroy: () => {
      if (target.getAttribute("src") === url) {
        target.removeAttribute("src");
        target.load();
      }
    },
  };
}

/** What choosing the starting audio rendition needs of hls.js. */
export interface AudioStart {
  on: (event: typeof Hls.Events.MANIFEST_PARSED, listener: () => void) => void;
  audioTrack: number;
}

/**
 * Starts with the audio rendition at audioIndex once the master playlist is read: hls.js picks the
 * playlist's default until then, and a choice made earlier is not kept.
 */
export function startWithAudio(hls: AudioStart, audioIndex: number): void {
  if (audioIndex === 0) {
    return;
  }
  hls.on(Hls.Events.MANIFEST_PARSED, () => {
    // eslint-disable-next-line no-param-reassign -- hls.js switches the mix when audioTrack is set
    hls.audioTrack = audioIndex;
  });
}

/**
 * What hls.js compares of the sound it has loaded and the one it switches to, beside the name and
 * language every group shares at a place (D368 §3): where either differs, it throws the loaded
 * sound away.
 */
export interface SoundForm {
  channels?: string | undefined;
  audioCodec?: string | undefined;
}

/** What keeping the audio rendition's place across audio groups needs of hls.js. */
export interface AudioKeeping {
  on: ((
    event: typeof Hls.Events.AUDIO_TRACKS_UPDATED,
    listener: (event: unknown, data: { readonly audioTracks: readonly SoundForm[] }) => void,
  ) => void) &
    ((
      event: typeof Hls.Events.AUDIO_TRACK_SWITCHING,
      listener: (
        event: unknown,
        data: SoundForm &
          Readonly<Pick<AudioTrackSwitchingData, "flushImmediate">> & { readonly id: number },
      ) => void,
    ) => void) &
    ((
      event: typeof Hls.Events.AUDIO_TRACK_SWITCHED,
      listener: (event: unknown, data: { readonly id: number }) => void,
    ) => void);
  audioTrack: number;
  /** hls.js's switch that loads the new sound at the playhead while the sound loaded plays on. */
  nextAudioTrack: number;
}

/**
 * Keeps the audio rendition at its place, startIndex until another plays, as the quality loading
 * moves between audio groups: a film's top quality plays its own sound in a group of its own, every
 * group listing its sounds in one order (D368 §3), and hls.js, matching a rendition across groups by
 * its name and channels, would take the first of its language for a copied 5.1 one, the standard
 * mix for the boosted. Before its first plays, the place is the one hls.js switched to.
 *
 * The move keeps the sound loaded: hls.js throws all of it away when the sound it switches to
 * differs in channels or codec from the one loaded (`flushAudioIfNeeded`), and moves the sound with
 * the picture's next chunk, which is loaded far ahead of where it plays, so a Fire TV whose quality
 * stepped down from the film's own 5.1 sound kept 2.5 s of sound against 38.6 s of picture, asked
 * for the light track's chunks from there, found them being made, and stalled (staging, Nosferatu,
 * 2026-10-28). The rendition at the place is switched to in the loaded one's form, its own given
 * back at once, so hls.js loads it on from the end of the sound loaded, as it does for two groups'
 * stereo tracks, and the new sound is heard where the picture of its quality begins.
 *
 * Returns the choosing of another rendition, the person's (the dialogue boost turned on or off), which
 * it keeps from then on: one way for every change of sound as the film plays. The sound loaded plays
 * on while hls.js asks for the new one's chunk at the playhead, which starts the server making it,
 * and the new one is heard from that chunk once it has loaded, the sound loaded after it thrown away
 * (`nextAudioTrack`): switched with `audioTrack`, hls.js threw all of it away at once and the video
 * waited for the new one's chunk, a boost turned on at the top of Thimbleford's ladder froze it 2.2 s
 * (staging, 2026-10-29). A switch to the place while one chosen is on its way, as the quality moves
 * to another group, is made the same way. Each choice is noted, with its cause, and again as it is
 * heard (idiosyncrasies.md).
 */
export function keepAudioRendition(
  hls: AudioKeeping,
  startIndex: number | undefined,
  note: (text: string) => void = () => undefined,
): (index: number, cause: SoundChoiceCause) => void {
  let kept = startIndex;
  let switchedTo: { readonly id: number; readonly form: SoundForm } | undefined;
  let chosen: { readonly id: number; readonly cause: SoundChoiceCause } | undefined;
  hls.on(Hls.Events.AUDIO_TRACK_SWITCHING, (_event, data) => {
    // A choice on its way leaves the sound loaded as it was until its chunk is heard.
    if (data.flushImmediate !== false) {
      switchedTo = { id: data.id, form: { channels: data.channels, audioCodec: data.audioCodec } };
    }
  });
  hls.on(Hls.Events.AUDIO_TRACK_SWITCHED, (_event, data) => {
    kept = data.id;
    if (chosen?.id === data.id) {
      note(`sound ${String(data.id)} heard (${chosen.cause})`);
      chosen = undefined;
    }
  });
  // A group's tracks are listed before hls.js chooses one of them: a rendition chosen then is the
  // one it plays.
  hls.on(Hls.Events.AUDIO_TRACKS_UPDATED, (_event, data) => {
    const place = kept ?? switchedTo?.id;
    if (place === undefined || place >= data.audioTracks.length || hls.audioTrack === place) {
      return;
    }
    if (chosen?.id === place) {
      // eslint-disable-next-line no-param-reassign -- hls.js switches the mix when nextAudioTrack is set
      hls.nextAudioTrack = place;
      return;
    }
    switchInForm(hls, { place, sound: data.audioTracks[place] }, switchedTo?.form);
  });
  return (index, cause) => {
    if (index === kept && chosen === undefined) {
      return;
    }
    note(
      `sound ${String(index)} chosen (${cause}), loading at the playhead as sound ${String(kept ?? hls.audioTrack)} plays`,
    );
    kept = index;
    chosen = { id: index, cause };
    // eslint-disable-next-line no-param-reassign -- hls.js switches the mix when nextAudioTrack is set
    hls.nextAudioTrack = index;
  };
}

/**
 * Starts with the audio rendition at startIndex, the playlist's default when omitted, and keeps it as
 * the quality moves (keepAudioRendition); returns the one way the person's sound changes as the film
 * plays, on a ladder and a prepared rendition alike, noting each change by note.
 */
export function startAndKeepAudio(
  hls: AudioStart & AudioKeeping,
  startIndex: number | undefined,
  note?: (text: string) => void,
): (index: number, cause: SoundChoiceCause) => void {
  startWithAudio(hls, startIndex ?? 0);
  // 0 is the playlist's default, which hls.js starts with: kept once it plays.
  return keepAudioRendition(hls, startIndex === 0 ? undefined : startIndex, note);
}

/**
 * Switches hls.js to the sound at a place in form, the form of the sound loaded, and gives the
 * sound its own form back once hls.js has read it; in its own where either is unknown.
 */
function switchInForm(
  hls: Pick<AudioKeeping, "audioTrack">,
  to: { readonly place: number; readonly sound: SoundForm | undefined },
  form: SoundForm | undefined,
): void {
  const { place, sound } = to;
  if (sound === undefined || form === undefined) {
    // eslint-disable-next-line no-param-reassign -- hls.js switches the mix when audioTrack is set
    hls.audioTrack = place;
    return;
  }
  const own: SoundForm = { channels: sound.channels, audioCodec: sound.audioCodec };
  Object.assign(sound, form);
  try {
    // eslint-disable-next-line no-param-reassign -- as above
    hls.audioTrack = place;
  } finally {
    Object.assign(sound, own);
  }
}

/** What telling the player's own moves needs of hls.js. */
export interface OwnMoves {
  on: (
    event: typeof Hls.Events.ERROR,
    listener: (event: unknown, data: Pick<ErrorData, "details">) => void,
  ) => void;
}

/**
 * Loads no more than the runway the follower of its group asks for, while one in the group is still
 * loading, and as far as its target in force otherwise (loadingRunwayEvent, D285 §2): ahead keeps
 * both, so lifting a runway goes back to the target as it is then, hls.js's own cut after a buffer
 * run full kept, never to the values hls.js was set to at the start, which undid that cut
 * (forward-buffer-2026-10-29.md §1.3). Returns the stopping.
 */
export function keepRunwayWhenTold(
  video: Pick<EventTarget, "addEventListener" | "removeEventListener">,
  ahead: Pick<ForwardBuffer, "keepRunway">,
): () => void {
  const listener = (event: Event): void => {
    const runway = loadingRunwayOf(event);
    if (runway !== undefined) {
      ahead.keepRunway(runway.aheadS);
    }
  };
  video.addEventListener(loadingRunwayEvent, listener);
  return () => {
    video.removeEventListener(loadingRunwayEvent, listener);
  };
}

/**
 * Loads as the follower of its group says: held while its group waits (holdLoadingWhenTold), and no
 * more than a runway while one in the group is behind (keepRunwayWhenTold), as far as its target in
 * force otherwise. Returns the stopping.
 */
export function obeyGroupLoading(
  hls: Pick<Hls, "stopLoad" | "startLoad">,
  video: Pick<EventTarget, "addEventListener" | "removeEventListener">,
  ahead: Pick<ForwardBuffer, "keepRunway">,
): () => void {
  const stops = [holdLoadingWhenTold(hls, video), keepRunwayWhenTold(video, ahead)];
  return () => {
    for (const stop of stops) {
      stop();
    }
  };
}

/** The header in which a player says it loads where a seek took it (D284 §2). */
export const soughtHeader = "Orpheum-Sought";

/**
 * Whether the video loads where a seek took it, its person's or its group's, from its seeking until
 * it can play there: a request then says so (soughtHeader), and the server starts a run there sooner
 * than it does for another (D284 §2). The seek of a follower catching up (catchUpSeekEvent), and
 * hls.js's own steps over a gap (playerMovedItselfEvent), are no jumps. The owner: "We should be able
 * to tell from the controlling client whether it's a jump or not" (2026-10-12).
 */
export function markSeeks(video: Pick<EventTarget, "addEventListener" | "removeEventListener">): {
  readonly isSought: () => boolean;
  readonly stop: () => void;
} {
  let isSought = false;
  let isNextSeekNoJump = false;
  const listeners: Record<string, () => void> = {
    [catchUpSeekEvent]: () => {
      isNextSeekNoJump = true;
    },
    [playerMovedItselfEvent]: () => {
      isNextSeekNoJump = true;
    },
    seeking: () => {
      isSought = !isNextSeekNoJump;
      isNextSeekNoJump = false;
    },
    canplay: () => {
      isSought = false;
    },
    playing: () => {
      isSought = false;
    },
  };
  for (const [event, listener] of Object.entries(listeners)) {
    video.addEventListener(event, listener);
  }
  return {
    isSought: () => isSought,
    stop: () => {
      for (const [event, listener] of Object.entries(listeners)) {
        video.removeEventListener(event, listener);
      }
    },
  };
}

/**
 * Holds hls.js's loading while the follower of its group says to, and lets it go on from where the
 * video is when told (holdLoadingEvent): a ready player then leaves the server's upload to those its
 * group waits for. Returns the stopping.
 */
export function holdLoadingWhenTold(
  hls: Pick<Hls, "stopLoad" | "startLoad">,
  video: Pick<EventTarget, "addEventListener" | "removeEventListener">,
): () => void {
  const listener = (event: Event): void => {
    const hold = loadingHoldOf(event);
    if (hold?.isHeld === true) {
      hls.stopLoad();
    } else if (hold?.isHeld === false) {
      hls.startLoad(-1); // from where the video is
    }
  };
  video.addEventListener(holdLoadingEvent, listener);
  return () => {
    video.removeEventListener(holdLoadingEvent, listener);
  };
}

/**
 * Tells video, by playerMovedItselfEvent, each time hls.js moves it itself: stepping over a gap in
 * what it loaded, or nudging a stalled video on. hls.js sets the position and then says so, before
 * the browser fires seeking, so a Watch Together host's page knows that seek for the player's own.
 */
export function tellOwnMoves(hls: OwnMoves, video: Pick<EventTarget, "dispatchEvent">): void {
  hls.on(Hls.Events.ERROR, (_event, data) => {
    // What it moved over goes with the event, for the pause's or the seek's context (event-context.ts).
    if (data.details === Hls.ErrorDetails.BUFFER_SEEK_OVER_HOLE) {
      video.dispatchEvent(new CustomEvent(playerMovedItselfEvent, { detail: "hole" }));
    }
    if (data.details === Hls.ErrorDetails.BUFFER_NUDGE_ON_STALL) {
      video.dispatchEvent(new CustomEvent(playerMovedItselfEvent, { detail: "nudge" }));
    }
  });
}

/** What telling what hls.js loads needs of it: its qualities, its link estimate, its events. */
export interface LoadingHls {
  readonly levels: readonly { readonly bitrate: number }[];
  readonly bandwidthEstimate: number;
  on: Hls["on"];
}

/**
 * Tells onFailed why, when hls.js says the playback cannot go on, unless the player recovers from
 * it by itself (isRecovered: a hole given up, or loading asked again after a load that failed), or
 * when the core says its film cannot be read now (a 503), which hls.js holds no failure: a sound
 * chunk's 503 it only loaded again every two seconds, the player spinning and saying nothing (the
 * owner's test, 2026-10-21). With the code goes what failed, for the core's log alone (failureText,
 * what carried it read by carrierOf where given).
 */
export function failOnFatal(
  hls: Pick<Hls, "on" | "stopLoad">,
  onFailed: PlaybackFailed,
  carrierOf?: (url: string) => string,
): void {
  hls.on(Hls.Events.ERROR, (_event, data) => {
    if (data.response?.code === unreachableStatus) {
      // Loading again finds it no more readable: the page plays it anew once it can be read.
      hls.stopLoad();
      onFailed(hlsErrorFailure(data), failureText(data, carrierOf));
    } else if (data.fatal && !isRecovered(data)) {
      onFailed(hlsErrorFailure(data), failureText(data, carrierOf));
    }
  });
}

/**
 * The link's speed hls.js measured, in kb/s, once it has loaded a chunk; undefined before. Until
 * then its estimate is its own starting value, which a phone's player reported as its link, 500
 * then 5000 kb/s, and the core remembered for its next playback (staging, 2026-10-07).
 */
export function measuredKbps(
  hls: Pick<LoadingHls, "bandwidthEstimate" | "on">,
): () => number | undefined {
  let hasMeasured = false;
  hls.on(Hls.Events.FRAG_LOADED, () => {
    hasMeasured = true;
  });
  return () =>
    hasMeasured && hls.bandwidthEstimate > 0 ? hls.bandwidthEstimate / 1000 : undefined;
}

/**
 * Tells video what hls.js loads, for a Watch Together player's log: each chunk, by
 * playerLoadedChunkEvent, with how long it took and how fast, and each quality it begins playing,
 * by playerSwitchedQualityEvent, with the quality it played before, none for its first, and its
 * link's estimate then, by its controller's linkKbps, hls.js's once a chunk loaded where it has
 * none, and why its controller chose it, with the link's samples then, where its whyChose says
 * (ChoiceWhy). A viewer's iPhone froze every few seconds and nothing said what it loaded, or how fast
 * (staging, 2026-10-06). A Fire TV's first quality read as a switch to the top on a link of 5000
 * kb/s, hls.js's starting value, its own controller having measured nothing (the owner's cast,
 * staging, 2026-10-28).
 */
export function tellLoading(
  hls: LoadingHls,
  video: Pick<EventTarget, "dispatchEvent">,
  controller: {
    readonly linkKbps: () => number | undefined;
    readonly whyChose?: (level: number) => ChoiceWhy | undefined;
  } = { linkKbps: measuredKbps(hls) },
): void {
  const { linkKbps: linkEstimateKbps, whyChose = () => undefined } = controller;
  hls.on(Hls.Events.FRAG_LOADED, (_event, data) => {
    const { frag } = data;
    const isVideo = frag.type === PlaylistLevelType.MAIN;
    if (!isVideo && frag.type !== PlaylistLevelType.AUDIO) {
      return;
    }
    const { loading, total } = frag.stats;
    const loadMs = Math.max(0, loading.end - loading.start);
    const loaded: ChunkLoaded = {
      track: isVideo ? "video" : "audio",
      loadMs,
      loadedKbps: loadMs > 0 ? (total * 8) / loadMs : 0,
      qualityKbps: isVideo ? (hls.levels[frag.level]?.bitrate ?? 0) / 1000 : 0,
      filmMs: frag.duration * 1000,
    };
    video.dispatchEvent(new CustomEvent(playerLoadedChunkEvent, { detail: loaded }));
  });
  let playedKbps: number | undefined;
  hls.on(Hls.Events.LEVEL_SWITCHED, (_event, data) => {
    const qualityKbps = (hls.levels[data.level]?.bitrate ?? 0) / 1000;
    const chose = whyChose(data.level);
    const switched: QualitySwitched = {
      qualityKbps,
      linkEstimateKbps: linkEstimateKbps(),
      ...(playedKbps === undefined ? {} : { fromQualityKbps: playedKbps }),
      ...(chose === undefined ? {} : { why: chose.why }),
      ...(chose === undefined || chose.linkSamples === ""
        ? {}
        : { linkSamples: chose.linkSamples }),
    };
    playedKbps = qualityKbps;
    video.dispatchEvent(new CustomEvent(playerSwitchedQualityEvent, { detail: switched }));
  });
}

/**
 * How hls.js plays a prepared rendition: from its start. A rendition still being prepared has an
 * open playlist (EVENT, no ENDLIST yet), which hls.js reads as live and, left to itself, starts
 * near its newest segment: a film whose prepare had made 46 s began at 40 s (staging,
 * 2026-10-06). A person's place to resume is a seek after it loads, as for any rendition.
 */
export const preparedHlsConfig = { startPosition: 0 } as const;

/**
 * Plays an HLS master playlist in video: with hls.js where the page has MSE, else with the
 * element's own HLS, as on an iPhone. The URL was produced by the SDK; the requests carry the
 * media cookie, never a credential in the address (D154).
 */
export function playHls(
  video: HTMLVideoElement,
  url: string,
  options: {
    /** Called with why, as a code, when the playback cannot go on. */
    readonly onFailed: PlaybackFailed;
    /** The audio rendition to start with; the playlist's default when omitted. */
    readonly startAudioRenditionIndex?: number;
  },
): HlsPlayback {
  const { onFailed } = options;
  const method = hlsMethod(hlsSupport(video));
  if (method === "native") {
    return playNative(video, url);
  }
  if (method === "none") {
    onFailed("hls_unsupported");
    return {
      setAudioRendition: () => undefined,
      destroy: () => undefined,
      estimateKbps: () => undefined,
    };
  }
  const hls = new Hls(preparedHlsConfig);
  const stopRecoveringDecoding = recoverFromDecodingErrors(hls, { note: linkNoter(video) });
  failOnFatal(hls, onFailed);
  const chooseAudio = startAndKeepAudio(hls, options.startAudioRenditionIndex);
  tellOwnMoves(hls, video);
  tellLoading(hls, video);
  const estimateKbps = measuredKbps(hls);
  hls.loadSource(url);
  hls.attachMedia(video);
  return {
    setAudioRendition: chooseAudio,
    destroy: () => {
      stopRecoveringDecoding();
      hls.destroy();
    },
    estimateKbps,
  };
}
