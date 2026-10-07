// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { voiceCodecOrder, withOpusSettings } from "./together-voice-codec.ts";

const offer = [
  "v=0",
  "m=audio 9 UDP/TLS/RTP/SAVPF 111 63 9",
  "a=rtpmap:111 opus/48000/2",
  "a=rtcp-fb:111 transport-cc",
  "a=fmtp:111 minptime=10;useinbandfec=1",
  "a=rtpmap:63 red/48000/2",
  "a=fmtp:63 111/111",
  "a=rtpmap:9 G722/8000",
  "",
].join("\r\n");

describe("voice's Opus settings (D262 §3)", () => {
  it("asks for FEC, DTX and 40 kbit/s in Opus's own line, keeping what it said", () => {
    const set = withOpusSettings(offer);
    expect(set).toContain(
      "a=fmtp:111 minptime=10;useinbandfec=1;usedtx=1;maxaveragebitrate=40000\r\n",
    );
    expect(set).toContain("a=fmtp:63 111/111\r\n");
    expect(set.split("\r\n")).toHaveLength(offer.split("\r\n").length);
  });

  it("adds Opus's line after its rtpmap where it had none, and leaves a description without Opus alone", () => {
    const bare = "m=audio 9 RTP/AVP 111\na=rtpmap:111 OPUS/48000/2\na=sendrecv\n";
    expect(withOpusSettings(bare)).toBe(
      "m=audio 9 RTP/AVP 111\na=rtpmap:111 OPUS/48000/2\na=fmtp:111 useinbandfec=1;usedtx=1;maxaveragebitrate=40000\na=sendrecv\n",
    );
    expect(withOpusSettings("m=audio 9 RTP/AVP 0\na=rtpmap:0 PCMU/8000\n")).toBe(
      "m=audio 9 RTP/AVP 0\na=rtpmap:0 PCMU/8000\n",
    );
  });
});

describe("voice's codec order (D262 §3)", () => {
  it("puts RED first and Opus next, the rest as listed", () => {
    const listed = [
      { mimeType: "audio/G722" },
      { mimeType: "audio/opus" },
      { mimeType: "audio/PCMU" },
      { mimeType: "audio/red" },
    ];
    expect(voiceCodecOrder(listed).map((codec) => codec.mimeType)).toEqual([
      "audio/red",
      "audio/opus",
      "audio/G722",
      "audio/PCMU",
    ]);
  });
});
