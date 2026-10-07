// SPDX-License-Identifier: MPL-2.0

/**
 * The Opus settings voice asks its partner to send with (D262 §3): in-band FEC, which carries a
 * lower copy of the frame before in each packet, on; DTX, sending almost nothing in silence, on, as
 * Signal's calls do; and an average of 40 kbit/s, room for FEC beside the voice. A browser sends as
 * its partner's description asks, so each side asks it of the other in its own.
 */
export const opusVoiceParameters: Readonly<Record<string, string>> = {
  useinbandfec: "1",
  usedtx: "1",
  maxaveragebitrate: "40000",
};

/** The payload type a description gives Opus, or undefined for one without it. */
function opusPayloadType(lines: readonly string[]): string | undefined {
  for (const line of lines) {
    const found = /^a=rtpmap:(\d+) opus\/48000/iu.exec(line);
    if (found !== null) {
      return found[1];
    }
  }
  return undefined;
}

/** A format line's parameters, after its prefix, by name, in order. */
function parametersOf(line: string | undefined, prefix: string): Map<string, string> {
  const parameters = new Map<string, string>();
  for (const pair of (line ?? prefix).slice(prefix.length).split(";")) {
    const [key = "", value = ""] = pair.trim().split("=");
    if (key !== "") {
      parameters.set(key, value);
    }
  }
  return parameters;
}

/**
 * A session description with Opus's format parameters set to opusVoiceParameters, the rest of each
 * line kept; one with no Opus is given back as it is.
 */
export function withOpusSettings(sdp: string): string {
  const lineEnd = sdp.includes("\r\n") ? "\r\n" : "\n";
  const lines = sdp.split(lineEnd);
  const payloadType = opusPayloadType(lines);
  if (payloadType === undefined) {
    return sdp;
  }
  const prefix = `a=fmtp:${payloadType} `;
  const at = lines.findIndex((line) => line.startsWith(prefix));
  const parameters = parametersOf(lines[at], prefix);
  for (const [key, value] of Object.entries(opusVoiceParameters)) {
    parameters.set(key, value);
  }
  const line = prefix + [...parameters].map(([key, value]) => `${key}=${value}`).join(";");
  if (at === -1) {
    lines.splice(
      lines.findIndex((each) => each.startsWith(`a=rtpmap:${payloadType} `)) + 1,
      0,
      line,
    );
  } else {
    lines[at] = line;
  }
  return lines.join(lineEnd);
}

/** A codec as the browser lists what it can send and receive. */
export interface ListedCodec {
  readonly mimeType: string;
}

/**
 * The browser's audio codecs in the order voice prefers them (D262 §3): RED first, each packet
 * carrying the frame before as well, which Chrome and Safari use only when it leads; then Opus;
 * then the rest as the browser listed them. Firefox lists no RED, and plain Opus is negotiated.
 */
export function voiceCodecOrder<Codec extends ListedCodec>(codecs: readonly Codec[]): Codec[] {
  const rank = (codec: Codec): number => {
    const mimeType = codec.mimeType.toLowerCase();
    if (mimeType === "audio/red") {
      return 0;
    }
    return mimeType === "audio/opus" ? 1 : 2;
  };
  return [...codecs].sort((a, b) => rank(a) - rank(b));
}
