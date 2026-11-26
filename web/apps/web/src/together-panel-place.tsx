// SPDX-License-Identifier: MPL-2.0

import { createContext, use, type Context, type JSX, type ReactNode } from "react";

/**
 * Whether a Watch Together session's panel (TogetherPanel, together-over-the-film.tsx) is in Watch
 * Together's dialog now, opened over the film or on the cast's remote, rather than on its page. A
 * context of its own, so the panel's parts read it without importing the panel's code.
 */
export const PanelInTheDialogContext: Context<boolean> = createContext(false);

/**
 * What a session's panel shows only while it is on its page, left out while it is in Watch Together's
 * dialog: the host's line that the session is on, which the dialog's controls above it already say
 * (the owner, staging, 2026-11-07: "another copy of the dialog below the first"). The volumes, the
 * film's and the people's, are in the player's 🔈 alone, never in the panel (the owner, 2026-11-26:
 * "merged into the volume button in the bottom bar"; D440 §2).
 */
export function OnlyOnThePanelsPage(props: { readonly children: ReactNode }): JSX.Element | null {
  return use(PanelInTheDialogContext) ? null : <>{props.children}</>;
}
