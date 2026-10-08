// SPDX-License-Identifier: MPL-2.0

import { diagnosisOf } from "./diagnosis.ts";
import type { components } from "@orpheum/sdk";

// The report's shapes are the API's, generated into the SDK, so a field the server renames is
// renamed here by the compiler rather than copied by hand. Only the types are taken: the player
// never calls the server itself.
type Schemas = components["schemas"];

/** One change, at a position in the file and a moment on the device's monotonic clock. */
export type PlayEvent = Readonly<Schemas["EventBody"]>;

/** A change of play state (D165): what happened, never a heartbeat. */
export type PlayEventKind = PlayEvent["kind"];

/** A run of watching that a change closed. */
export type PlaySpan = Readonly<Schemas["SpanBody"]>;

/**
 * What the player tells the server (cut-a §5): the body of POST /playback/{token}/report. The
 * player always sends its changes and spans, if only none, where the API also takes null.
 */
export type PlayReport = Readonly<
  Omit<Schemas["ReportInputBody"], "events" | "spans" | "stalls">
> & {
  readonly events: readonly PlayEvent[];
  readonly spans: readonly PlaySpan[];
  readonly stalls?: readonly PlayStall[];
};

/** A stall of the player, where it was (D247 §2): about its link, not what was watched. */
export type PlayStall = Readonly<Schemas["StallBody"]>;

/** How long a playback took to start, from asking how the file plays: told once. */
export type PlayStartup = Readonly<Schemas["StartupBody"]>;

/** A quality the player gave up as one its device cannot play, and where (D266). */
export type PlayUnplayable = Readonly<Schemas["UnplayableBody"]>;

/** How often a playing player reports where it is (play-state.md: about every ten seconds). */
export const reportEveryMs = 10_000;

/**
 * What send rejects with when a report failed for good: the server refused it for a reason that
 * sending it again cannot change (the playback it names is gone). The reporter then stops; the
 * cause is what the server said.
 */
export class ReportRefused extends Error {
  override readonly cause: unknown;

  constructor(cause: unknown) {
    super("the server refused the play report for good");
    this.name = "ReportRefused";
    this.cause = cause;
  }
}

/**
 * Keeps one playback's play state and reports it: the changes, the spans of watching they close,
 * and where it is. Times are the device's monotonic clock, which the server anchors on arrival, so
 * a report that arrives late never rewinds the position (D217 §7). A report that fails to send is
 * sent again, with its changes, in the next one, unless send rejected with ReportRefused: then
 * nothing more is sent, since nothing would be taken.
 */
/**
 * The longest a stall's buffered ranges may be, as the server takes them (StallBody): a buffer
 * fragmented by many seeks keeps its latest ranges, at the end.
 */
export const bufferedTextMost = 400;

export class PlayReporter {
  private hasStarted = false;
  private events: PlayEvent[] = [];
  private spans: PlaySpan[] = [];
  private stalls: PlayStall[] = [];
  private startup: PlayStartup | undefined;
  private unplayable: PlayUnplayable | undefined;
  private readLinkKbps: () => number | undefined = () => undefined;
  private openSpan: { fromPositionMs: number; startedMonoMs: number } | undefined;
  private isSending = false;
  private isRefused = false;

  /**
   * nextReportSeq numbers the reports. Every reporter of one playback must share it: the server
   * takes a report numbered no higher than the last as sent again, so a second reporter counting
   * from 1 would be ignored.
   */
  constructor(
    private readonly send: (report: PlayReport) => Promise<void>,
    private readonly nextReportSeq: () => number,
    private readonly readMonotonicMs: () => number = () => Math.round(performance.now()),
  ) {}

  /** Playing began or resumed at positionMs. */
  recordStarted(positionMs: number): void {
    const now = this.readMonotonicMs();
    this.hasStarted = true;
    this.openSpan ??= { fromPositionMs: positionMs, startedMonoMs: now };
    this.recordEvent("started", positionMs, now);
  }

  /** Playing stopped at positionMs. */
  recordPaused(positionMs: number): void {
    this.recordEvent("paused", positionMs, this.closeSpan(positionMs));
  }

  /**
   * The person moved from fromMs to toMs. What was playing before the move is a span; watching
   * goes on from toMs if it was playing.
   */
  recordSeeked(fromMs: number, toMs: number): void {
    const wasPlaying = this.openSpan !== undefined;
    const now = this.closeSpan(fromMs);
    if (wasPlaying) {
      this.openSpan = { fromPositionMs: toMs, startedMonoMs: now };
    }
    this.recordEvent("seeked", toMs, now);
  }

  /**
   * Reports the link's speed, as read measures it, with every report from now on (D247 §1);
   * undefined when it cannot say yet.
   */
  measureLinkWith(read: () => number | undefined): void {
    this.readLinkKbps = read;
  }

