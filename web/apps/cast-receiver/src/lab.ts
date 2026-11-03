// SPDX-License-Identifier: MPL-2.0

import { playChunked, watchStalls, type ChunkedPlayback } from "@orpheum/player";
import {
  browserDeviceFacts,
  deviceReportFrom,
  failureText,
  testLoopback,
  toneTrack,
  type FetchRequest,
  type LoopbackResult,
} from "@orpheum/sdk";

import { reachCore, reachOf, type Reach, type Reached } from "./reach.ts";

/**
 * The lab receiver's spike (plans/tv-device-tests.md; casting-2026-10-25.md §5.2 step 2, review §4):
 * the device report the web app's /lab/device makes, with a WebRTC call with itself; our ladder
 * played by our own player, its events and stalls logged; the page's memory each minute. Everything
 * is shown on screen and sent to the senders, whose page sends it to the server's log.
 */

/** How many lines of log the screen shows and a report carries: well under a Cast message's 64 KiB. */
const linesKept = 150;

/** How often the page's memory is sampled: a leak shows over a two-hour film, not in five minutes. */
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

/** What a sender asks of the receiver: its report, a film to play, or to pause or play on. */
export type LabRequest =
  | { readonly type: "report" }
  | { readonly type: "toggle" }
  | {
      readonly type: "play";
      readonly path: string;
      readonly startAtBps: number;
      readonly authorization: string | undefined;
      /** How to reach the sender's core from away, where it sends one; this page's own core else. */
      readonly reach: Reach | undefined;
    };

/** A sender's message as a request, or undefined for anything else. */
export function labRequestOf(message: unknown): LabRequest | undefined {
  if (typeof message !== "object" || message === null) {
    return undefined;
  }
  const type: unknown = Reflect.get(message, "type");
  if (type === "report" || type === "toggle") {
    return { type };
  }
  const path: unknown = Reflect.get(message, "path");
  const startAtBps: unknown = Reflect.get(message, "startAtBps");
  const authorization: unknown = Reflect.get(message, "authorization");
  if (type !== "play" || typeof path !== "string" || !path.startsWith("/")) {
    return undefined;
  }
  return {
    type,
    path,
    startAtBps: typeof startAtBps === "number" ? startAtBps : 0,
    authorization: typeof authorization === "string" ? authorization : undefined,
    reach: reachOf(Reflect.get(message, "reach")),
  };
}

/** The receiver's session: its log on screen, its report, and its film. */
export class LabReceiver {
  private readonly startedAtMs = performance.now();
  private readonly lines: string[] = [];
  private readonly memory: MemorySample[] = [];
  private stallCount = 0;
  private playback: ChunkedPlayback | undefined;
  private stopWatching: (() => void) | undefined;
  private reached: Reached | undefined;
  /** Each play asked, counted, so a play whose core is reached after a later one was asked yields. */
  private plays = 0;

  constructor(
    private readonly screen: HTMLElement,
    private readonly video: HTMLVideoElement,
    private readonly send: (message: Readonly<Record<string, unknown>>) => void,
  ) {
    for (const type of loggedVideoEvents) {
      video.addEventListener(type, () => {
        this.note(`video ${type} at ${video.currentTime.toFixed(1)} s`);
      });
    }
    document.addEventListener("securitypolicyviolation", (event) => {
      this.note(`blocked by the page's policy: ${event.violatedDirective} ${event.blockedURI}`);
    });
  }

  /** Notes line on screen and in the log the report carries. */
  note(line: string): void {
    const atS = ((performance.now() - this.startedAtMs) / 1000).toFixed(1);
    this.lines.push(`${atS} s ${line}`);
    if (this.lines.length > linesKept) {
      this.lines.shift();
    }
    // Newest first; not toReversed, which the 3rd-generation Chromecast's Chromium 90 lacks.
    this.screen.textContent = [...this.lines].reverse().join("\n");
  }

  /** Answers a sender's request. */
  heard(message: unknown): void {
    const request = labRequestOf(message);
    if (request === undefined) {
      this.note(`a message it does not know: ${JSON.stringify(message).slice(0, 200)}`);
      return;
    }
    if (request.type === "report") {
      void this.report();
      return;
    }
    if (request.type === "toggle") {
      this.toggle();
      return;
    }
    this.play(request);
  }

