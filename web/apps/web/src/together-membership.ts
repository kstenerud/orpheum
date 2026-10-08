// SPDX-License-Identifier: MPL-2.0

import { useMemo, useSyncExternalStore } from "react";

import type { StringStorage } from "./browser-storage.ts";
import {
  membershipText,
  readMembership,
  subscribeToMemberships,
  type Membership,
  type MembershipRole,
} from "./together-storage.ts";

/**
 * The membership kept in storage for role, re-rendering as one is kept or forgotten anywhere in the
 * tab: had back from the server after the page drew (D267 §2), or started, ended or left here.
 */
export function useMembership(
  storage: () => StringStorage,
  role: MembershipRole,
): Membership | undefined {
  const text = useSyncExternalStore(
    subscribeToMemberships,
    () => membershipText(storage(), role),
    () => membershipText(storage(), role),
  );
  // Read again only when the stored text changes, so the membership is one object while it lasts.
  return useMemo(
    () => (text === null ? undefined : readMembership(storage(), role)),
    [text, role, storage],
  );
}
