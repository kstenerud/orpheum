// SPDX-License-Identifier: MPL-2.0

/**
 * Which face button of a game controller is OK on this browser, as its person chose on the This
 * device page: the bottom one, the right one, or automatic, by the controller's make (input-pads.ts).
 * A pad's labels need not match the make it reports (the owner's 8BitDo SN30 Pro, 2026-10-25: in
 * X-input mode it reports an Xbox pad, its labels Nintendo's), so the choice wins over the guess. Kept
 * in this browser's storage, read by the app's navigation and by the device lab's button test alike.
 */

import { useSyncExternalStore } from "react";

import { browserStorage, type StringStorage } from "./browser-storage.ts";
import type { OkButton } from "./input-buttons.ts";

/** The person's choice: one of the face buttons, or automatic, by the controller's make. */
export type OkChoice = OkButton | "automatic";

/** The choices, in the order offered. */
export const okChoices: readonly OkChoice[] = ["automatic", "bottom", "right"];

/** The storage key the choice is kept under (client-keys.ts); none kept is automatic. */
export const okChoiceKey = "orpheum.controller.ok";

/** The choice a kept value says: automatic for none, or for one this release does not know. */
function choiceOf(kept: string | null): OkChoice {
  return kept === "bottom" || kept === "right" ? kept : "automatic";
}

/** The choice as last read or made, read once from storage: the poll asks for it every frame. */
let known: OkChoice | undefined;

/** Who is told of a new choice. */
const listeners = new Set<() => void>();

/** The person's choice of OK on this browser. */
export function okChoice(storage: () => StringStorage = browserStorage): OkChoice {
  known ??= choiceOf(storage().getItem(okChoiceKey));
  return known;
}

/** The face button the person chose as OK, or undefined for automatic. */
export function chosenOkButton(): OkButton | undefined {
  const choice = okChoice();
  return choice === "automatic" ? undefined : choice;
}

/** Keeps the person's choice of OK on this browser, and tells whoever listens. */
export function chooseOk(choice: OkChoice, storage: () => StringStorage = browserStorage): void {
  if (choice === "automatic") {
    storage().removeItem(okChoiceKey);
  } else {
    storage().setItem(okChoiceKey, choice);
  }
  known = choice;
  for (const listener of listeners) {
    listener();
  }
}

/** Tells listener of each new choice; returns the stopping. */
function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** The person's choice of OK, drawn again as it changes. */
export function useOkChoice(): OkChoice {
  return useSyncExternalStore(subscribe, () => okChoice());
}
