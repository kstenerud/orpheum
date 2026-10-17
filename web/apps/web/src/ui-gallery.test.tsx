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
  for (const icon of ["warning-icon", "uncertain-match-icon", "still-reading-icon"]) {
    expect(markup).toContain(icon);
  }
  expect(markup).toContain("Still being read, match guessed");
});

it("shows the banner's status tray with each sign it can show, beside the menu (the owner, 2026-10-17)", () => {
  const markup = renderToStaticMarkup(withServices(<UiGallery />));
  const trays = [...markup.matchAll(/<div class="ui-gallery__banner">(.*?)<\/div><\/div>/gu)].map(
    (match) => match[1] ?? "",
  );
  expect(trays).toHaveLength(3);
  const signs = trays.map((tray) => [
    tray.includes("status-tray__button--warning"),
    tray.includes("status-tray__icon"),
    tray.includes("app-shell__menu-icon"),
  ]);
  expect(signs).toEqual([
    [true, false, true],
    [false, true, true],
    [true, true, true],
  ]);
});