  /** Samples the page's memory, and tells the senders. */
  sampleMemory(): void {
    const sample = memoryOf(
      Reflect.get(performance, "memory"),
      Math.round((performance.now() - this.startedAtMs) / 1000),
    );
    if (sample === undefined) {
      return;
    }
    this.memory.push(sample);
    this.note(`memory ${String(sample.usedMb)} of ${String(sample.limitMb)} MiB`);
    this.send({ type: "memory", ...sample });
  }

  /** Makes the device report, with a call with itself, and sends it. */
  async report(): Promise<void> {
    this.note("making the report");
    const loopback = await loopbackWithTone();
    this.note(
      typeof loopback === "string"
        ? `WebRTC: ${loopback}`
        : `WebRTC: ${loopback.failure ?? "connected"}`,
    );
    this.send({
      type: "report",
      ...deviceReportFrom(browserDeviceFacts()),
      loopback,
      stalls: this.stallCount,
      memory: this.memory,
      log: this.lines,
    });
  }

  private toggle(): void {
    if (this.video.paused) {
      void this.video.play().catch((cause: unknown) => {
        this.note(`play refused: ${failureText(cause)}`);
      });
    } else {
      this.video.pause();
    }
  }

  private play(request: Extract<LabRequest, { readonly type: "play" }>): void {
    this.stopWatching?.();
    this.stopWatching = undefined;
    this.playback?.destroy();
    this.playback = undefined;
    this.reached?.close();
    this.reached = undefined;
    this.plays += 1;
    const play = this.plays;
    if (request.reach === undefined) {
      this.playOn(request, undefined);
      return;
    }
    void reachCore(request.reach, (line) => {
      this.note(line);
    }).then(
      (reached) => {
        if (play !== this.plays) {
          reached.close();
          return;
        }
        this.reached = reached;
        this.playOn(request, reached.fetch);
      },
      (cause: unknown) => {
        this.note(`could not reach the core: ${failureText(cause)}`);
      },
    );
  }

  /** Plays request's film by fetch, or this page's own requests without one. */
  private playOn(
    request: Extract<LabRequest, { readonly type: "play" }>,
    fetch: FetchRequest | undefined,
  ): void {
    const url = new URL(request.path, globalThis.location.origin).href;
    const authorization =
      request.reach === undefined ? request.authorization : `Bearer ${request.reach.token}`;
    this.note(
      `playing ${request.path}${authorization === undefined ? ", signed in by nothing" : ""}`,
    );
    this.playback = playChunked(this.video, url, {
      onFailed: (failure) => {
        this.note(`failed: ${failure}`);
      },
      startAtBps: request.startAtBps,
      startPositionS: 0,
      monotonicMs: () => performance.now(),
      authorization,
      fetch,
    });
    this.stopWatching = watchStalls(this.video, (stall) => {
      this.stallCount = stall.count;
      this.note(`stall ${String(stall.count)} at ${(stall.positionMs / 1000).toFixed(1)} s`);
    });
    void this.video.play().catch((cause: unknown) => {
      this.note(`play refused: ${failureText(cause)}`);
    });
  }
}

/** Calls the page with itself carrying a tone, its arrival played quietly by an element of its own. */
async function loopbackWithTone(): Promise<LoopbackResult | string> {
  if (!("RTCPeerConnection" in globalThis) || !("AudioContext" in globalThis)) {
    return "no RTCPeerConnection or AudioContext";
  }
  const context = new AudioContext();
  const playOut = new Audio();
  playOut.volume = 0.05;
  try {
    await context.resume();
    const tone = toneTrack(context);
    try {
      return await testLoopback(tone.track, (received) => {
        playOut.srcObject = received ?? null;
        void playOut.play().catch(() => undefined);
      });
    } finally {
      tone.stop();
    }
  } catch (cause) {
    return failureText(cause);
  } finally {
    await context.close().catch(() => undefined);
  }
}
