// SPDX-License-Identifier: MPL-2.0

import { playerBufferedFirstEvent } from "@orpheum/sdk";
import Hls, { type BufferCodecsData, type ErrorData } from "hls.js";

/**
 * What a chunked playback's video holds and what went wrong last, for a stall's report (D266):
 * a video that sat paused at readyState 1 on one device while the same chunks played on others
 * said nothing of whether its sound or its picture was missing where it stood, or whether an
 * append failed, and a guess took its place (the owner's test, 2026-10-08).
 */
export interface PlaybackDiagnosis {
  /** What the picture's and the sound's buffers hold, as "from-to" ranges in seconds. */
  readonly videoBuffered: string;
  readonly audioBuffered: string;
  /** The last error hls.js reported, recovered or not, and the element's own; "" for none. */
  readonly lastError: string;
  /**
   * How hls.js placed what it loaded first: each playlist's first chunk buffered, by its number,
   * its start in the playlist and its media time, and each track's initPTS; "" before any.
   */
  readonly mapping: string;
  /**
   * Whether an iPhone's ManagedMediaSource lets the player load now, "streaming" or "stopped", as
   * its last startstreaming or endstreaming said; "" where the source says neither.
   */
  readonly mediaSourceState: string;
  /**
   * The player's last steps, oldest first: each quality it chose, each playlist and chunk it loaded,
   * aborted or failed, and its video detached and attached; "" before any. A Mac's Firefox, its
   * film's own picture given up at 0:00, stood with nothing loaded and asked for nothing more, and no
   * report said where its player had stopped (the owner's test, 2026-10-18). Its own choices come
   * last.
   */
  readonly steps?: string;
  /**
   * The link's last samples in a line (samplesText), for a stall's and a seek's report beside its
   * steps, never among them, so they share none of the steps' length (D352 §5); absent before any.
   */
  readonly linkSamples?: string;
}

/** What a diagnosis keeps as hls.js tells it. */
interface Kept {
  video: string;
  audio: string;
  lastError: string;
  mapping: string[];
  mediaSourceState: string;
  steps: string[];
  /**
   * The player's own last choices of quality, with why, kept apart so the steps never push them out,
   * each with when it was made by choicesClock, where its noter was given one.
   */
  choices: { readonly text: string; readonly atMs: number | undefined }[];
  /** The clock the choices were timed by, their noter's (stepNoter); undefined for none. */
  choicesClock: (() => number) | undefined;
  /** The link's last samples in a line (samplesText), kept apart from the steps (D352 §5); "" before any. */
  samples: string;
  /** The codecs each buffer took, by its track ("video avc1.64001f"), the latest for each. */
  codecs: Map<string, string>;
}

/** The longest a diagnosis's mapping may be, as the server takes it. */
const mappingTextMost = 300;

/** How many of the player's last steps a diagnosis keeps, and the longest they may be, as the server takes them. */
const stepsKept = 12;
/** How many of the player's own last choices of quality a diagnosis keeps, after its steps. */
const choicesKept = 3;
const stepsTextMost = 400;

/**
 * The hls.js events a diagnosis keeps as steps, beside its errors and its video's attaching. A
 * buffer thrown away is one: a Fire TV's sound, moved earlier by Web Audio's latency as it began to
 * play, was thrown away and loaded again, and the video, its picture loaded nine seconds ahead,
 * waited 2.5 s for it with nothing to say why (the owner's cast, staging, 2026-10-28).
 */
const stepEvents = [
  Hls.Events.BUFFER_FLUSHING,
  Hls.Events.LEVEL_SWITCHING,
  Hls.Events.LEVEL_LOADING,
  Hls.Events.LEVEL_LOADED,
  Hls.Events.FRAG_LOADING,
  Hls.Events.FRAG_LOADED,
  Hls.Events.FRAG_LOAD_EMERGENCY_ABORTED,
  Hls.Events.MEDIA_DETACHED,
] as const;

/** What a step's event says, of what hls.js tells with it. */
interface StepData {
  readonly level?: number;
  /** A flush's buffer ("audio", "video", or none for every one) and where it starts and ends, in seconds. */
  readonly type?: string | null;
  readonly startOffset?: number;
  readonly endOffset?: number;
  readonly frag?: {
    readonly level: number;
    readonly sn: number | "initSegment";
    readonly type: string;
  };
}

