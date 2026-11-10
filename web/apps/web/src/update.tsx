// SPDX-License-Identifier: MPL-2.0

import { reportFailure, type components } from "@orpheum/sdk";
import { createContext, use, useMemo, useRef, type JSX, type ReactNode } from "react";

import type { StringStorage } from "./browser-storage.ts";
import { shownAndEveryPoller } from "./polling.ts";
import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useResource, type Resource } from "./use-resource.ts";

/** What the server knows of an update, for its admins (GET /system/update; security-updates.md). */
export type Update = components["schemas"]["UpdateBody"];

/**
 * The storage key holding until when, and for which fix, this browser's admin snoozed the security
 * banner (client-keys.ts): "<until, Unix ms> <fixedIn>".
 */
/** The command that pulls the release a compose-file server follows and starts it (D450 §2). */
export const composeUpdateCommand = "docker compose pull && docker compose up -d";

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

/** How often a shown page reads the update state again (D462 §10.9): ten minutes. */
export const updateReadEveryMs = 10 * 60 * 1000;

/** The update state the shell reads for an admin and keeps current, and how to read it again now. */
interface SharedUpdate {
  readonly update: Resource<Update | undefined>;
  readonly readAgain: () => void;
}

const SharedUpdateContext = createContext<SharedUpdate | undefined>(undefined);

/**
 * Reads the update state once for everything within, for an admin: again each time the page is
 * shown and every ten minutes, so the menu's mark comes without a page change (D462 §10.9). A read
 * that fails keeps what was read before, so a moment offline takes no mark away; the page's Check
 * now says a failure itself. Nothing is asked for anyone else, whom the server would refuse.
 */
export function UpdateProvider(props: {
  readonly isAdmin: boolean;
  readonly children: ReactNode;
}): JSX.Element {
  const { api } = useServices();
  const lastRead = useRef<Update>(undefined);
  const { isAdmin } = props;
  const [update, readAgain] = useResource(
    isAdmin ? "system-update" : "system-update-not-admin",
    async (signal) => {
      if (!isAdmin) {
        return undefined;
      }
      try {
        lastRead.current = dataOrThrow(await api.GET("/system/update", { signal: signal ?? null }));
      } catch (cause) {
        if (lastRead.current === undefined) {
          throw cause;
        }
        reportFailure(cause);
      }
      return lastRead.current;
    },
    isAdmin ? updatePoller : undefined,
  );
  const shared = useMemo(() => ({ update, readAgain }), [update, readAgain]);
  return <SharedUpdateContext value={shared}>{props.children}</SharedUpdateContext>;
}

/** The update state's poller: when the page is shown again, and every ten minutes (D462 §10.9). */
const updatePoller = shownAndEveryPoller(updateReadEveryMs);

/**
 * What the server knows of an update, for an admin: the shell's (UpdateProvider) within it, else
 * read once; nothing is asked for anyone else, whom the server would refuse.
 */
export function useUpdate(isAdmin: boolean): Resource<Update | undefined> {
  const { api } = useServices();
  const shared = use(SharedUpdateContext);
  const isOwnRead = isAdmin && shared === undefined;
  const [update] = useResource(isOwnRead ? "system-update" : "system-update-not-read", async () =>
    isOwnRead ? dataOrThrow(await api.GET("/system/update")) : undefined,
  );
  if (!isAdmin) {
    return update;
  }
  return shared?.update ?? update;
}

/** Has the shell read the update state again now, as after Check now; nothing outside a shell. */
export function useReadUpdateAgain(): () => void {
  return use(SharedUpdateContext)?.readAgain ?? noReadAgain;
}

const noReadAgain = (): void => undefined;

/** Whether update says a newer release is out, which the menu marks "! Update" (D462 §8). */
export function isUpdateKnown(update: Update | undefined): boolean {
  return update?.latest !== undefined || update?.security !== undefined;
}
