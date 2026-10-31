// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";

import type { StringStorage } from "./browser-storage.ts";
import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useResource, type Resource } from "./use-resource.ts";

/** What the server knows of an update, for its admins (GET /system/update; security-updates.md). */
export type Update = components["schemas"]["UpdateBody"];

/**
 * The storage key holding until when, and for which fix, this browser's admin snoozed the security
 * banner (client-keys.ts): "<until, Unix ms> <fixedIn>".
 */
export const securityUpdateSnoozeKey = "orpheum.securityUpdate.snoozed";

/** How long a snooze hides the banner: a day (security-updates.md Part B). */
export const snoozeLastsMs = 24 * 60 * 60 * 1000;

/**
 * Whether this browser's admin snoozed the banner of the fix in fixedIn, and the day has not passed
 * at nowMs. A snooze of another fix hides nothing: a new fix is shown at once.
 */
export function isSnoozed(storage: StringStorage, fixedIn: string, nowMs: number): boolean {
  const [until = "", snoozedFix] = (storage.getItem(securityUpdateSnoozeKey) ?? "").split(" ");
  return snoozedFix === fixedIn && nowMs < Number(until);
}

/** Snoozes the banner of the fix in fixedIn, in this browser, for a day from nowMs. */
export function snooze(storage: StringStorage, fixedIn: string, nowMs: number): void {
  storage.setItem(securityUpdateSnoozeKey, `${String(nowMs + snoozeLastsMs)} ${fixedIn}`);
}

/**
 * What the server knows of an update, read once for an admin; nothing is asked for anyone else, whom
 * the server would refuse.
 */
export function useUpdate(isAdmin: boolean): Resource<Update | undefined> {
  const { api } = useServices();
  const [update] = useResource(isAdmin ? "system-update" : "system-update-not-admin", async () =>
    isAdmin ? dataOrThrow(await api.GET("/system/update")) : undefined,
  );
  return update;
}
