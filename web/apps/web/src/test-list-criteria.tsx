// SPDX-License-Identifier: MPL-2.0

/**
 * A person's steps through a list's Sort and filter (listing-tools.tsx, D417), for the tests of the
 * pages that have one: open it, type in its field, choose in its choosers, close it. What is chosen
 * reaches the address as the pop-up closes (useListCriteria), so each step that writes closes it.
 */

import { act } from "react";

import { buttonNamed, click, enter } from "./test-dom.tsx";

/** The list's Sort and filter button, on its bar. */
export function criteriaButton(container: HTMLElement): HTMLButtonElement | null {
  return container.querySelector<HTMLButtonElement>(".list-criteria__button");
}

/** The pop-up the button opened, null while it is closed. */
export function criteriaPopUp(container: HTMLElement): HTMLDialogElement | null {
  return container.querySelector(".list-criteria")?.closest("dialog") ?? null;
}

/** Opens the pop-up, unless it is open. */
export async function openCriteria(container: HTMLElement): Promise<HTMLDialogElement> {
  if (criteriaPopUp(container) === null) {
    await click(criteriaButton(container));
  }
  const popUp = criteriaPopUp(container);
  if (popUp === null) {
    throw new Error("the Sort and filter did not open");
  }
  return popUp;
}

/** Closes the pop-up as Escape does (the dialog's cancel), what was chosen in it going to the address. */
export async function closeCriteria(container: HTMLElement): Promise<void> {
  const popUp = criteriaPopUp(container);
  if (popUp === null) {
    return;
  }
  await act(async () => {
    popUp.dispatchEvent(new Event("cancel", { cancelable: true }));
    await Promise.resolve();
  });
}

/** Chooses text in the pop-up's chooser of field (Sort, Genre, Decade), leaving it open. */
export async function chooseInCriteria(
  container: HTMLElement,
  field: string,
  text: string,
): Promise<void> {
  const popUp = await openCriteria(container);
  const chooser = [...popUp.querySelectorAll<HTMLElement>(".route-chooser")].find(
    (each) => each.querySelector(".route-chooser__field")?.textContent === field,
  );
  if (chooser === undefined) {
    throw new Error(`no chooser of ${field} in the Sort and filter`);
  }
  await click(chooser.querySelector("button[aria-haspopup]"));
  const list = chooser.querySelector<HTMLElement>("dialog");
  const choice = list === null ? undefined : buttonNamed(list, text);
  if (choice === undefined) {
    throw new Error(`no choice ${text} in the chooser of ${field}`);
  }
  await click(choice);
}

/** Chooses the sort labelled label, then closes the pop-up. */
export async function chooseSort(container: HTMLElement, label: string): Promise<void> {
  await chooseInCriteria(container, "Sort", label);
  await closeCriteria(container);
}

/** Types text into the pop-up's field, the name the list's posters are narrowed by, then closes it. */
export async function narrowByName(container: HTMLElement, text: string): Promise<void> {
  const popUp = await openCriteria(container);
  await enter(popUp.querySelector("input.list-criteria__name"), text);
  await closeCriteria(container);
}

/** Presses the pop-up's Unwatched toggle, leaving it open. */
export async function pressUnwatched(container: HTMLElement): Promise<void> {
  const popUp = await openCriteria(container);
  await click(popUp.querySelector(".list-chips__toggle"));
}