/**
 * A step in a line: "level 2", "playlist 2 loaded", "frag 2/31 aborted", "sound 0/31 loading",
 * "audio flushed 0.00-957.29", "all flushed 12.50-end". A flush says its end: hls.js's trim of what
 * it keeps behind the playhead and its throwing away of everything both start at 0, and a seek's
 * report saying "video flushed from 0.00; audio flushed from 0.00" was read as a quality switch
 * that threw away what played, when it was the trim of film half an hour behind (the owner's
 * Chrome, staging, 2026-11-01).
 */
export function stepText(event: string, data: StepData): string {
  const { frag } = data;
  const flushing: string = Hls.Events.BUFFER_FLUSHING;
  if (event === flushing) {
    return flushText(data);
  }
  if (frag !== undefined) {
    const what = `${frag.type === "main" ? "frag" : "sound"} ${String(frag.level)}/${String(frag.sn)}`;
    const outcome: Partial<Record<string, string>> = {
      [Hls.Events.FRAG_LOADING]: "loading",
      [Hls.Events.FRAG_LOADED]: "loaded",
      [Hls.Events.FRAG_LOAD_EMERGENCY_ABORTED]: "aborted",
    };
    return `${what} ${outcome[event] ?? event}`;
  }
  const level = String(data.level ?? "?");
  const texts: Partial<Record<string, string>> = {
    [Hls.Events.LEVEL_SWITCHING]: `level ${level}`,
    [Hls.Events.LEVEL_LOADING]: `playlist ${level} loading`,
    [Hls.Events.LEVEL_LOADED]: `playlist ${level} loaded`,
    [Hls.Events.MEDIA_DETACHED]: "detached",
  };
  return texts[event] ?? event;
}

/** A flush as a step: its buffer, and where it starts and ends ("video flushed 0.00-957.29"). */
function flushText(data: StepData): string {
  const end = data.endOffset;
  const endText = end === undefined || !Number.isFinite(end) ? "end" : end.toFixed(2);
  return `${data.type ?? "all"} flushed ${(data.startOffset ?? 0).toFixed(2)}-${endText}`;
}

/** Keeps each of stepEvents hls.js tells as the diagnosis's latest step. */
function keepSteps(hls: DiagnosedHls, diagnosis: Kept): void {
  for (const event of stepEvents) {
    hls.on(event, (_event, data) => {
      keepStep(diagnosis, stepText(event, data));
    });
  }
}

/** Keeps step as the diagnosis's latest, the stepsKept latest at most. */
function keepStep(diagnosis: Kept, step: string): void {
  diagnosis.steps.push(step);
  if (diagnosis.steps.length > stepsKept) {
    diagnosis.steps.shift();
  }
}

const diagnoses = new WeakMap<object, Kept>();

/** Ranges as "from-to from-to", in seconds to two places. */
export function rangesText(ranges: Pick<TimeRanges, "length" | "start" | "end">): string {
  const parts: string[] = [];
  for (let index = 0; index < ranges.length; index++) {
    parts.push(`${ranges.start(index).toFixed(2)}-${ranges.end(index).toFixed(2)}`);
  }
  return parts.join(" ");
}

/** What hls.js's error says went wrong, as a listener of its errors reads it. */
type ErrorSaid = Pick<ErrorData, "details" | "fatal"> & {
  /**
   * Its error, which hls.js types as always there but a caller may leave out: read in hls.js's own
   * handling of an error, a throw for its absence would be hls.js's next fatal error.
   */
  readonly error?: Pick<Error, "message"> | undefined;
};

/** What hls.js said went wrong, in a line. */
function errorText(data: ErrorSaid): string {
  const message = data.error?.message ?? "no message";
  return `${data.details}${data.fatal ? " (fatal)" : ""}: ${message}`.slice(0, 200);
}

/**
 * What failed, for the core's log: hls.js's error, its type, details, whether fatal and its message
 * (errorText), the answer's status, none where no answer came, and what carried the load
 * (carrierOf, the SDK's mediaCarrierOf), "unknown" where nothing says. A load that failed while the
 * owner's Chrome was offline was reported only as "the browser gave no reason" (staging, 2026-10-29).
 */
export function failureText(
  data: ErrorSaid & Pick<ErrorData, "type" | "response" | "frag" | "url">,
  carrierOf: ((url: string) => string) | undefined,
): string {
  const url = data.frag?.url ?? data.response?.url ?? data.url;
  const carrier = url === undefined || carrierOf === undefined ? "unknown" : carrierOf(url);
  const code = data.response === undefined ? "none" : String(data.response.code);
  return `${data.type} ${errorText(data)}; status ${code}; carried ${carrier}`;
}

