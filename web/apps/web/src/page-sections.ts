// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { createContext, use, type Context } from "react";

import type { Resource } from "./resource.ts";
import type { Given } from "./use-resource.ts";

/**
 * An admin page's first read, which carries what the page's other sections read, each as its own
 * endpoint answers it (sections=true; the client-server audit's finding 10): still waited on, or
 * read, with the sections it carried. It stands for each section's first read, as the shell's
 * stands for its parts' (D476 §4): each section stays fresh at its own endpoint, its poll asking
 * next at its first wait and a write's refresh reading there.
 */
export type PageRead<S> =
  { readonly state: "waiting" } | { readonly state: "read"; readonly sections: S | undefined };

/** The System page's sections, as its first read carries them. */
export type SystemSections = components["schemas"]["SystemSectionsBody"];
/** A user's page's sections: their devices and the roles to choose from. */
export type UserSections = components["schemas"]["UserSectionsBody"];
/** The Users page's sections: the roles (the invitations carry their links, D342). */
export type UsersSections = components["schemas"]["UsersSectionsBody"];

/** The System page's first read, for its sections; none outside it, where each reads its own. */
export const SystemSectionsContext: Context<PageRead<SystemSections> | undefined> = createContext<
  PageRead<SystemSections> | undefined
>(undefined);
/** A user's page's first read, for its sections. */
export const UserSectionsContext: Context<PageRead<UserSections> | undefined> = createContext<
  PageRead<UserSections> | undefined
>(undefined);
/** The Users page's first read, for its sections. */
export const UsersSectionsContext: Context<PageRead<UsersSections> | undefined> = createContext<
  PageRead<UsersSections> | undefined
>(undefined);

/**
 * The page read a page's first read stands for: waited on while it loads; read, with the sections
 * it carried. One that failed, or carried none, is read with none, and each section reads its own.
 */
export function pageReadOf<S>(
  resource: Resource<{ readonly sections?: S | undefined }>,
): PageRead<S> {
  if (resource.state === "loading") {
    return { state: "waiting" };
  }
  return {
    state: "read",
    sections: resource.state === "loaded" ? resource.value.sections : undefined,
  };
}

/**
 * What a section's first read is given of its page's (useResource's given): "waiting" while the
 * page's read is waited on; the part, where that read carried it whole; nothing where it did not, or
 * outside such a page, and the section then reads its own endpoint.
 */
export function sectionGiven<S, T>(
  read: PageRead<S> | undefined,
  part: (sections: S) => { readonly value: T } | undefined,
): Given<T> | undefined {
  if (read === undefined) {
    return undefined;
  }
  if (read.state === "waiting") {
    return "waiting";
  }
  return read.sections === undefined ? undefined : part(read.sections);
}

/**
 * What the page's own part's first read is given of the page's first read, which is that part's
 * answer whether or not it carried the sections: "waiting" while it is waited on; the part, as own
 * takes it from the read; nothing where the read failed, and the part reads itself.
 */
export function ownGiven<B, T>(
  first: Resource<B>,
  own: (read: B) => { readonly value: T } | undefined,
): Given<T> | undefined {
  if (first.state === "loading") {
    return "waiting";
  }
  return first.state === "loaded" ? own(first.value) : undefined;
}

/** sectionGiven of the page read context holds: the page's own, where the section is drawn on it. */
export function useSectionGiven<S, T>(
  context: Context<PageRead<S> | undefined>,
  part: (sections: S) => { readonly value: T } | undefined,
): Given<T> | undefined {
  return sectionGiven(use(context), part);
}

/**
 * A list's first page as a whole list's first read, where it is the whole list (no nextCursor); a
 * list with more reads itself, page by page, as before.
 */
export function wholeListOf<T>(
  entries: readonly T[] | null | undefined,
  nextCursor: string | undefined,
): { readonly value: T[] } | undefined {
  return nextCursor === undefined || nextCursor === ""
    ? { value: [...(entries ?? [])] }
    : undefined;
}
