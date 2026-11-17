// SPDX-License-Identifier: MPL-2.0

/**
 * The libraries a person reaches, as the banner's Libraries menu, Home and a library's page read them
 * (D349 §9.1): one way to read them for all, and their order, the person's locale's alphabetical.
 * The shell reads them once for every one of them under it (LibrariesReachedContext), so the
 * server is asked and polled once, not once per reader (the web client's architecture audit of
 * 2026-11-10, Startup 10).
 */

import type { components } from "@orpheum/sdk";
import { createContext, use, useMemo } from "react";

import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useShellPart } from "./shell-read.ts";
import { useResource, type Resource } from "./use-resource.ts";

/** A library, as the server sends it. */
export type LibraryBody = components["schemas"]["LibraryBody"];

/** The libraries the person reaches, in their locale's alphabetical order (D349 §9.1). */
export function sortedLibraries(loaded: readonly LibraryBody[]): LibraryBody[] {
  return [...loaded].sort((first, second) => first.name.localeCompare(second.name));
}

/** The libraries' one reading, and a way to read them again, as after starting a rescan. */
export interface LibrariesRead {
  readonly libraries: Resource<readonly LibraryBody[]>;
  readonly readAgain: () => void;
}

/**
 * The one reading of the libraries every reader under it shares; the shell provides it
 * (useLibrariesRead). Where none is provided, a reader reads them itself.
 */
export const LibrariesReachedContext = createContext<LibrariesRead | undefined>(undefined);

/** The libraries the person reaches, as the server lists them, read again as the server changes. */
export function useLibrariesReached(): Resource<readonly LibraryBody[]> {
  return useLibrariesReachedRead().libraries;
}

/**
 * The libraries' shared reading with the way to read it again: the admin's Libraries page reads the
 * shell's, one poll, not one of its own beside it (D478).
 */
export function useLibrariesReachedRead(): LibrariesRead {
  const shared = use(LibrariesReachedContext);
  const own = useLibrariesRead(shared === undefined);
  return shared ?? own;
}

/** A read the shared one makes needless: it never answers, so it asks nothing and sets nothing. */
const readsNothing = (): Promise<readonly LibraryBody[]> => new Promise(() => undefined);

/**
 * The reading of the libraries LibrariesReachedContext shares, or, where isNeeded is false (one is
 * shared already), none: no request and no poll. Its first read is the shell's (D476).
 */
export function useLibrariesRead(isNeeded = true): LibrariesRead {
  const { api } = useServices();
  const poller = useBackgroundChangesPoller();
  const fromShell = useShellPart((shell): readonly LibraryBody[] => shell.libraries ?? []);
  const [libraries, readAgain] = useResource(
    isNeeded ? "libraries-reached" : "libraries-shared",
    isNeeded
      ? async (): Promise<readonly LibraryBody[]> =>
          dataOrThrow(await api.GET("/libraries")).libraries ?? []
      : readsNothing,
    isNeeded ? { poller, given: fromShell } : {},
  );
  return useMemo(() => ({ libraries, readAgain }), [libraries, readAgain]);
}

/** Does nothing: where no reading is shared, none is kept that a change could leave behind. */
const readNothingAgain = (): void => undefined;

/**
 * The way to read the shared libraries again after a change the page made itself, as a library
 * created or deleted: the banner's Libraries menu lists them from that reading, and the
 * background-changes poller does not tell it of the change. Where none is shared it does nothing,
 * and reads nothing of its own.
 */
export function useReadLibrariesAgain(): () => void {
  return use(LibrariesReachedContext)?.readAgain ?? readNothingAgain;
}
