// SPDX-License-Identifier: MPL-2.0

import { createContext, useContext } from "react";

/**
 * Whether the page plays for a guest, whom the core refuses every route of a person's own, their
 * preferences and a file's waits among them: a guest receiver, signed in for one cast alone (D469
 * §4), which the core opens only its cast's film's routes to, its Up next among them (D471 §1); or a
 * Watch Together guest's film, the host's, whose sound and subtitles the guest chooses for themselves
 * alone (D471). Its player asks for none of those routes, offering no levelling's waits and keeping
 * no sound or subtitle choice as the person's. False on every other page, and before the page knows
 * who is signed in.
 */
export const IsGuestContext = createContext(false);

/** Whether the page plays for a guest (IsGuestContext). */
export function useIsGuest(): boolean {
  return useContext(IsGuestContext);
}
