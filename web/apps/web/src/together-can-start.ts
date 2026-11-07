// SPDX-License-Identifier: MPL-2.0

import { createContext } from "react";

/**
 * Whether the person may start a Watch Together session (their role's canHostWatchTogether), as the
 * signed-in app's shell (app-shell.tsx) tells the player's overlay, whose sign over the film then shows with no session too
 * (D440 §6, together-over-the-film.tsx). A context of its own, as Cast's sign is
 * (cast-sign-over-the-film.tsx), so the app's first chunk does not take the player's code; false,
 * the default, where no one is signed in, as on a guest's page by link alone (D246 §3).
 */
export const CanStartTogetherContext = createContext(false);
