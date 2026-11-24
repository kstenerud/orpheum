// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { readFileSync } from "node:fs";
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
    "waiting-icon",
    "watched-icon",
    "app-shell__menu-icon",
    "status-tray__icon",
    "status-tray__icon",
    "together-voice__icon",
    "together-voice__icon",
    "settings__icon",
    "settings__icon",
    "eye-icon",
    "eye-icon",
    "route-chooser__chevron",
    "poster__clapperboard",
    "file-browser__icon",
    "file-browser__icon",
    "file-browser__icon",
    "app-shell__place-glyph",
    "app-shell__place-glyph",
    "app-shell__place-glyph",
    "more-actions__icon",
    "listing-tools__icon",
    "listing-tools__icon",
    "listing-tools__arrow",
    "listing-tools__arrow",
    "poster-row__arrow-icon",
    "poster-row__arrow-icon",
    "stepper__sign",
    "stepper__sign",
    "checkbox__tick",
    "cast-kind cast-kind--chromecast",
    "cast-kind cast-kind--find",
    "volume-row__film-icon",
    "poster__clock",
    ...Array.from({ length: 17 }, () => "player-icons__icon"),
    "how-it-plays__icon",
  ]);
});

it("shows each icon of icons.tsx and of player-icons.tsx in its icons section (the owner, 2026-11-11)", () => {
  const gallery = readFileSync(new URL("./ui-gallery.tsx", import.meta.url), "utf8");
  for (const file of ["icons.tsx", "player-icons.tsx"]) {
    const source = readFileSync(new URL(`./${file}`, import.meta.url), "utf8");
    const names = [...source.matchAll(/^export function (\w+)\(/gmu)].map(
      (match) => match[1] ?? "",
    );
    expect(names.length).toBeGreaterThan(10);
    for (const name of names) {
      expect(gallery, `${file}'s ${name}`).toMatch(new RegExp(`<${name}\\b`, "u"));
    }
  }
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
  expect([...player.matchAll(/class="timeline__range"/gu)]).toHaveLength(4);
  expect([...player.matchAll(/<div class="player-controls">/gu)]).toHaveLength(3);
  for (const name of ["Pause", "Full screen", "Mute", "Back 10 s", "Forward 10 s"]) {
    expect(player).toContain(`aria-label="${name}"`);
  }
});

it("shows a stepper among the buttons, its field, its unit and its − and + (the owner, 2026-11-22; D538 §9)", () => {
  const markup = renderToStaticMarkup(withServices(<UiGallery />));
  const buttons = markup.slice(
    markup.indexOf('id="gallery-buttons"'),
    markup.indexOf('id="gallery-pop-ups"'),
  );
  expect(buttons).toMatch(
    /<input type="text" class="stepper__field"[^>]*aria-label="A number, in ms, from -500 to 500"[^>]*value="0"/u,
  );
  expect(buttons).toContain('aria-label="50 ms less"');
  expect(buttons).toContain('aria-label="50 ms more"');
  expect(buttons).toMatch(
    /<output id="[^"]+" class="stepper__caption stepper__caption--unshown">A stepper: 0 ms<\/output>/u,
  );
});

it("shows the form parts, each switch a button of role switch saying its state (D538, interface rule 7)", () => {
  const markup = renderToStaticMarkup(withServices(<UiGallery />));
  const forms = markup.slice(
    markup.indexOf('id="gallery-forms"'),
    markup.indexOf('id="gallery-pop-ups"'),
  );
  expect(forms).toMatch(
    /<button type="button" role="switch" class="switch" aria-checked="false"[^>]*>.*A switch</u,
  );
  expect(forms).toContain('<span class="switch__status"> (+4.1 dB)</span>');
});

it("shows the parts the widgets are built from, a pop-up at its button and the layout primitives (D408; plans/toolkit.md step 7)", () => {
  const markup = renderToStaticMarkup(withServices(<UiGallery />));
  const parts = markup.slice(
    markup.indexOf('id="gallery-pop-ups"'),
    markup.indexOf('id="gallery-player"'),
  );
  expect(parts).toMatch(
    /<div[^>]*aria-haspopup="dialog"[^>]*role="button"[^>]*>Open a pop-up<\/div>/u,
  );
  expect(parts).toContain('<div class="stack layout-space-2"><button type="button">First choice');
  expect(parts).toContain(
    '<div class="cluster cluster--align-center layout-space-3"><button type="button">First choice',
  );
});