/** What diagnosing a playback needs of hls.js. */
export interface DiagnosedHls {
  on: ((
    event: typeof Hls.Events.FRAG_BUFFERED,
    listener: (
      event: string,
      data: {
        readonly id: string;
        readonly frag: {
          readonly sn: number | "initSegment";
          readonly start: number;
          readonly startPTS?: number;
        };
      },
    ) => void,
  ) => void) &
    ((
      event: typeof Hls.Events.INIT_PTS_FOUND,
      listener: (
        event: string,
        data: { readonly id: string; readonly initPTS: number; readonly timescale: number },
      ) => void,
    ) => void) &
    ((
      event: typeof Hls.Events.BUFFER_CODECS,
      listener: (
        event: string,
        data: Pick<BufferCodecsData, "video" | "audio" | "audiovideo">,
      ) => void,
    ) => void) &
    ((
      event: typeof Hls.Events.MEDIA_ATTACHED,
      listener: (event: string, data: { readonly mediaSource?: EventTarget }) => void,
    ) => void) &
    ((
      event: typeof Hls.Events.BUFFER_APPENDED,
      listener: (
        event: string,
        data: { readonly timeRanges: Partial<Record<string, TimeRanges>> },
      ) => void,
    ) => void) &
    ((event: typeof Hls.Events.ERROR, listener: (event: string, data: ErrorSaid) => void) => void) &
    ((
      event: (typeof stepEvents)[number],
      listener: (event: string, data: StepData) => void,
    ) => void);
}

/**
 * Keeps video's diagnosis as hls.js appends to its buffers and reports errors, and tells video by
 * playerBufferedFirstEvent as each playlist's first chunk is buffered.
 */
export function diagnose(hls: DiagnosedHls, video: EventTarget): void {
  const diagnosis = newKept();
  diagnoses.set(video, diagnosis);
  keepSteps(hls, diagnosis);
  hls.on(Hls.Events.BUFFER_APPENDED, (_event, data) => {
    const { video: picture, audio, audiovideo } = data.timeRanges;
    if (picture !== undefined) {
      diagnosis.video = rangesText(picture);
    }
    if (audio !== undefined) {
      diagnosis.audio = rangesText(audio);
    }
    if (audiovideo !== undefined) {
      diagnosis.video = rangesText(audiovideo);
      diagnosis.audio = diagnosis.video;
    }
  });
  hls.on(Hls.Events.ERROR, (_event, data) => {
    diagnosis.lastError = errorText(data);
    keepStep(diagnosis, `error ${data.details}${data.fatal ? " (fatal)" : ""}`);
  });
  // How hls.js placed its first chunks, and whether its media source lets it load.
  const buffered = new Set<string>();
  const initPtsFound = new Set<string>();
  hls.on(Hls.Events.INIT_PTS_FOUND, (_event, data) => {
    // The first of each track's, as its first chunk's: hls.js finds it again at a new quality's
    // init, and the mapping grew with every switch (the M4 closing audit, 2026-10-08).
    if (initPtsFound.has(data.id)) {
      return;
    }
    initPtsFound.add(data.id);
    diagnosis.mapping.push(`initPTS ${data.id} ${(data.initPTS / data.timescale).toFixed(3)}`);
  });
  hls.on(Hls.Events.FRAG_BUFFERED, (_event, data) => {
    if (buffered.has(data.id)) {
      return;
    }
    buffered.add(data.id);
    const { frag } = data;
    diagnosis.mapping.push(
      `${data.id} sn ${String(frag.sn)} at ${frag.start.toFixed(2)} pts ${frag.startPTS?.toFixed(2) ?? "?"}`,
    );
    video.dispatchEvent(new CustomEvent(playerBufferedFirstEvent, { detail: data.id }));
  });
  keepCodecs(hls, diagnosis);
  hls.on(Hls.Events.MEDIA_ATTACHED, (_event, data) => {
    keepStep(diagnosis, "attached");
    data.mediaSource?.addEventListener("startstreaming", () => {
      diagnosis.mediaSourceState = "streaming";
    });
    data.mediaSource?.addEventListener("endstreaming", () => {
      diagnosis.mediaSourceState = "stopped";
    });
  });
}

/** A diagnosis with nothing kept yet. */
function newKept(): Kept {
  return {
    video: "",
    audio: "",
    lastError: "",
    mapping: [],
    mediaSourceState: "",
    steps: [],
    choices: [],
    choicesClock: undefined,
    samples: "",
    codecs: new Map(),
  };
}

