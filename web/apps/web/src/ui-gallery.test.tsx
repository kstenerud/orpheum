// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { expect, it } from "vitest";

import { parseRoute } from "./router.ts";
import { withServices } from "./test-services.tsx";
import { UiGallery } from "./ui-gallery.tsx";

it("shows every status cue over pale, mid, dark and busy posters, at /ui-gallery (the owner, 2026-10-17)", () => {
  expect(parseRoute("/ui-gallery", "")).toEqual({ screen: "ui-gallery" });
  const markup = renderToStaticMarkup(withServices(<UiGallery />));
  for (const tone of ["light", "medium", "dark", "busy"]) {
    expect(markup).toContain(`ui-gallery__poster--${tone}`);
  }
  for (const icon of [
    "warning-icon",
    "uncertain-match-icon",
    "still-reading-icon",
    "watched-icon",
  ]) {
    expect(markup).toContain(icon);
  }
  expect(markup).toContain("Still being read, match guessed");
  expect(markup).toContain("Match guessed, watched");
});

it("shows every icon the app draws together, each drawn by Icon (the owner, 2026-10-27)", () => {
  const markup = renderToStaticMarkup(withServices(<UiGallery />));
  const icons = markup.slice(
    markup.indexOf('id="gallery-icons"'),
    markup.indexOf('id="gallery-buttons"'),
  );
  const drawn = [...icons.matchAll(/<svg class="icon ([^"]+)"/gu)].map((match) => match[1]);
  expect(drawn).toEqual([
    "warning-icon",
    "uncertain-match-icon",
    "uncertain-match-icon",
    "still-reading-icon",
    "still-reading-icon",
    "watched-icon",
    "app-shell__menu-icon",
    "status-tray__icon",
    "status-tray__icon",
    "together-voice__icon",
    "together-voice__icon",
    "settings__icon",
    "settings__icon",
    "route-chooser__chevron",
    "poster__clapperboard",
    ...Array.from({ length: 15 }, () => "player-icons__icon"),
  ]);
});

it("shows the banner's status tray with each sign it can show, Watch Together's and Cast's always, off or on, beside the menu (D364 §12)", () => {
  const markup = renderToStaticMarkup(withServices(<UiGallery />));
  const trays = [...markup.matchAll(/<div class="ui-gallery__banner">(.*?)<\/div><\/div>/gu)].map(
    (match) => match[1] ?? "",
  );
  expect(trays).toHaveLength(5);
  const signs = trays.map((tray) => [
    tray.includes("status-tray__button--warning"),
    [...tray.matchAll(/class="icon-button( icon-button--running)?"/gu)]
      .map((match) => (match[1] === undefined ? "off" : "on"))
      .join(" "),
    tray.includes("app-shell__menu-icon"),
  ]);
  expect(signs).toEqual([
    [false, "off off", true],
    [true, "off off", true],
    [false, "on off", true],
    [false, "off on", true],
    [true, "on on", true],
  ]);
});

it("shows the player's widgets, drawn by the components themselves (D369 §1)", () => {
  const markup = renderToStaticMarkup(withServices(<UiGallery />));
  const player = markup.slice(markup.indexOf('id="gallery-player"'));
  expect(player).toContain('class="icon-button" aria-label="An icon button"');
  expect(player).toContain(
    'class="icon-button icon-button--over-video" aria-label="An icon button over a film"',
  );
  expect([...player.matchAll(/class="timeline__range"/gu)]).toHaveLength(3);
  expect([...player.matchAll(/<div class="player-controls">/gu)]).toHaveLength(2);
  for (const name of ["Pause", "Full screen", "Mute", "Back 10 s", "Forward 10 s"]) {
    expect(player).toContain(`aria-label="${name}"`);
  }
});
