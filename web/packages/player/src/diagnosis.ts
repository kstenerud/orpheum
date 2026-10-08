// SPDX-License-Identifier: MPL-2.0

import { playerBufferedFirstEvent } from "@orpheum/sdk";
import Hls, { type ErrorData } from "hls.js";

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
  readonly streaming: string;
}

/** What a diagnosis keeps as hls.js tells it. */
interface Kept {
  video: string;
  audio: string;
  lastError: string;
  mapping: string[];
  streaming: string;
}

/** The longest a diagnosis's mapping may be, as the server takes it. */
const mappingTextMost = 300;

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
    ) => void);
}

/**
 * Keeps video's diagnosis as hls.js appends to its buffers and reports errors, and tells video by
 * playerBufferedFirstEvent as each playlist's first chunk is buffered.
 */
export function diagnose(hls: DiagnosedHls, video: EventTarget): void {
  const diagnosis: Kept = { video: "", audio: "", lastError: "", mapping: [], streaming: "" };
  diagnoses.set(video, diagnosis);
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
  });
  // How hls.js placed its first chunks, and whether its media source lets it load.
  const buffered = new Set<string>();
  hls.on(Hls.Events.INIT_PTS_FOUND, (_event, data) => {
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
  hls.on(Hls.Events.MEDIA_ATTACHED, (_event, data) => {
    data.mediaSource?.addEventListener("startstreaming", () => {
      diagnosis.streaming = "streaming";
    });
    data.mediaSource?.addEventListener("endstreaming", () => {
      diagnosis.streaming = "stopped";
    });
  });
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
    streaming: diagnosis.streaming,
  };
}
