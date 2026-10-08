// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { voiceCallStatsOf } from "./together-voice-stats.ts";

describe("a voice call's statistics (D262 §1)", () => {
  it("are read from the audio's reports and the chosen pair's local candidate", () => {
    const stats = voiceCallStatsOf(
      [
        {
          type: "inbound-rtp",
          kind: "audio",
          jitterBufferDelay: 90,
          jitterBufferTargetDelay: 88,
          jitterBufferEmittedCount: 100,
          totalSamplesReceived: 10_000,
          concealedSamples: 600,
          silentConcealedSamples: 170,
          packetsLost: 31,
          packetsDiscarded: 9,
          fecPacketsReceived: 4,
          totalAudioEnergy: 0.32,
          totalSamplesDuration: 20,
        },
        { type: "media-source", kind: "audio", totalAudioEnergy: 0.02, totalSamplesDuration: 20 },
        { type: "remote-inbound-rtp", kind: "audio", roundTripTime: 0.34 },
        { type: "candidate-pair", nominated: true, state: "succeeded", localCandidateId: "L1" },
        { type: "local-candidate", id: "L1", candidateType: "relay", relayProtocol: "tcp" },
      ],
      "elementInterrupted",
    );
    expect(stats).toEqual({
      roundTripMs: 340,
      jitterBufferMs: 900,
      jitterBufferTargetMs: 880,
      concealedShare: 0.043,
      packetsLost: 31,
      packetsLate: 9,
      fecPackets: 4,
      path: "tcp",
      receivedLevel: 0.126,
      sentLevel: 0.032,
      playback: "elementInterrupted",
    });
  });

  it("say a direct path, and nothing measured, before the call carries sound", () => {
    expect(voiceCallStatsOf([], "webAudio")).toEqual({
      roundTripMs: 0,
      jitterBufferMs: 0,
      jitterBufferTargetMs: 0,
      concealedShare: 0,
      packetsLost: 0,
      packetsLate: 0,
      fecPackets: 0,
      path: "direct",
      receivedLevel: 0,
      sentLevel: 0,
      playback: "webAudio",
    });
  });
});
