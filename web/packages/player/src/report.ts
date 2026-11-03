// SPDX-License-Identifier: MPL-2.0

import { linkNotesKept } from "./link-estimate.ts";
import { diagnosisOf } from "./diagnosis.ts";
import { framesBetween, type FramesShown } from "./frames.ts";
import type { components } from "@orpheum/sdk";

// The report's shapes are the API's, generated into the SDK, so a field the server renames is
// renamed here by the compiler rather than copied by hand. Only the types are taken: the player
// never calls the server itself.
type Schemas = components["schemas"];

/** One change, at a position in the file and a moment on the device's monotonic clock. */
export type PlayEvent = Readonly<Schemas["EventBody"]>;

/** A change of play state (D165): what happened, never a heartbeat. */
export type PlayEventKind = PlayEvent["kind"];

/** What was true of the page as it paused or moved, which says where the pause or the move came from. */
export type PlayEventContext = Readonly<Schemas["EventContextBody"]>;

export type PlayPathMove = Readonly<Schemas["PathMoveBody"]>;

/** A run of watching that a change closed. */
export type PlaySpan = Readonly<Schemas["SpanBody"]>;

/**
 * What the player tells the server (cut-a §5): the body of POST /playback/{token}/report. The
 * player always sends its changes and spans, if only none, where the API also takes null.
 */
export type PlayReport = Readonly<
  Omit<Schemas["ReportInputBody"], "events" | "spans" | "stalls" | "pathMoves">
> & {
  readonly events: readonly PlayEvent[];
  readonly spans: readonly PlaySpan[];
  readonly stalls?: readonly PlayStall[];
  readonly pathMoves?: readonly PlayPathMove[];
};

/** A stall of the player, where it was (D247 §2): about its link, not what was watched. */
export type PlayStall = Readonly<Schemas["StallBody"]>;

/** How long a playback took to start, from asking how the file plays: told once. */
export type PlayStartup = Readonly<Schemas["StartupBody"]>;

/** A quality the player gave up as one its device cannot play, and where (D266). */
export type PlayUnplayable = Readonly<Schemas["UnplayableBody"]>;

/** Where the player, asked to play, kept its position, and what its video showed. */
export type PlayStandstill = Readonly<Schemas["StandstillBody"]>;

/** How long one of the person's seeks took to play again. */
export type PlaySeek = Readonly<Schemas["SeekBody"]>;

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
 * sent again as it was, under its own reportSeq, unless send rejected with ReportRefused: then
 * nothing more is sent, since nothing would be taken.
 */
/**
 * The longest a stall's buffered ranges may be, as the server takes them (StallBody): a buffer
 * fragmented by many seeks keeps its latest ranges, at the end.
 */
export const bufferedTextMost = 400;

/** The longest path name a move is reported with, and the most moves one report carries (the API's). */
const pathNameMost = 200;
const pathMovesMost = 20;

/** How a report goes: isLeaving when the page is going, so the browser must finish it unattended (keepalive). */
export interface SendOptions {
  readonly isLeaving: boolean;
}

export class PlayReporter {
  private hasStarted = false;
  private events: PlayEvent[] = [];
  private spans: PlaySpan[] = [];
  private stalls: PlayStall[] = [];
  private startup: PlayStartup | undefined;
  private unplayables: PlayUnplayable[] = [];
  private seeks: PlaySeek[] = [];
  private standstills: PlayStandstill[] = [];
  private pathMoves: PlayPathMove[] = [];
  private readLinkKbps: () => number | undefined = () => undefined;
  private readSteadiness: () => number | undefined = () => undefined;
  private readLinkSpread: () => number | undefined = () => undefined;
  private takeLinkNotes: () => readonly string[] = () => [];
  /** Notes on the link a report that failed carried, sent again with the next. */
  private linkNotesUnsent: string[] = [];
  private readFrames: () => FramesShown | undefined = () => undefined;
  /** The frames counted when the last report was sent, from which the next counts. */
  private framesSent: FramesShown | undefined;
  private openSpan: { fromPositionMs: number; startedMonoMs: number } | undefined;
  private isSending = false;
  private isRefused = false;
  /**
   * The report whose sending failed, sent again as it was, under its own reportSeq: if its answer
   * was only lost, the server drops it as seen twice (L2). What was gathered since waits for the
   * report after it.
   */
  private unsent: PlayReport | undefined;
  /** A flush asked while a report was in flight, made when it settles (L1). */
  private flushAsked: { positionMs: number; isLeaving: boolean } | undefined;

