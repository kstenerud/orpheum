// SPDX-License-Identifier: MPL-2.0

import type { Api } from "@orpheum/sdk";
import { useSyncExternalStore } from "react";

import { browserStorage, tabStorage } from "./browser-storage.ts";
import { idempotent } from "./idempotent.ts";
import { ApiProblem, dataOrThrow } from "./problem.ts";
import { useMembership } from "./together-membership.ts";
import { forgetMembership, keepMembership, readMembership } from "./together-storage.ts";

/**
 * The session the person hosts on another device, as this tab hosts none: its id, and the name of the
 * device it is on, where the server knows one (D442 §1).
 */
export interface HostedElsewhere {
  readonly sessionId: number;
  readonly deviceName: string | undefined;
}

let hostedElsewhere: HostedElsewhere | undefined;
const elsewhereListeners = new Set<() => void>();

/** Notes the session the person hosts on another device, or that there is none. */
export function noteHostedElsewhere(next: HostedElsewhere | undefined): void {
  if (
    next?.sessionId === hostedElsewhere?.sessionId &&
    next?.deviceName === hostedElsewhere?.deviceName
  ) {
    return;
  }
  hostedElsewhere = next;
  for (const listener of elsewhereListeners) {
    listener();
  }
}

function subscribeToElsewhere(listener: () => void): () => void {
  elsewhereListeners.add(listener);
  return () => elsewhereListeners.delete(listener);
}

/** The session the person hosts on another device, as last learned. */
export function readHostedElsewhere(): HostedElsewhere | undefined {
  return hostedElsewhere;
}

/** The session the person hosts on another device, re-rendering as it is learned or goes. */
export function useHostedElsewhere(): HostedElsewhere | undefined {
  return useSyncExternalStore(subscribeToElsewhere, readHostedElsewhere, readHostedElsewhere);
}

/**
 * The session the person hosts on another device, as useHostedElsewhere has it, while this tab
 * neither hosts a session nor is a guest in one: what Watch Together then says, and offers to move
 * here (D442 §2).
 */
export function useHostedElsewhereHere(): HostedElsewhere | undefined {
  const elsewhere = useHostedElsewhere();
  const isHosting = useMembership(tabStorage, "host") !== undefined;
  const isViewing = useMembership(browserStorage, "viewer") !== undefined;
  return isHosting || isViewing ? undefined : elsewhere;
}

/**
 * Moves the session the person hosts on another device to this tab, which hosts it from then on, the
 * other device told it moved (D442 §2), and says whether it did. One the server no longer has, or
 * that is no longer theirs, is forgotten, so a new one may start.
 */
export async function moveHostingHere(
  api: Api,
  sessionId: number,
  idempotencyKey: string,
): Promise<boolean> {
  try {
    const moved = dataOrThrow(
      await api.POST("/together/{id}/host-here", {
        params: { path: { id: sessionId } },
        ...idempotent(idempotencyKey),
      }),
    );
    keepMembership(tabStorage(), "host", {
      sessionId: moved.sessionId,
      credential: moved.membershipCredential,
    });
    noteHostedElsewhere(undefined);
    return true;
  } catch (cause) {
    if (cause instanceof ApiProblem && (cause.code === "not_found" || cause.code === "forbidden")) {
      noteHostedElsewhere(undefined);
      return false;
    }
    throw cause;
  }
}

/**
 * Forgets the session this tab hosts when it is still the one hosted as the server was asked,
 * askedId, and the server's sessions do not include it as this person's to host.
 */
function forgetHostedIfGone(
  askedId: number | undefined,
  sessions: readonly { readonly sessionId: number; readonly isHost: boolean }[],
): void {
  const hosted = readMembership(tabStorage(), "host");
  if (hosted === undefined || hosted.sessionId !== askedId) {
    return;
  }
  if (!sessions.some((each) => each.isHost && each.sessionId === hosted.sessionId)) {
    forgetMembership(tabStorage(), "host");
  }
}

/**
 * Has back, on this device, the Watch Together sessions the signed-in person is in. One they joined
 * signed in follows them, for the viewer's page (D267 §2); a membership this browser keeps for it is
 * left as it is, and each device has a credential of its own, so asking cuts off none of their others
 * (§3). One they host stays on the device that started it, surviving a reload there in the tab's
 * storage (D442 §1): another device keeps no membership for it, and notes it as hosted elsewhere, on
 * the device the server names, for Watch Together to say so and offer to move it here; one they host
 * and joined as a guest from another device is not had back as a guest's either. A session this tab
 * hosts that the server no longer has, ended or lost with a restart, is forgotten: its sign stayed in
 * the status tray, a session gone (the owner's Android, 2026-10-11).
 */
export async function haveBackMySessions(api: Api): Promise<void> {
  // The session hosted as the server was asked: one started while it answered is not forgotten.
  const asked = readMembership(tabStorage(), "host");
  const { sessions } = dataOrThrow(await api.GET("/me/together"));
  const listed = sessions ?? [];
  forgetHostedIfGone(asked?.sessionId, listed);
  const hosted = listed.find((each) => each.isHost);
  noteHostedElsewhere(
    hosted === undefined || readMembership(tabStorage(), "host") !== undefined
      ? undefined
      : { sessionId: hosted.sessionId, deviceName: hosted.hostDeviceName },
  );
  for (const each of listed) {
    // A session the person hosts is not had back as a guest's, though another device of theirs
    // joined it signed in: its sign stayed on, leading to "This session is over", once the host
    // ended it (the owner's Chrome, 2026-10-29).
    if (!each.isHost && each.sessionId !== hosted?.sessionId) {
      keepUnlessKept(each);
    }
  }
}

/** Keeps a joined session's membership, unless this browser keeps one for it already. */
function keepUnlessKept(each: {
  readonly sessionId: number;
  readonly membershipCredential?: string | undefined;
  readonly locator?: string | undefined;
}): void {
  const storage = browserStorage();
  if (
    each.membershipCredential !== undefined &&
    readMembership(storage, "viewer")?.sessionId !== each.sessionId
  ) {
    keepMembership(storage, "viewer", {
      sessionId: each.sessionId,
      credential: each.membershipCredential,
      locator: each.locator,
    });
  }
}
