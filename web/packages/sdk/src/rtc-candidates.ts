// SPDX-License-Identifier: MPL-2.0

/**
 * What a WebRTC offer's candidates are, told by kind and family and never by address, the errors
 * its gathering met, and which of its ICE servers relay (rtc-connect.ts makes the offers).
 */

/** Whether url names a TURN server, through which a relayed candidate will come. */
function isRelayUrl(url: string): boolean {
  return /^turns?:/i.test(url);
}

/**
 * Whether url names a TURN server over UDP: a turn: URL that does not say transport=tcp, as one
 * that names no transport is UDP (RFC 7065). A turns: URL is TLS, or DTLS, never plain UDP.
 */
function isUdpRelayUrl(url: string): boolean {
  return /^turn:/i.test(url) && !/[?&]transport=tcp(?:&|$)/i.test(url);
}

/** Every ICE server URL a configuration names. */
function urlsOf(rtcConfig: RTCConfiguration): string[] {
  return (rtcConfig.iceServers ?? []).flatMap((server) =>
    Array.isArray(server.urls) ? server.urls : [server.urls],
  );
}

/** rtcConfig with only the URLs keep keeps, and a server left with none dropped. */
function withUrlsKept(
  rtcConfig: RTCConfiguration,
  keep: (url: string) => boolean,
): RTCConfiguration {
  const iceServers = (rtcConfig.iceServers ?? []).flatMap((server) => {
    const urls = (Array.isArray(server.urls) ? server.urls : [server.urls]).filter(keep);
    return urls.length === 0 ? [] : [{ ...server, urls }];
  });
  return { ...rtcConfig, iceServers };
}

/** Whether a configuration names a TURN server, through which a relayed candidate will come. */
export function hasRelayServer(rtcConfig: RTCConfiguration): boolean {
  return urlsOf(rtcConfig).some(isRelayUrl);
}

/** Whether a configuration names a TURN server over UDP (isUdpRelayUrl). */
export function hasUdpRelayServer(rtcConfig: RTCConfiguration): boolean {
  return urlsOf(rtcConfig).some(isUdpRelayUrl);
}

/** Whether a configuration names a TURN server by a URL that is not UDP: a turns: one, or transport=tcp. */
export function hasRelayBesidesUdp(rtcConfig: RTCConfiguration): boolean {
  return urlsOf(rtcConfig).some((url) => isRelayUrl(url) && !isUdpRelayUrl(url));
}

/**
 * rtcConfig with every TURN server's URL taken out, and a server left with none dropped: what a
 * connection asks first, its STUN servers alone (D559 §1).
 */
export function withoutRelayServers(rtcConfig: RTCConfiguration): RTCConfiguration {
  return withUrlsKept(rtcConfig, (url) => !isRelayUrl(url));
}

/**
 * rtcConfig with every TURN URL but the UDP ones taken out, its STUN URLs kept, and a server left
 * with none dropped: what a connection asks once it has not connected without a relay, before the
 * relay's TLS address (D561 §2). A browser allocates at each relay URL for each of its addresses, and
 * the TLS allocations of the hosted relay carried nothing while they filled its limit.
 */
export function withUdpRelaysAlone(rtcConfig: RTCConfiguration): RTCConfiguration {
  return withUrlsKept(rtcConfig, (url) => !isRelayUrl(url) || isUdpRelayUrl(url));
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
