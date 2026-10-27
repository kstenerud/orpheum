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
   * after, and the link's last samples last (D352 §5).
   */
  readonly steps?: string;
}

/** What a diagnosis keeps as hls.js tells it. */
interface Kept {
  video: string;
  audio: string;
  lastError: string;
  mapping: string[];
  mediaSourceState: string;
  steps: string[];
  /** The player's own last choices of quality, with why, kept apart so the steps never push them out. */
  choices: string[];
  /** The link's last samples in a line (samplesText), kept after the choices (D352 §5); "" before any. */
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

/** The hls.js events a diagnosis keeps as steps, beside its errors and its video's attaching. */
const stepEvents = [
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
  readonly frag?: {
    readonly level: number;
    readonly sn: number | "initSegment";
    readonly type: string;
  };
}

/** A step in a line: "level 2", "playlist 2 loaded", "frag 2/31 aborted", "sound 0/31 loading". */
export function stepText(event: string, data: StepData): string {
  const { frag } = data;
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

/** What hls.js said went wrong, in a line. */
function errorText(data: Pick<ErrorData, "details" | "fatal" | "error">): string {
  return `${data.details}${data.fatal ? " (fatal)" : ""}: ${data.error.message}`.slice(0, 200);
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
    ((
      event: typeof Hls.Events.ERROR,
      listener: (event: string, data: Pick<ErrorData, "details" | "fatal" | "error">) => void,
    ) => void) &
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
 * (the owner's Firefox on nas.example, 2026-10-20); nothing for a video not diagnosed.
 */
export function stepNoter(video: object): (step: string) => void {
  return (step) => {
    const diagnosis = diagnoses.get(video);
    if (diagnosis !== undefined) {
      diagnosis.choices.push(step);
      diagnosis.choices.splice(0, Math.max(0, diagnosis.choices.length - choicesKept));
    }
  };
}

/**
 * What keeps the link's last samples in video's diagnosis, the latest line replacing the one before,
 * for every stall's and seek's report with its steps (D352 §5): the owner's report of a film held at
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
    // The choices and the link's samples last, which the cut to stepsTextMost keeps: the steps
    // around them are many.
    steps: [
      ...diagnosis.steps,
      ...diagnosis.choices.map((choice) => `chose ${choice}`),
      ...(diagnosis.samples === "" ? [] : [diagnosis.samples]),
    ]
      .join("; ")
      .slice(-stepsTextMost),
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
