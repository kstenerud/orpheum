// SPDX-License-Identifier: MPL-2.0

/**
 * A WebRTC connection's candidate pairs as the browser sees them (RTCPeerConnection.getStats), for
 * the core's log: the selected one, and whether the pairs to the core's host candidates, its LAN
 * address's among them, were tried and failed. That one line would have said "the LAN host pair
 * failed: the UDP port is unreachable" where a day's digging did (local-path-2026-10-27.md §4 item 2).
 */

/** One candidate pair: its state, each end by type, protocol and address, and whether it is selected. */
export interface CandidatePairSeen {
  readonly state: string;
  /** The browser's end, its address as the browser gives it: an mDNS name for a host it hides. */
  readonly browser: string;
  /** The core's end. */
  readonly core: string;
  readonly isSelected?: boolean;
}

/** The most pairs reported, and the longest each end's and a state's words are: the API's bounds. */
export const pairsReportedMost = 16;
const endMost = 200;
const stateMost = 20;

/** A stats entry's field, as a string, or "" where it has none. */
function field(entry: object, name: string): string {
  const value: unknown = Reflect.get(entry, name);
  return typeof value === "string" || typeof value === "number" ? String(value) : "";
}

/** A candidate's words: its type, protocol and address. */
function endOf(candidate: object | undefined): string {
  if (candidate === undefined) {
    return "unknown";
  }
  const named = field(candidate, "address");
  const address = named === "" ? field(candidate, "ip") : named;
  const words = `${field(candidate, "candidateType")} ${field(candidate, "protocol")} ${address}:${field(candidate, "port")}`;
  return words.slice(0, endMost);
}

/**
 * The pairs stats holds, the selected one first, then those to the core's host candidates, then the
 * rest, at most pairsReportedMost. The selected one is the transport's (Chrome, Safari), the pair
 * marked selected (Firefox), or else a nominated pair that succeeded.
 */
export function candidatePairsOf(stats: Iterable<unknown>): CandidatePairSeen[] {
  const byId = new Map<string, object>();
  const pairs: object[] = [];
  const selectedIds = new Set<string>();
  for (const entry of stats) {
    if (typeof entry !== "object" || entry === null) {
      continue;
    }
    byId.set(field(entry, "id"), entry);
    const type = field(entry, "type");
    if (type === "candidate-pair") {
      pairs.push(entry);
      if (Reflect.get(entry, "selected") === true) {
        selectedIds.add(field(entry, "id"));
      }
    } else if (type === "transport" && field(entry, "selectedCandidatePairId") !== "") {
      selectedIds.add(field(entry, "selectedCandidatePairId"));
    }
  }
  if (selectedIds.size === 0) {
    const nominated = pairs.find(
      (pair) => Reflect.get(pair, "nominated") === true && field(pair, "state") === "succeeded",
    );
    if (nominated !== undefined) {
      selectedIds.add(field(nominated, "id"));
    }
  }
  const rank = (pair: object): number => {
    if (selectedIds.has(field(pair, "id"))) {
      return 0;
    }
    const remote = byId.get(field(pair, "remoteCandidateId"));
    return remote !== undefined && field(remote, "candidateType") === "host" ? 1 : 2;
  };
  return pairs
    .map((pair, index) => ({ pair, index, rank: rank(pair) }))
    .sort((one, other) =>
      one.rank === other.rank ? one.index - other.index : one.rank - other.rank,
    )
    .slice(0, pairsReportedMost)
    .map(({ pair, rank: placed }) => ({
      state: field(pair, "state").slice(0, stateMost),
      browser: endOf(byId.get(field(pair, "localCandidateId"))),
      core: endOf(byId.get(field(pair, "remoteCandidateId"))),
      ...(placed === 0 ? { isSelected: true } : {}),
    }));
}
