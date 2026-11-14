// SPDX-License-Identifier: MPL-2.0

/**
 * Who is signed in, as the shell read it, for every screen within it (D478): a screen that needs to
 * know reads the shell's, not GET /me again (the client-server audit of 2026-11-13, finding 8).
 */

import type { components } from "@orpheum/sdk";
import { createContext, use } from "react";

import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useResource, type Resource } from "./use-resource.ts";

/** The signed-in person, as GET /me answers. */
export type Me = components["schemas"]["UserBody"];

/** Who is signed in, and a way to read it again once something about them changed. */
export interface SignedInMe {
  readonly me: Resource<Me>;
  readonly readMeAgain: () => void;
}

/** The shell's read of who is signed in (app.tsx's SignedIn provides it). */
export const MeContext = createContext<SignedInMe | undefined>(undefined);

/** A read the shell's makes needless: it never answers, so it asks nothing and sets nothing. */
const readsNothing = (): Promise<Me> => new Promise(() => undefined);

/**
 * Who is signed in: the shell's read, or, with none above (a screen drawn alone, as a test draws
 * one), the screen's own, made only when isAsked (a page that may be open signed out asks nothing
 * then).
 */
export function useMe(isAsked = true): SignedInMe {
  const shared = use(MeContext);
  const { api } = useServices();
  const isOwnRead = shared === undefined && isAsked;
  const [me, readMeAgain] = useResource(
    isOwnRead ? "me" : "me-not-read",
    isOwnRead ? async () => dataOrThrow(await api.GET("/me")) : readsNothing,
  );
  return shared ?? { me, readMeAgain };
}