  /**
   * nextReportSeq numbers the reports. Every reporter of one playback must share it: the server
   * takes a report numbered no higher than the last as sent again, so a second reporter counting
   * from 1 would be ignored.
   */
  constructor(
    private readonly send: (report: PlayReport, options: SendOptions) => Promise<void>,
    private readonly nextReportSeq: () => number,
    private readonly readMonotonicMs: () => number = () => Math.round(performance.now()),
  ) {}

  /** Playing began or resumed at positionMs, with what was true as it did. */
  recordStarted(positionMs: number, context?: PlayEventContext): void {
    const now = this.readMonotonicMs();
    this.hasStarted = true;
    this.openSpan ??= { fromPositionMs: positionMs, startedMonoMs: now };
    this.recordEvent({ kind: "started", positionMs, happenedMonoMs: now, context });
  }

  /** Playing stopped at positionMs, with what was true of the page then. */
  recordPaused(positionMs: number, context?: PlayEventContext): void {
    this.recordEvent({
      kind: "paused",
      positionMs,
      happenedMonoMs: this.closeSpan(positionMs),
      context,
    });
  }

  /**
   * The person moved from fromMs to toMs. What was playing before the move is a span; watching
   * goes on from toMs if it was playing.
   */
  recordSeeked(fromMs: number, toMs: number, context?: PlayEventContext): void {
    const wasPlaying = this.openSpan !== undefined;
    const now = this.closeSpan(fromMs);
    if (wasPlaying) {
      this.openSpan = { fromPositionMs: toMs, startedMonoMs: now };
    }
    this.recordEvent({ kind: "seeked", positionMs: toMs, happenedMonoMs: now, context });
  }

  /** The page moved to another path to the core, named to; sent with the next report. */
  recordPathMove(to: string): void {
    this.pathMoves.push({ to: to.slice(0, pathNameMost), happenedMonoMs: this.readMonotonicMs() });
  }

  /**
   * Reports the link's speed, as read measures it, with every report from now on (D247 §1);
   * undefined when it cannot say yet.
   */
  measureLinkWith(read: () => number | undefined): void {
    this.readLinkKbps = read;
  }

  /**
   * Reports the steadiness level of the chunked player, as read says it, with every report from now
   * on, which the server keeps beside the link's speed for the device's next playback on its
   * network (D352 Q2); undefined from a player that has none.
   */
  reportSteadinessWith(read: () => number | undefined): void {
    this.readSteadiness = read;
  }

  /**
   * Reports the spread of the link's samples, in natural log, as read says it, with every report
   * that carries the link's speed from now on, which the server keeps beside it for the device's
   * next start (bandwidth-sampling-2026-10-29.md P2); undefined where it says none.
   */
  reportLinkSpreadWith(read: () => number | undefined): void {
    this.readLinkSpread = read;
  }

  /**
   * Reports the player's notes on its link since the last report, as take hands them over, with
   * every report from now on, which the server logs: each sample with its expected time and its
   * bound's, each chunk given up or stuck and what carried it (bandwidth-sampling-2026-10-29.md P6;
   * the owner, 2026-10-29).
   */
  reportLinkNotesWith(take: () => readonly string[]): void {
    this.takeLinkNotes = take;
  }

