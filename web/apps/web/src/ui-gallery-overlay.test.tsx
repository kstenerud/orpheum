// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { fire, mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { UiGallery } from "./ui-gallery.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** The gallery's full-screen overlay, the player's own (player-overlay.tsx). */
function galleryOverlay(): HTMLElement {
  const overlay = mounted?.container.querySelector<HTMLElement>(
    '.ui-gallery__film .player-overlay[data-shown="full"]',
  );
  if (overlay === null || overlay === undefined) {
    throw new Error("no overlay in the gallery");
  }
  return overlay;
}

/** The names of the pop-up a button of the overlay opens: its entries, as a list box shows them. */
async function popUpOf(name: string): Promise<string[]> {
  const opener = [...galleryOverlay().querySelectorAll("button")].find(
    (button) => button.getAttribute("aria-label") === name,
  );
  if (opener === undefined) {
    throw new Error(`no button ${name}`);
  }
  const wasOpen = new Set(galleryOverlay().querySelectorAll("dialog[open]"));
  await fire(opener, new MouseEvent("click", { bubbles: true }));
  const opened = [...galleryOverlay().querySelectorAll("dialog[open]")].filter(
    (dialog) => !wasOpen.has(dialog),
  );
  return opened.flatMap((dialog) =>
    [...dialog.querySelectorAll(".list-box li")].map((place) => place.textContent),
  );
}

// The UI review's "the gallery's pop-ups per widget and its real player rows" (the UI revamp's
// first wave, item 10).
it("shows the player's real overlay rows over a film, each of whose menus opens its own pop-up", async () => {
  mounted = await mount(withServices(<UiGallery />));
  const overlay = galleryOverlay();
  expect(overlay.querySelector('input[type="range"].timeline__range')).not.toBeNull();
  const names = [...overlay.querySelectorAll("button")].map((button) =>
    button.getAttribute("aria-label"),
  );
  expect(names).toEqual(
    expect.arrayContaining(["Subtitles: Off", "Sound track: English", "Settings"]),
  );
  expect(await popUpOf("Subtitles: Off")).toEqual(["Off", "English", "French"]);
  expect(await popUpOf("Sound track: English")).toEqual(["English", "English · commentary"]);
});

it("opens 🔈's row of sliders in its session: the film's, then each person's letters (D440 §2)", async () => {
  mounted = await mount(withServices(<UiGallery />));
  await popUpOf("Volume");
  const letters = [...galleryOverlay().querySelectorAll("dialog[open] .volume-row__letters")].map(
    (mark) => mark.textContent,
  );
  expect(letters).toEqual(["Gr", "Ga", "D"]);
  expect(galleryOverlay().querySelectorAll("dialog[open] input[type=range]")).toHaveLength(4);
});
