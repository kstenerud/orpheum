// SPDX-License-Identifier: MPL-2.0

/**
 * How a call's sound plays where it is received: through Web Audio, running, at the person's gain;
 * or through its audio element, Web Audio being suspended, interrupted (iOS, as the microphone
 * opens; WebKit 273511), closed, or absent ("element").
 */
export type VoicePlayback =
  | "webAudio"
  | "elementSuspended"
  | "elementInterrupted"
  | "elementClosed"
  | "element"
  /** Its element refused to play for want of a tap, Web Audio not running: no one hears it. */
  | "elementBlocked";

/**
 * A voice call as its receiver measured it since it began (D262 §1): how far its sound came (the
 * round trip), how long the jitter buffer held it and aimed to on average, the share of samples
 * concealed apart from silence, what a listener hears go missing, packets never received, packets
 * that came too late to play and packets FEC recovered, and its path: "direct", or the relay's
 * "udp", "tcp" or "tls" (D262 §4). And how loud, on average from 0 to 1, what it received and what
 * its microphone sent were, and how the sound plays: a call that carries packets of silence, or
 * sound nobody plays, says so (the owner's test, 2026-10-08, where an iPhone and an Android phone
 * connected and neither heard the other).
 */
export interface VoiceCallStats {
  readonly roundTripMs: number;
  readonly jitterBufferMs: number;
  readonly jitterBufferTargetMs: number;
  readonly concealedShare: number;
  readonly packetsLost: number;
  readonly packetsLate: number;
  readonly fecPackets: number;
  readonly path: "direct" | "udp" | "tcp" | "tls";
  readonly receivedLevel: number;
  readonly sentLevel: number;
  readonly playback: VoicePlayback;
}

/** How often a voice call's statistics go to the core's log, and how soon after it connects first. */
export const voiceStatsEveryMs = 10_000;
export const voiceFirstStatsAfterMs = 2000;

/** A report's number field, or 0 where it has none. */
function numberIn(report: Record<string, unknown> | undefined, key: string): number {
  const value = report?.[key];
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

/** The average level, 0 to 1, of the sound a report measured: the root of its mean energy. */
function levelIn(report: Record<string, unknown> | undefined): number {
  const seconds = numberIn(report, "totalSamplesDuration");
  const level = seconds > 0 ? Math.sqrt(numberIn(report, "totalAudioEnergy") / seconds) : 0;
  return Math.round(level * 1000) / 1000;
}

/**
 * A call's statistics from its RTCPeerConnection's reports: its audio's received stream and the
 * remote side's report on what it sent, the microphone's source, and the local candidate of the
 * pair it chose; and how its sound plays, which the reports do not say.
 */
export function voiceCallStatsOf(
  reports: readonly Record<string, unknown>[],
  playback: VoicePlayback,
): VoiceCallStats {
  const audio = (type: string): Record<string, unknown> | undefined =>
    reports.find((report) => report["type"] === type && report["kind"] === "audio");
  const inbound = audio("inbound-rtp");
  const emitted = numberIn(inbound, "jitterBufferEmittedCount");
  const received = numberIn(inbound, "totalSamplesReceived");
  const audible =
    numberIn(inbound, "concealedSamples") - numberIn(inbound, "silentConcealedSamples");
  const pair = reports.find(
    (report) =>
      report["type"] === "candidate-pair" &&
      report["nominated"] === true &&
      report["state"] === "succeeded",
  );
  const local = reports.find((report) => report["id"] === pair?.["localCandidateId"]);
  return {
    roundTripMs: Math.round(numberIn(audio("remote-inbound-rtp"), "roundTripTime") * 1000),
    jitterBufferMs:
      emitted > 0 ? Math.round((numberIn(inbound, "jitterBufferDelay") / emitted) * 1000) : 0,
    jitterBufferTargetMs:
      emitted > 0 ? Math.round((numberIn(inbound, "jitterBufferTargetDelay") / emitted) * 1000) : 0,
    concealedShare: received > 0 ? Math.max(0, audible) / received : 0,
    packetsLost: numberIn(inbound, "packetsLost"),
    packetsLate: numberIn(inbound, "packetsDiscarded"),
    fecPackets: numberIn(inbound, "fecPacketsReceived"),
    path: local?.["candidateType"] === "relay" ? relayPathOf(local["relayProtocol"]) : "direct",
    receivedLevel: levelIn(inbound),
    sentLevel: levelIn(audio("media-source")),
    playback,
  };
}

/** The relay's protocol, as a path; one the browser does not name is "udp", ICE's first. */
function relayPathOf(protocol: unknown): VoiceCallStats["path"] {
  return protocol === "tcp" || protocol === "tls" ? protocol : "udp";
}
