// SPDX-License-Identifier: MPL-2.0

import { createContext, useContext } from "react";

/**
 * Whether the page is signed in as a guest receiver, for one cast alone (D469 §4): the core refuses
 * such a session every route of a person's own (D465 §2), so its play screen asks for none of them,
 * offering no Up next, no levelling's waits and no sound choice kept as the person's. False on every
 * other page, and before the page knows who is signed in.
 */
export const IsGuestReceiverContext = createContext(false);

/** Whether the page is a guest receiver's (IsGuestReceiverContext). */
export function useIsGuestReceiver(): boolean {
  return useContext(IsGuestReceiverContext);
}
