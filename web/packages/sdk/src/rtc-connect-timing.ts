// SPDX-License-Identifier: MPL-2.0

/**
 * How long each step of making a WebRTC connection took (rtc-connect.ts makes them), and what is
 * reported of one that was slow (D484 §1).
 */

/**
 * A connection that takes longer than this to connect is reported to the core's log, with where its
 * time went: a sign-in that waited half a minute on its connections said so nowhere (staging,
 * 2026-10-23).
 */
export const slowConnectMs = 3_000;

/** How long each step of making a connection took, in milliseconds. */
export interface ConnectTimings {
  readonly gathering: number;
  readonly answer: number;
  readonly connecting: number;
  /** How connecting's time went, as far as the browser lets the page see it (D484 §1). */
  readonly steps?: ConnectingSteps | undefined;
}

/**
 * How a connection's connecting, from the core's answer to its verdict, went, in milliseconds, as far
 * as the browser lets the page see it (D484 §1): a connection that took 6.5 s said only that 5.4 s of
 * it went connecting, which could have been ICE's pacing, DTLS's retransmits or the verdict's wait
 * (the slow-link war game, 2026-11-14).
 */
export interface ConnectingSteps {
  /**
   * ICE's checks, until a pair answered (its ICE connection state connected or completed); undefined
   * where the browser said so only with the connection's state, its time then in secure.
   */
  readonly ice: number | undefined;
  /** The secure connection (DTLS) over that pair, until the connection's state was connected. */
  readonly secure: number;
  /** SCTP's association and the negotiation channel's own opening, until that channel opened. */
  readonly channel: number;
  /** The core's verdict on the negotiation channel (verdictOf), until it said the connection held. */
  readonly verdict: number;
}

/**
 * When, on one clock, each step of connecting ended: the core's answer came, ICE had a pair (none
 * where the browser did not say), the connection was connected, its negotiation channel opened (none
 * where it was not seen), and the core held it.
 */
export interface ConnectingMarks {
  readonly answeredAt: number;
  readonly iceAt: number | undefined;
  readonly connectedAt: number;
  readonly channelAt: number | undefined;
  readonly heldAt: number;
}

/**
 * How long each step of connecting took, from when each ended: a step whose end was not seen, or
 * was heard out of order (a channel's open told before the connection's state), takes none, its time
 * in the step after it.
 */
export function connectingStepsOf(marks: ConnectingMarks): ConnectingSteps {
  const { answeredAt, connectedAt, heldAt } = marks;
  const iceAt =
    marks.iceAt === undefined
      ? undefined
      : Math.min(Math.max(marks.iceAt, answeredAt), connectedAt);
  const secureFrom = iceAt ?? answeredAt;
  const channelAt = Math.min(Math.max(marks.channelAt ?? connectedAt, connectedAt), heldAt);
  return {
    ice: iceAt === undefined ? undefined : iceAt - answeredAt,
    secure: Math.max(0, connectedAt - secureFrom),
    channel: channelAt - Math.min(connectedAt, channelAt),
    verdict: Math.max(0, heldAt - channelAt),
  };
}

/** What to report of a connection that took over slowConnectMs, or undefined for one that did not. */
export function slowConnection(timings: ConnectTimings): string | undefined {
  const total = timings.gathering + timings.answer + timings.connecting;
  if (total <= slowConnectMs) {
    return undefined;
  }
  const ms = (value: number): string => `${String(Math.round(value))} ms`;
  const { steps } = timings;
  const how =
    steps === undefined
      ? ""
      : ` (${
          steps.ice === undefined
            ? `ICE and DTLS ${ms(steps.secure)}`
            : `ICE ${ms(steps.ice)}, DTLS ${ms(steps.secure)}`
        }, SCTP and the channel ${ms(steps.channel)}, the core's verdict ${ms(steps.verdict)})`;
  return (
    `a WebRTC connection took ${ms(total)}: gathering its candidates ${ms(timings.gathering)}, ` +
    `the core's answer ${ms(timings.answer)}, connecting ${ms(timings.connecting)}${how}`
  );
}
