// SPDX-License-Identifier: MPL-2.0

import { createContext, use, type JSX, type ReactNode } from "react";

/**
 * Whether a Watch Together session's panel (TogetherPanel, together-over-the-film.tsx) is in Watch
 * Together's dialog now, opened over the film or on the cast's remote, rather than on its page: there
 * the slider row beside the dialog's button carries the people's volumes and the film's (D440 §2). A
 * context of its own, so the panel's parts read it without importing the panel's code.
 */
export const PanelInTheDialogContext = createContext(false);

/**
 * What a session's panel shows only while it is on its page, left out while it is in Watch Together's
 * dialog: the volumes, the film's or the people's, which the slider row beside the dialog's button
 * carries (D440 §8, amending D281 §5; where the row does not show, on a phone or a TV with nothing to
 * mix, there are no levels for the person to set, D440 §2); and the host's line that the session is
 * on, which the dialog's controls above it already say (the owner, staging, 2026-11-07: "another
 * copy of the dialog below the first").
 */
export function OnlyOnThePanelsPage(props: { readonly children: ReactNode }): JSX.Element | null {
  return use(PanelInTheDialogContext) ? null : <>{props.children}</>;
}
