// SPDX-License-Identifier: MPL-2.0

/**
 * The most candidates an offer through a discovery may list: the discovery refuses more, and so does
 * the core (internal/core/rtcapi.MaxRelayedCandidates; the protocol's §2.5, S16).
 */
export const relayedCandidatesMost = 10;

/** The candidate types an offer keeps first when it has too many: relayed paths reach any core. */
const typesKeptFirst = ["relay", "srflx", "prflx", "host"];

/** One `a=candidate:` line's fields that decide whether it is kept. */
interface Candidate {
  readonly line: string;
  readonly address: string;
  readonly protocol: string;
  readonly type: string;
}

/** The candidate a line holds, or undefined for any other line. */
function candidateOf(line: string): Candidate | undefined {
  if (!line.startsWith("a=candidate:")) {
    return undefined;
  }
  const fields = line.split(" ");
  const typeAt = fields.indexOf("typ");
  return {
    line,
    address: fields[4] ?? "",
    protocol: (fields[2] ?? "").toLowerCase(),
    type: typeAt < 0 ? "" : (fields[typeAt + 1] ?? ""),
  };
}

/** The IPv4 address's four parts, or undefined if text is not one. */
function ipv4Parts(text: string): readonly number[] | undefined {
  const parts = text.split(".").map(Number);
  return parts.length === 4 &&
    parts.every((part) => Number.isInteger(part) && part >= 0 && part <= 255)
    ? parts
    : undefined;
}

/**
 * The IPv4 ranges only their own network reaches, as [first part, second part's lowest, highest]:
 * this network, private, loopback, carrier-grade NAT and link-local.
 */
const privateIPv4: readonly (readonly [number, number, number])[] = [
  [0, 0, 255],
  [10, 0, 255],
  [127, 0, 255],
  [100, 64, 127],
  [169, 254, 254],
  [172, 16, 31],
  [192, 168, 168],
];

/** Whether an IPv4 address is one only its own network reaches. */
function isPrivateIPv4([first = 0, second = 0]: readonly number[]): boolean {
  return privateIPv4.some(
    ([part, lowest, highest]) => first === part && second >= lowest && second <= highest,
  );
}

/** Whether an IPv6 address is one only its own network reaches: loopback, unspecified, link-local, ULA. */
function isPrivateIPv6(text: string): boolean {
  const lower = text.toLowerCase();
  const mapped = /^::ffff:(\d+\.\d+\.\d+\.\d+)$/u.exec(lower)?.[1];
  if (mapped !== undefined) {
    const parts = ipv4Parts(mapped);
    return parts === undefined || isPrivateIPv4(parts);
  }
  return lower === "::" || lower === "::1" || /^fe[89ab]/u.test(lower) || /^f[cd]/u.test(lower);
}

/**
 * Whether a candidate's address is one a core outside the client's network can reach: an mDNS name
 * resolves only on the client's own LAN, and the core never checks a private address it was offered
 * through a discovery (internal/core/rtcapi's relayed offers).
 */
function isReachableFromOutside(address: string): boolean {
  const parts = ipv4Parts(address);
  if (parts !== undefined) {
    return !isPrivateIPv4(parts);
  }
  return address.includes(":") && !isPrivateIPv6(address);
}

/** The order an offer keeps candidates in when it has too many: UDP first, then by typesKeptFirst. */
function keptBefore(one: Candidate, other: Candidate): number {
  const rank = (candidate: Candidate): number => {
    const typeRank = typesKeptFirst.indexOf(candidate.type);
    return (
      (candidate.protocol === "udp" ? 0 : typesKeptFirst.length) +
      (typeRank < 0 ? typesKeptFirst.length : typeRank)
    );
  };
  return rank(one) - rank(other);
}

/**
 * The offer's SDP as it goes through a discovery: without the candidates no core outside the client's
 * network could use, and with at most relayedCandidatesMost of the rest, relayed and UDP ones kept
 * first. A phone on Wi-Fi lists more than that, and the discovery would refuse the whole offer.
 */
export function offerThroughDiscovery(sdp: string): string {
  const lines = sdp.replaceAll("\r\n", "\n").split("\n");
  const usable = lines
    .map(candidateOf)
    .filter(
      (candidate): candidate is Candidate =>
        candidate !== undefined && isReachableFromOutside(candidate.address),
    );
  const kept = new Set(
    [...usable]
      .sort(keptBefore)
      .slice(0, relayedCandidatesMost)
      .map((candidate) => candidate.line),
  );
  return lines.filter((line) => candidateOf(line) === undefined || kept.has(line)).join("\r\n");
}