it("draws a film waiting for data with the player's own spinner and its bar, the real thing once its look is final (the owner, 2026-11-17)", () => {
  const markup = renderToStaticMarkup(withServices(<UiGallery />));
  const player = markup.slice(markup.indexOf('id="gallery-player"'));
  const waiting = player.slice(player.indexOf("A film waiting for data"));
  expect(waiting).toMatch(
    /<div class="ui-gallery__film"><span class="film-waits"><span class="film-waits__sign" aria-hidden="true"><span class="film-waits__spinner"><\/span><span class="film-waits__hourglass">/u,
  );
  expect(waiting).toContain(
    '<span class="film-waits__bar" aria-hidden="true"><span class="film-waits__fill" style="width:40%">',
  );
  // Its words are a Pill's, in place of the sign's label (D538 §1).
  expect(waiting).toContain(
    '<span class="film-waits__words"><output class="pill pill--said"><span class="pill__words">Getting ready to play</span></output></span>',
  );
  expect(markup).not.toMatch(/ui-gallery__(ring|hourglass|link)/u);
});

it("shows a page's layout parts, drawn by the parts themselves: Page, Section, ActionRow, Pill and OfferCard (D538 §1)", () => {
  const markup = renderToStaticMarkup(withServices(<UiGallery />));
  const parts = markup.slice(
    markup.indexOf('id="gallery-layout-parts"'),
    markup.indexOf('id="gallery-player"'),
  );
  for (const part of [
    'class="page"',
    'class="page__job"',
    'class="section"',
    'class="action-row"',
    'class="pill pill--said"',
    'class="pill pill--volume"',
    'class="pill pill--silent"',
    'class="pill pill--action"',
    'class="offer-card"',
    'class="offer-card__content offer-card__take"',
  ]) {
    expect(parts).toContain(part);
  }
  expect(parts).toContain("Needs you");
  expect(parts).toContain("Still watching?");
});

it("draws each part in its place over a film, and each offer in its states (D538 §1, player-spec's three places)", () => {
  const markup = renderToStaticMarkup(withServices(<UiGallery />));
  const places = markup.slice(
    markup.indexOf('class="ui-gallery__film ui-gallery__places"'),
    markup.indexOf('class="ui-gallery__film ui-gallery__over-film"'),
  );
  expect(places).toContain('class="sound-on-pill"');
  expect(places).toMatch(
    /class="player-overlay-centre player-overlay-volume">(?:<[^>]+>)*Volume 70 %/u,
  );
  expect(places).toMatch(/class="player-overlay-seeked" data-side="right" data-shown=""/u);
  expect(places).toMatch(/class="ui-gallery__offer-place"(?:(?!corner-notes).)*Plays in 8 s/u);
  expect(places).not.toContain("Dismiss Up next"); // no ✕: Back puts it away (the owner, 2026-11-23)
  expect(places).toContain('class="corner-notes"');
  const states = markup.slice(markup.indexOf('class="ui-gallery__offers"'));
  expect(states).toMatch(/Up next: [^<]+ · Cat/u);
  expect(states).toContain("Play now");
  expect(states).toMatch(/class="offer-card__content offer-card__take">(?:<[^>]+>)*Skip intro/u);
  expect(states).toContain("Still watching?");
});

it("shows the file browser loading, empty, with a count not finished, unreadable and with a machine not answering (D539 §1)", () => {
  const markup = renderToStaticMarkup(withServices(<UiGallery />));
  const states = markup.slice(markup.indexOf('id="gallery-file-browser"'));
  expect(states).toContain('aria-busy="true"');
  expect(states).toContain("No folders in media");
  expect(states).toContain("12 videos");
  expect(states).toContain("Orpheum can&#x27;t read media");
  expect(states).toContain("attic isn&#x27;t answering");
});

