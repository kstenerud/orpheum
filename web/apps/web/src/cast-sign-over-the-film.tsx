// SPDX-License-Identifier: MPL-2.0

import { createContext, type ReactNode } from "react";

/**
 * Cast's sign over the film (D422), drawn for the player's overlay by the signed-in app, which draws
 * the tray it belongs to: the sign opens Cast's dialog, whose code reaches back to the player, so the
 * overlay asks for it here rather than importing it. Given what tells the overlay its dialog is open
 * or closed, it gives the sign; nothing where no one is signed in, as on a guest's page by link
 * alone, which has no devices to cast to.
 */
export const CastSignOverTheFilmContext = createContext<
  (onOpenChange: (isOpen: boolean) => void) => ReactNode
>(() => null);