/** Keeps the codecs each of hls.js's buffers takes in diagnosis, the latest for each track. */
function keepCodecs(hls: DiagnosedHls, diagnosis: Kept): void {
  hls.on(Hls.Events.BUFFER_CODECS, (_event, data) => {
    for (const track of ["video", "audio", "audiovideo"] as const) {
      const taken = data[track];
      if (taken !== undefined) {
        diagnosis.codecs.set(track, `${taken.container} ${taken.codec ?? "?"}`);
      }
    }
  });
}

/**
 * What keeps one of the player's own choices of quality, with why, in video's diagnosis: its last
 * choicesKept, after its steps, which a dozen chunks' loads pushed out before a seek's report was sent
 * (the owner's Firefox on nas.example, 2026-10-20); nothing for a video not diagnosed. Timed by
 * monotonicMs where given, each choice says how long before the report it was made ("chose 502 s
 * ago ceiling 0 to 1: stepped up"): kept apart from the steps, a choice minutes old read as the
 * seek's own, a step up made 501 s before a seek taken for its cause (the owner's Chrome, staging,
 * 2026-11-01).
 */
export function stepNoter(video: object, monotonicMs?: () => number): (step: string) => void {
  return (step) => {
    const diagnosis = diagnoses.get(video);
    if (diagnosis !== undefined) {
      diagnosis.choicesClock = monotonicMs;
      diagnosis.choices.push({ text: step, atMs: monotonicMs?.() });
      diagnosis.choices.splice(0, Math.max(0, diagnosis.choices.length - choicesKept));
    }
  };
}

/** A kept choice as a step: "chose <text>", with how long ago where it was timed. */
function choiceText(
  choice: { readonly text: string; readonly atMs: number | undefined },
  nowMs: number | undefined,
): string {
  if (choice.atMs === undefined || nowMs === undefined) {
    return `chose ${choice.text}`;
  }
  const agoS = Math.max(0, Math.round((nowMs - choice.atMs) / 1000));
  return `chose ${String(agoS)} s ago ${choice.text}`;
}

/** The longest the link's samples may be, as the server takes them (StallBody, SeekBody). */
export const linkSamplesTextMost = 300;

/**
 * What keeps the link's last samples in video's diagnosis, the latest line replacing the one before,
 * for every stall's and seek's report beside its steps (D352 §5): the owner's report of a film held at
 * its lowest quality needed a day's reading of code to explain one sample (2026-10-26); nothing for
 * a video not diagnosed.
 */
export function samplesNoter(video: object): (samples: string) => void {
  return (samples) => {
    const diagnosis = diagnoses.get(video);
    if (diagnosis !== undefined) {
      diagnosis.samples = samples;
    }
  };
}

/** The video's diagnosis, its element error over hls.js's; undefined for a video not diagnosed. */
export function diagnosisOf(
  video: object & { readonly error?: MediaError | null },
): PlaybackDiagnosis | undefined {
  const diagnosis = diagnoses.get(video);
  if (diagnosis === undefined) {
    return undefined;
  }
  const elementError = video.error;
  return {
    videoBuffered: diagnosis.video,
    audioBuffered: diagnosis.audio,
    lastError:
      elementError === null || elementError === undefined
        ? diagnosis.lastError
        : `media error ${String(elementError.code)}: ${elementError.message}`.slice(0, 200),
    mapping: diagnosis.mapping.join("; ").slice(0, mappingTextMost),
    mediaSourceState: diagnosis.mediaSourceState,
    // The choices last, which the cut to stepsTextMost keeps: the steps before them are many.
    steps: [
      ...diagnosis.steps,
      ...diagnosis.choices.map((choice) => choiceText(choice, diagnosis.choicesClock?.())),
    ]
      .join("; ")
      .slice(-stepsTextMost),
    ...(diagnosis.samples === ""
      ? {}
      : { linkSamples: diagnosis.samples.slice(0, linkSamplesTextMost) }),
  };
}

/**
 * The codecs video's buffers took, each with its track and container ("video video/mp4 avc1.64001f;
 * audio audio/mp4 mp4a.40.2"), the latest for each; "" for a video not diagnosed or before any: the
 * owner's TCL TV played every film's sound half a second late, and nothing said what it decoded
 * (2026-10-24).
 */
export function codecsOf(video: object): string {
  const diagnosis = diagnoses.get(video);
  if (diagnosis === undefined) {
    return "";
  }
  return [...diagnosis.codecs].map(([track, codec]) => `${track} ${codec}`).join("; ");
}
