// SPDX-License-Identifier: MPL-2.0

/**
 * What a WebRTC offer's candidates are, told by kind and family and never by address, the errors
 * its gathering met, and which of its ICE servers relay (rtc-connect.ts makes the offers).
 */

/** Whether a configuration names a TURN server, through which a relayed candidate will come. */
export function hasRelayServer(rtcConfig: RTCConfiguration): boolean {
  return (rtcConfig.iceServers ?? []).some((server) =>
    (Array.isArray(server.urls) ? server.urls : [server.urls]).some((url) => /^turns?:/i.test(url)),
  );
}

/**
 * rtcConfig with every TURN server's URL taken out, and a server left with none dropped: what a
 * connection asks first, its STUN servers alone (D559 §1).
 */
export function withoutRelayServers(rtcConfig: RTCConfiguration): RTCConfiguration {
  const iceServers = (rtcConfig.iceServers ?? []).flatMap((server) => {
    const urls = (Array.isArray(server.urls) ? server.urls : [server.urls]).filter(
      (url) => !/^turns?:/i.test(url),
    );
    return urls.length === 0 ? [] : [{ ...server, urls }];
  });
  return { ...rtcConfig, iceServers };
}

/** One candidate an offer holds, by its kinds alone: never its address. */
interface CandidateKind {
  readonly type: string;
  /** "mDNS" for a name ending .local, which the core drops; "IPv4" or "IPv6" for an address. */
  readonly family: "mDNS" | "IPv4" | "IPv6";
}

/** The kinds of each candidate an offer's SDP holds (its a=candidate lines). */
function candidateKindsOf(sdp: string): CandidateKind[] {
  const kinds: CandidateKind[] = [];
  for (const line of sdp.split(/\r?\n/)) {
    // a=candidate:<foundation> <component> <transport> <priority> <address> <port> typ <type> ...
    const fields = /^a=candidate:\S+ \S+ \S+ \S+ (\S+) \S+ typ (\S+)/.exec(line.trim());
    if (fields === null) {
      continue;
    }
    const [, address = "", type = "unknown"] = fields;
    kinds.push({ type, family: familyOf(address) });
  }
  return kinds;
}

/** The family of a candidate's address: an mDNS name (ending .local), IPv6 or IPv4. */
function familyOf(address: string): CandidateKind["family"] {
  if (/\.local\.?$/i.test(address)) {
    return "mDNS";
  }
  return address.includes(":") ? "IPv6" : "IPv4";
}

/**
 * Whether an offer's SDP holds a candidate the core can reach this device by: a server-reflexive or
 * relayed one, or a host one with an address, not an mDNS name (the core drops those, rtcapi's
 * checkRelayedOffer).
 */
export function isPastThisDevice(sdp: string): boolean {
  return candidateKindsOf(sdp).some(({ type, family }) => type !== "host" || family !== "mDNS");
}

/**
 * What an offer's SDP holds, by kind and family, never an address or a name, with the errors its
 * gathering met: "2 host (mDNS), 1 srflx IPv4", or "2 host (mDNS), no candidate past this device;
 * gathering errors: turn 486".
 */
export function gatheringText(sdp: string, errors: readonly string[] = []): string {
  const counts = new Map<string, number>();
  for (const { type, family } of candidateKindsOf(sdp)) {
    const kind = family === "mDNS" ? `${type} (mDNS)` : `${type} ${family}`;
    counts.set(kind, (counts.get(kind) ?? 0) + 1);
  }
  const held = Array.from(counts, ([kind, count]) => `${String(count)} ${kind}`);
  if (!isPastThisDevice(sdp)) {
    held.push("no candidate past this device");
  }
  const text = held.join(", ");
  return errors.length === 0 ? text : `${text}; ${errorsText(errors)}`;
}

/** Gathering's errors in words: "gathering errors: turn 486, stun 701". */
export function errorsText(errors: readonly string[]): string {
  return `gathering errors: ${errors.join(", ")}`;
}

/** What gatheringErrorsOf needs of a peer connection: the browser's, or a test's. */
export interface ErringPeer {
  addEventListener(
    type: "icecandidateerror",
    listener: (event: RTCPeerConnectionIceErrorEvent) => void,
  ): void;
}

/**
 * The errors peer's gathering meets, each told once as its server's scheme and its code ("turn 486",
 * a TURN server's refusal; "stun 701", a STUN server not reached), never the server's address or its
 * credentials, nor this device's address: why a kind of candidate is missing.
 */
export function gatheringErrorsOf(peer: ErringPeer): () => readonly string[] {
  const errors = new Set<string>();
  peer.addEventListener("icecandidateerror", (event) => {
    const scheme = /^(stuns?|turns?):/i.exec(event.url)?.[1]?.toLowerCase() ?? "unknown";
    errors.add(`${scheme} ${String(event.errorCode)}`);
  });
  return () => Array.from(errors);
}
