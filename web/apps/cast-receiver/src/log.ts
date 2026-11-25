// SPDX-License-Identifier: MPL-2.0

/**
 * The receiver's log (the owner, 2026-11-12): lines it notes from its start, the video's events and
 * the page's memory each minute among them, shown on screen, newest first, once a sender asks for
 * it (a cast whose sender's person shows debug tools and ticked Show cast log). Until then the screen
 * shows the film alone, the notes kept all the while.
 */

/** How many lines of log the screen shows: a page's worth, enough to read a cast's start by. */
const linesKept = 150;

/** How often the page's memory is noted: a leak shows over a two-hour film, not in five minutes. */
export const memoryEveryMs = 60_000;

/** The video element's events the log notes. */
const loggedVideoEvents = [
  "loadedmetadata",
  "canplay",
  "playing",
  "waiting",
  "stalled",
  "pause",
  "ended",
  "error",
  "seeking",
  "seeked",
] as const;

/** What the page says of its memory where it says anything (Chromium's performance.memory). */
export interface MemorySample {
  readonly atS: number;
  readonly usedMb: number;
  readonly totalMb: number;
  readonly limitMb: number;
}

/** The page's memory now, in MiB, or undefined where the browser does not say. */
export function memoryOf(memory: unknown, atS: number): MemorySample | undefined {
  if (typeof memory !== "object" || memory === null) {
    return undefined;
  }
  const inMb = (name: string): number | undefined => {
    const bytes: unknown = Reflect.get(memory, name);
    return typeof bytes === "number" ? Math.round(bytes / 1024 / 102.4) / 10 : undefined;
  };
  const used = inMb("usedJSHeapSize");
  const total = inMb("totalJSHeapSize");
  const limit = inMb("jsHeapSizeLimit");
  return used === undefined || total === undefined || limit === undefined
    ? undefined
    : { atS, usedMb: used, totalMb: total, limitMb: limit };
}

/** The receiver's log, on screen once asked for: its video's events, its page's refusals and memory. */
export class ReceiverLog {
  private readonly startedAtMs = performance.now();
  private readonly lines: string[] = [];

  constructor(
    private readonly screen: HTMLElement,
    video: HTMLVideoElement,
  ) {
    this.screen.hidden = true; // until a sender asks for it (showLog)
    for (const type of loggedVideoEvents) {
      video.addEventListener(type, () => {
        this.note(`video ${type} at ${video.currentTime.toFixed(1)} s`);
      });
    }
    document.addEventListener("securitypolicyviolation", (event) => {
      this.note(`blocked by the page's policy: ${event.violatedDirective} ${event.blockedURI}`);
    });
  }

  /** Notes line in the log, and on screen where the log shows (showLog). */
  note(line: string): void {
    const atS = ((performance.now() - this.startedAtMs) / 1000).toFixed(1);
    this.lines.push(`${atS} s ${line}`);
    if (this.lines.length > linesKept) {
      this.lines.shift();
    }
    // Newest first; not toReversed, which the 3rd-generation Chromecast's Chromium 90 lacks.
    this.screen.textContent = [...this.lines].reverse().join("\n");
  }

  /** Shows the log on screen from now on; the notes are kept whether or not it shows. */
  showLog(): void {
    this.screen.hidden = false;
  }

  /** Notes the page's memory, where the browser says it. */
  noteMemory(): void {
    const sample = memoryOf(
      Reflect.get(performance, "memory"),
      Math.round((performance.now() - this.startedAtMs) / 1000),
    );
    if (sample !== undefined) {
      this.note(`memory ${String(sample.usedMb)} of ${String(sample.limitMb)} MiB`);
    }
  }
}