it("draws the player's new Pills and every shell Banner in each state (D538 §1)", () => {
  const markup = renderToStaticMarkup(withServices(<UiGallery />));
  const overFilm = markup.slice(markup.indexOf('id="gallery-layout-parts"'));
  // Stop casting here, and disabled while its end is asked for.
  expect(overFilm).toMatch(
    /<button type="button" class="pill pill--action" data-dangerous-action="">(?:<[^>]+>)*Stop casting here/u,
  );
  expect(overFilm).toMatch(
    /<button type="button" class="pill pill--action" data-dangerous-action="" disabled="">(?:<[^>]+>)*Stop casting here/u,
  );
  // The sound-on prompt: as an action, for the voices, and a receiver's notice.
  expect(overFilm.match(/class="sound-on-pill"/gu)).toHaveLength(3);
  expect(overFilm).toContain("Tap to hear everyone&#x27;s voices");
  // The corner's stack of three.
  expect(overFilm).toMatch(
    /class="corner-notes"(?:(?!<\/div>).)*Sam joined(?:(?!<\/div>).)*Alex left(?:(?!<\/div>).)*Controlled from/u,
  );
  const overlays = markup.slice(markup.indexOf('id="gallery-overlays"'));
  for (const words of [
    "This page lost its connection to your server",
    "A security update is available (v0.9.1)",
    "For testers: media won",
    "This page is open over plain http",
  ]) {
    expect(overlays).toContain(words);
  }
  expect(overlays).toContain("Remind me tomorrow");
});

it("draws Settings' pages by their own views: the index, a search found and not, a section's settings saved, refused and unanswered, and Keys and passwords (D538 §11)", () => {
  const markup = renderToStaticMarkup(withServices(<UiGallery />));
  const settings = markup.slice(
    markup.indexOf('id="gallery-settings"'),
    markup.indexOf('id="gallery-overlays"'),
  );
  // The index: Keys and passwords first, its keys' states and its warning, then the sections.
  expect(settings).toContain(
    "TMDB API key set · OpenSubtitles username not set · OpenSubtitles password set",
  );
  expect(settings).toContain("warning-icon");
  expect(settings).toContain("Diagnostics");
  // A search's results, each with its page's name, and one nothing matches.
  expect(settings).toMatch(/Time zone<\/span><span class="settings-index__summary">General/u);
  expect(settings).toContain("No setting matches.");
  // A setting of each kind: a Switch, a Stepper and a text field.
  expect(settings).toContain('role="switch"');
  expect(settings).toContain("Most titles in Continue watching: one more");
  expect(settings).toContain('value="Europe/Berlin"');
  // Saved, refused, and every setting advanced.
  expect(settings.match(/>Saved</gu)?.length).toBeGreaterThanOrEqual(3);
  expect(settings.match(/The server can&#x27;t use this value\./gu)).toHaveLength(3);
  expect(settings).toContain("Every setting here is advanced.");
  // Keys and passwords: set with its last four, not set, and to be entered again.
  expect(settings).toContain("Set, ending in 4f2a");
  expect(settings).toContain("Not set");
  expect(settings.match(/>Remove…</gu)).toHaveLength(2);
});

it("shows a number with wide bounds as a stepper alone, as Settings draws it, never the browser's number field (the owner, 2026-11-23; VQ382)", () => {
  const markup = renderToStaticMarkup(withServices(<UiGallery />));
  const forms = markup.slice(markup.indexOf('id="gallery-forms"'));
  const numbers = forms.slice(
    forms.indexOf('id="gallery-numbers"'),
    forms.indexOf("</section>", forms.indexOf('id="gallery-numbers"')),
  );
  expect(numbers).toContain(">A number with wide bounds</h3>");
  expect(numbers).toContain("Most titles in Continue watching");
  expect(numbers).toMatch(/<input type="text" class="stepper__field"[^>]*value="20"/u);
  expect(numbers).toContain('aria-label="Most titles in Continue watching, from 1 to 1000"');
  expect(numbers).toContain('aria-label="Most titles in Continue watching: one more"');
  expect(numbers).not.toContain('type="number"');
  expect(markup).not.toContain('type="number"');
});
