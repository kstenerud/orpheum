// SPDX-License-Identifier: MPL-2.0

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
}

const diagnoses = new WeakMap<object, { video: string; audio: string; lastError: string }>();

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

/** Keeps video's diagnosis as hls.js appends to its buffers and reports errors. */
export function diagnose(hls: DiagnosedHls, video: object): void {
  const diagnosis = { video: "", audio: "", lastError: "" };
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
  };
}
