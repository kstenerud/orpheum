// SPDX-License-Identifier: MPL-2.0

/**
 * A chunked playback's loading asked again after hls.js stopped it for a load that failed: never the
 * end of the playback (isLoadStopped).
 */

import Hls, { type ErrorData } from "hls.js";

import { recoverFrom } from "./abr-controller.ts";
import type { AskingAgain } from "./abr-on-its-way.ts";
import { failureText } from "./diagnosis.ts";
import { isLoadStopped } from "./failure.ts";
import { resumePositionOf, type ResumingHls } from "./resume-position.ts";

/** What asking hls.js to load on needs of it: its errors, its loading, its playlist and its video. */
export type LoadingOnHls = Pick<Hls, "on" | "off" | "startLoad" | "loadSource" | "levels" | "url"> &
  ResumingHls;

/**
 * Never lets a load that failed end the playback: at each fatal hls.js error that only stopped its
 * loading (isLoadStopped), the error is marked recovered, so failOnFatal, told after, passes it by,
 * and hls.js is asked to load on from where the playback resumes (resumePositionOf), never from a
 * video a rebuild put at 0, after the wait askingAgain says, the
 * bitrate controller's for asking again, counted in a row with its chunks stuck on their way, each
 * twice as long (Choosing.askingAgain); and at once when page, the window, says the browser is back
 * online. What is loaded plays on meanwhile, the page showing only its wait. Each is noted: what
 * failed and what carried it (failureText, by carrierOf), the time in a row and the wait, and each
 * ask with why then. Taken as the end of the playback, a load that failed while the owner's Chrome
 * was offline stopped its loading for good: it played its minute out and stood still, though the
 * link came back (staging, 2026-10-29; VQ125). Returns the stopping.
 */
export function loadOnAfterFailures(
  hls: LoadingOnHls,
  by: {
    readonly askingAgain: () => AskingAgain | undefined;
    readonly note: (note: string) => void;
    readonly carrierOf: (url: string) => string;
    readonly page: Pick<EventTarget, "addEventListener" | "removeEventListener"> | undefined;
  },
): () => void {
  let waiting: ReturnType<typeof setTimeout> | undefined;
  const loadOn = (why: string): void => {
    waiting = undefined;
    loadOnFromTheVideo(hls, by.note, why);
  };
  const onError = (_event: unknown, data: ErrorData): void => {
    if (!isLoadStopped(data)) {
      return;
    }
    recoverFrom(data);
    const asking = waiting === undefined ? by.askingAgain() : undefined;
    if (asking === undefined) {
      return; // one waiting already stands
    }
    const { waitMs, inARow } = asking;
    by.note(
      `loading stopped: ${failureText(data, by.carrierOf)}; asking again in ${String(waitMs)} ms,` +
        ` time ${String(inARow)} in a row`,
    );
    waiting = setTimeout(() => {
      loadOn(`after ${String(waitMs)} ms`);
    }, waitMs);
  };
  const onOnline = (): void => {
    if (waiting !== undefined) {
      clearTimeout(waiting);
      loadOn("the browser back online");
    }
  };
  hls.on(Hls.Events.ERROR, onError);
  by.page?.addEventListener("online", onOnline);
  return () => {
    clearTimeout(waiting);
    hls.off(Hls.Events.ERROR, onError);
    by.page?.removeEventListener("online", onOnline);
  };
}

/**
 * Asks hls.js to load on from where the playback resumes (resumePositionOf), noting why now: where its
 * video stands, or, after a rebuild of its decoding put the video at 0, where it stood before; its
 * master playlist asked for again where none loaded, there being nothing to load on from.
 */
function loadOnFromTheVideo(hls: LoadingOnHls, note: (note: string) => void, why: string): void {
  if (hls.levels.length === 0 && hls.url !== null) {
    note(`asked again for the playlist, ${why}`);
    hls.loadSource(hls.url);
    return;
  }
  const positionS = resumePositionOf(hls);
  note(`asked again to load from ${positionS.toFixed(1)} s, ${why}`);
  hls.startLoad(positionS);
}
