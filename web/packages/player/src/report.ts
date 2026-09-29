// SPDX-License-Identifier: MPL-2.0

/** A change of play state (D165): what happened, never a heartbeat. */
export type PlayEventKind = "started" | "paused" | "seeked" | "completed" | "abandoned";

/** One change, at a position in the file and a moment on the device's monotonic clock. */
export interface PlayEvent {
  readonly kind: PlayEventKind;
  readonly positionMs: number;
  readonly happenedMonoMs: number;
}

/** A run of watching that a change closed. */
export interface PlaySpan {
  readonly fromPositionMs: number;
  readonly toPositionMs: number;
  readonly startedMonoMs: number;
  readonly endedMonoMs: number;
}

/** What the player tells the server (cut-a §5): the body of POST /playback/{token}/report. */
export interface PlayReport {
  readonly reportSeq: number;
  readonly sentMonoMs: number;
  readonly positionMs: number;
  readonly positionReadMonoMs: number;
  readonly events: readonly PlayEvent[];
  readonly spans: readonly PlaySpan[];
}

/** How often a playing player reports where it is (play-state.md: about every ten seconds). */
export const reportEveryMs = 10_000;

/**
 * Keeps one playback's play state and reports it: the changes, the spans of watching they close,
 * and where it is. Times are the device's monotonic clock, which the server anchors on arrival, so
 * a report that arrives late never rewinds the position (D217 §7). A report that fails to send is
 * sent again, with its changes, in the next one.
 */
export class PlayReporter {
  private hasStarted = false;
  private events: PlayEvent[] = [];
  private spans: PlaySpan[] = [];
  private openSpan: { fromPositionMs: number; startedMonoMs: number } | undefined;
  private isSending = false;

  /**
   * nextSeq numbers the reports. Every reporter of one playback must share it: the server takes a
   * report numbered no higher than the last as sent again, so a second reporter counting from 1
   * would be ignored.
   */
  constructor(
    private readonly send: (report: PlayReport) => Promise<void>,
    private readonly nextSeq: () => number,
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

  /** Reports where the playback is, with any changes not yet sent. */
  flush(positionMs: number): void {
    if (this.isSending) {
      return;
    }
    const now = this.readMonotonicMs();
    const report: PlayReport = {
      reportSeq: this.nextSeq(),
      sentMonoMs: now,
      positionMs: Math.max(0, Math.round(positionMs)),
      positionReadMonoMs: now,
      events: this.events,
      spans: this.spans,
    };
    this.events = [];
    this.spans = [];
    this.isSending = true;
    this.send(report).then(
      () => {
        this.isSending = false;
      },
      () => {
        this.isSending = false;
        // Kept for the next report, which is numbered afresh: the server took none of this one.
        this.events = [...report.events, ...this.events];
        this.spans = [...report.spans, ...this.spans];
      },
    );
  }

  private recordEvent(kind: PlayEventKind, positionMs: number, monoMs: number): void {
    this.events.push({
      kind,
      positionMs: Math.max(0, Math.round(positionMs)),
      happenedMonoMs: monoMs,
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