  /**
   * Reports the frames the video showed and dropped since the last report, as read counts them
   * from its start, with every report from now on, so a burst of dropped frames shows in the log
   * near when it happened (the owner, 2026-10-28); none where read says undefined.
   */
  reportFramesWith(read: () => FramesShown | undefined): void {
    this.readFrames = read;
    this.framesSent = read();
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
            ...(diagnosis.steps === undefined || diagnosis.steps === ""
              ? {}
              : { steps: diagnosis.steps }),
            ...(diagnosis.linkSamples === undefined || diagnosis.linkSamples === ""
              ? {}
              : { linkSamples: diagnosis.linkSamples }),
          }),
    });
    this.flush(positionMs);
  }

  /** The file played to its end. */
  recordCompleted(positionMs: number): void {
    this.recordEvent({ kind: "completed", positionMs, happenedMonoMs: this.closeSpan(positionMs) });
  }

  /** The person left the playback at positionMs; a playback that never played says nothing. */
  recordAbandoned(positionMs: number): void {
    if (this.hasStarted) {
      this.recordEvent({
        kind: "abandoned",
        positionMs,
        happenedMonoMs: this.closeSpan(positionMs),
      });
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
    this.unplayables.push(unplayable);
    this.flush(positionMs);
  }

  /** The player, asked to play, kept its position; the server is told at once. */
  recordStandstill(standstill: PlayStandstill): void {
    this.standstills.push(standstill);
    this.flush(standstill.positionMs);
  }

  /**
   * One of the person's seeks played again, sent with the next report: the one its started event
   * sends at once, when it is recorded just before.
   */
  recordSeekReady(seek: PlaySeek): void {
    this.seeks.push(seek);
  }

  /**
   * Reports where the playback is, with any changes not yet sent. isLeaving says the page is going
   * (hidden, or pagehide), so the report is sent to survive it. Asked while a report is in flight,
   * it is made when that one settles; after a report that failed, the first goes again as it was.
   */
  flush(positionMs: number, isLeaving = false): void {
    if (this.isRefused) {
      return;
    }
    if (this.isSending) {
      this.flushAsked = {
        positionMs,
        isLeaving: isLeaving || (this.flushAsked?.isLeaving ?? false),
      };
      return;
    }
    if (this.unsent !== undefined) {
      this.flushAsked = { positionMs, isLeaving };
      this.transmit(this.unsent, isLeaving);
      return;
    }
    const now = this.readMonotonicMs();
    const linkNotes = [...this.linkNotesUnsent, ...this.takeLinkNotes()].slice(-linkNotesKept);
    this.linkNotesUnsent = [];
    const report: PlayReport = {
      ...this.framesSinceSent(),
      ...(linkNotes.length === 0 ? {} : { linkNotes }),
      reportSeq: this.nextReportSeq(),
      sentMonoMs: now,
      positionMs: Math.max(0, Math.round(positionMs)),
      positionReadMonoMs: now,
      events: this.events,
      spans: this.spans,
      ...(this.stalls.length === 0 ? {} : { stalls: this.stalls }),
      ...this.linkNow(),
      ...(this.startup === undefined ? {} : { startup: this.startup }),
      ...(this.unplayables.length === 0 ? {} : { unplayables: this.unplayables }),
      ...(this.seeks.length === 0 ? {} : { seeks: this.seeks }),
      ...(this.standstills.length === 0 ? {} : { standstills: this.standstills }),
      ...(this.pathMoves.length === 0 ? {} : { pathMoves: this.pathMoves.slice(-pathMovesMost) }),
    };
    this.events = [];
    this.spans = [];
    this.stalls = [];
    this.startup = undefined;
    this.unplayables = [];
    this.seeks = [];
    this.standstills = [];
    this.pathMoves = [];
    this.transmit(report, isLeaving);
  }

  private transmit(report: PlayReport, isLeaving: boolean): void {
    this.isSending = true;
    this.send(report, { isLeaving }).then(
      () => {
        this.isSending = false;
        this.unsent = undefined;
        const asked = this.flushAsked;
        this.flushAsked = undefined;
        if (asked !== undefined) {
          this.flush(asked.positionMs, asked.isLeaving);
        }
      },
      (cause: unknown) => {
        this.isSending = false;
        if (cause instanceof ReportRefused) {
          this.isRefused = true;
          return;
        }
        // Kept to go again as it was, under its own number: the server may have taken it.
        this.unsent = report;
        this.flushAsked = undefined;
      },
    );
  }

  /** The frames shown and dropped since the last report sent, absent where none are counted. */
  private framesSinceSent(): Pick<PlayReport, "totalFrames" | "droppedFrames"> {
    const now = this.readFrames();
    if (now === undefined) {
      return {};
    }
    const since = framesBetween(this.framesSent, now);
    this.framesSent = now;
    return { totalFrames: since.total, droppedFrames: since.dropped };
  }

  /**
   * The link's speed, with its samples' spread, and the player's steadiness level as read now, each
   * absent where unsaid.
   */
  private linkNow(): Pick<PlayReport, "linkKbps" | "linkSpread" | "steadiness"> {
    const linkKbps = this.readLinkKbps();
    const linkSpread = linkKbps === undefined ? undefined : this.readLinkSpread();
    const steadiness = this.readSteadiness();
    return {
      ...(linkKbps === undefined ? {} : { linkKbps: Math.round(linkKbps) }),
      ...(linkSpread === undefined ? {} : { linkSpread: Math.round(linkSpread * 1000) / 1000 }),
      ...(steadiness === undefined ? {} : { steadiness }),
    };
  }

  private recordEvent(event: {
    readonly kind: PlayEventKind;
    readonly positionMs: number;
    readonly happenedMonoMs: number;
    readonly context?: PlayEventContext | undefined;
  }): void {
    const { kind, positionMs, happenedMonoMs, context } = event;
    this.events.push({
      kind,
      positionMs: Math.max(0, Math.round(positionMs)),
      happenedMonoMs,
      ...(context === undefined ? {} : { context }),
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
export function seekTo(video: Pick<HTMLVideoElement, "currentTime">, positionMs: number): void {
  // eslint-disable-next-line no-param-reassign -- a media element seeks when currentTime is set
  video.currentTime = positionMs / 1000;
}
