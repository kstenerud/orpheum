// SPDX-License-Identifier: MPL-2.0

import type { AudioOutput } from "./accounts.ts";
import type { StringStorage } from "./browser-storage.ts";
import { audioOutputs } from "./sound.tsx";

/**
 * A Watch Together membership as it is kept: its session, the credential it acts by, and, for a
 * guest's, the link it was joined by, so the same link opened again in this browser comes back as
 * the same member (D267 §1).
 */
export interface Membership {
  readonly sessionId: number;
  readonly credential: string;
  readonly link?: string;
}

/**
 * Whose membership: the host's, for the session they started (D246 §5), kept in the tab, which has
 * it back from the server on any device they sign in on (D267 §2); or a viewer's, from their link
 * (D246 §3), kept in the browser, so closing the page does not lose it (D267 §1).
 */
export type MembershipRole = "host" | "viewer";

const membershipKeys: Readonly<Record<MembershipRole, string>> = {
  host: "orpheum.together.host",
  viewer: "orpheum.together.viewer",
};

/** What was kept, when it is a membership this tab wrote. */
function asMembership(kept: unknown): Membership | undefined {
  if (typeof kept !== "object" || kept === null) {
    return undefined;
  }
  const sessionId: unknown = Reflect.get(kept, "sessionId");
  const credential: unknown = Reflect.get(kept, "credential");
  const link: unknown = Reflect.get(kept, "link");
  if (typeof sessionId !== "number" || typeof credential !== "string" || credential === "") {
    return undefined;
  }
  return typeof link === "string" ? { sessionId, credential, link } : { sessionId, credential };
}

/** The membership this tab keeps for role, or undefined when it keeps none. */
export function readMembership(
  storage: StringStorage,
  role: MembershipRole,
): Membership | undefined {
  const kept = storage.getItem(membershipKeys[role]);
  try {
    return kept === null ? undefined : asMembership(JSON.parse(kept));
  } catch {
    return undefined; // not what this tab wrote
  }
}

/** Who is told when this tab keeps or forgets a membership: storage itself tells no one in the tab. */
const membershipListeners = new Set<() => void>();

/** Calls listener whenever this tab keeps or forgets a membership; returns the unsubscribe. */
export function subscribeToMemberships(listener: () => void): () => void {
  membershipListeners.add(listener);
  return () => {
    membershipListeners.delete(listener);
  };
}

function tellMembershipChanged(): void {
  for (const listener of membershipListeners) {
    listener();
  }
}

/** Keeps a membership for role in this tab, across a reload. */
export function keepMembership(
  storage: StringStorage,
  role: MembershipRole,
  membership: Membership,
): void {
  storage.setItem(
    membershipKeys[role],
    JSON.stringify({
      sessionId: membership.sessionId,
      credential: membership.credential,
      link: membership.link,
    }),
  );
  tellMembershipChanged();
}

/** The membership kept for role, as its stored text: what changes when it does, for a snapshot. */
export function membershipText(storage: StringStorage, role: MembershipRole): string | null {
  return storage.getItem(membershipKeys[role]);
}

/** Forgets role's membership: the session ended, or the person left it. */
export function forgetMembership(storage: StringStorage, role: MembershipRole): void {
  storage.removeItem(membershipKeys[role]);
  tellMembershipChanged();
}

const viewerAudioOutputKey = "orpheum.together.audioOutput";

/**
 * How a viewer's device makes sound, as they answered when they first joined (D246 §6), kept in
 * this browser: a viewer has no device row on the server to keep it in.
 */
export function readViewerAudioOutput(storage: StringStorage): AudioOutput | undefined {
  const kept = storage.getItem(viewerAudioOutputKey);
  return audioOutputs.find((output) => output === kept);
}

/** Keeps a viewer's answer to how their device makes sound, for every session they join here. */
export function keepViewerAudioOutput(storage: StringStorage, output: AudioOutput): void {
  storage.setItem(viewerAudioOutputKey, output);
}

/**
 * The header a member's request carries: their membership credential, in place of a session
 * token, never in the address (D154).
 */
export function membershipHeaders(credential: string): { readonly authorization: string } {
  return { authorization: `Bearer ${credential}` };
}