  /**
   * The player stalled at positionMs (D247 §2), the video then at readyState, paused or not, with
   * what its buffers held and its last error, which say whether it lacked data or could not play
   * what it had (D266); the server is told at once.
   */
  recordStall(
    positionMs: number,
    video?: Pick<HTMLMediaElement, "readyState" | "paused" | "error">,
  ): void {
    const diagnosis = video === undefined ? undefined : diagnosisOf(video);
    this.stalls.push({
      positionMs: Math.max(0, Math.round(positionMs)),
      ...(video === undefined ? {} : { readyState: video.readyState, isPaused: video.paused }),
      // Only what a stall's body takes: the server refuses any other field, and a refused report
      // silences the reporter for good. The whole diagnosis, spread in, carried the mapping a
      // Watch Together note added, and every chunked playback's stall was refused (2026-10-08).
      ...(diagnosis === undefined
        ? {}
        : {
            videoBuffered: diagnosis.videoBuffered.slice(-bufferedTextMost),
            audioBuffered: diagnosis.audioBuffered.slice(-bufferedTextMost),
            lastError: diagnosis.lastError,
          }),
    });
    this.flush(positionMs);
  }

  /** The file played to its end. */
  recordCompleted(positionMs: number): void {
    this.recordEvent("completed", positionMs, this.closeSpan(positionMs));
  }

  /** The person left the playback at positionMs; a playback that never played says nothing. */
  recordAbandoned(positionMs: number): void {
    if (this.hasStarted) {
      this.recordEvent("abandoned", positionMs, this.closeSpan(positionMs));
    }
  }

  /**
   * How long the playback took to start, sent with the next report: the one its first started
   * event sends at once, when it is recorded just before.
   */
  recordStartup(startup: PlayStartup): void {
    this.startup = startup;
  }

  /** The player gave up a quality as one its device cannot play; the server is told at once (D266). */
  recordUnplayable(unplayable: PlayUnplayable, positionMs: number): void {
    this.unplayable = unplayable;
    this.flush(positionMs);
  }

  /** Reports where the playback is, with any changes not yet sent. */
  flush(positionMs: number): void {
    if (this.isSending || this.isRefused) {
      return;
    }
    const now = this.readMonotonicMs();
    const linkKbps = this.readLinkKbps();
    const report: PlayReport = {
      reportSeq: this.nextReportSeq(),
      sentMonoMs: now,
      positionMs: Math.max(0, Math.round(positionMs)),
      positionReadMonoMs: now,
      events: this.events,
      spans: this.spans,
      ...(this.stalls.length === 0 ? {} : { stalls: this.stalls }),
      ...(linkKbps === undefined ? {} : { linkKbps: Math.round(linkKbps) }),
      ...(this.startup === undefined ? {} : { startup: this.startup }),
      ...(this.unplayable === undefined ? {} : { unplayable: this.unplayable }),
    };
    this.events = [];
    this.spans = [];
    this.stalls = [];
    this.startup = undefined;
    this.unplayable = undefined;
    this.isSending = true;
    this.send(report).then(
      () => {
        this.isSending = false;
      },
      (cause: unknown) => {
        this.isSending = false;
        if (cause instanceof ReportRefused) {
          this.isRefused = true;
          return;
        }
        // Kept for the next report, which is numbered afresh: the server took none of this one.
        this.events = [...report.events, ...this.events];
        this.spans = [...report.spans, ...this.spans];
        this.stalls = [...(report.stalls ?? []), ...this.stalls];
        this.startup ??= report.startup ?? undefined;
        this.unplayable ??= report.unplayable ?? undefined;
      },
    );
  }

  private recordEvent(kind: PlayEventKind, positionMs: number, happenedMonoMs: number): void {
    this.events.push({
      kind,
      positionMs: Math.max(0, Math.round(positionMs)),
      happenedMonoMs,
    });
    this.flush(positionMs);
  }

  /** Closes the open span at positionMs, if one is open, and says when. */
  private closeSpan(positionMs: number): number {
    const now = this.readMonotonicMs();
    if (this.openSpan !== undefined) {
      this.spans.push({
        fromPositionMs: Math.max(0, Math.round(this.openSpan.fromPositionMs)),
        toPositionMs: Math.max(0, Math.round(positionMs)),
        startedMonoMs: this.openSpan.startedMonoMs,
        endedMonoMs: now,
      });
      this.openSpan = undefined;
    }
    return now;
  }
}

/** Moves a video to positionMs: where the person left it, or the start. */
export function seekTo(video: HTMLVideoElement, positionMs: number): void {
  // eslint-disable-next-line no-param-reassign -- a media element seeks when currentTime is set
  video.currentTime = positionMs / 1000;
}
