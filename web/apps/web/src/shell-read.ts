// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { createContext, use } from "react";

import type { Given } from "./use-resource.ts";

/** What the shell reads as it starts, beside who is signed in (D476). */
export type ShellBody = components["schemas"]["ShellBody"];

/**
 * The shell's one read as it starts (D476, principles §30): still waited on, or read, with what it
 * carried; undefined when it carried none (the server left it out, or the read failed), and each part
 * is then read at its own endpoint.
 */
export type ShellRead =
  { readonly state: "waiting" } | { readonly state: "read"; readonly shell: ShellBody | undefined };

/**
 * The shell's read for everything within it. With none above (a screen drawn alone, as a test draws
 * one), each part is read at its own endpoint, as it is when the read carried none.
 */
export const ShellReadContext = createContext<ShellRead>({ state: "read", shell: undefined });

/**
 * The part of the shell's read that stands for a resource's first read (useResource's given): waited
 * on while the read is, and undefined, so the resource reads its own, when the read carried none.
 */
export function useShellPart<T>(part: (shell: ShellBody) => T | undefined): Given<T> | undefined {
  const read = use(ShellReadContext);
  if (read.state === "waiting") {
    return "waiting";
  }
  const value = read.shell === undefined ? undefined : part(read.shell);
  return value === undefined ? undefined : { value };
}
