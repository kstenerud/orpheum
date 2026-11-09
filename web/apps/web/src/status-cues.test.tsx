// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { i18n } from "./i18n.ts";
import { statusCues, StatusNotes, StatusTray } from "./status-cues.tsx";
import { withServices } from "./test-services.tsx";
import { IsAdminContext } from "./warning-sign.tsx";

describe("the status cues", () => {
  it("are one list, the warning of one that cannot play always first (the owner, 2026-10-15)", () => {
    expect(statusCues.map((cue) => cue.name)).toEqual([
      "cannotPlay",
      "uncertainMatch",
      "stillReading",
      "watched",
    ]);
  });

  it("show in the list's order over a poster and on a page, and nothing for a subject with none", () => {
    const both = {
      unavailable: { reason: "nodeAway" as const, nodeName: "devnas" },
      isGuessed: true,
    };
    for (const markup of [
      renderToStaticMarkup(withServices(<StatusTray subject={both} />)),
      renderToStaticMarkup(withServices(<StatusNotes subject={both} />)),
    ]) {
      const warning = markup.indexOf("This cannot be played now");
      expect(warning).toBeGreaterThanOrEqual(0);
      expect(markup.indexOf("This may be identified incorrectly")).toBeGreaterThan(warning);
    }
    expect(renderToStaticMarkup(withServices(<StatusTray subject={{}} />))).toBe("");
  });

  it("mark what its person watched with the white check, the tray's last, read as Watched (D370 §2)", () => {
    // As the API's schema types it, so a schema without it does not type-check.
    const isWatched: components["schemas"]["TitleEntryBody"]["isWatched"] = true;
    const watchedAndGuessed = { isGuessed: true, isWatched };
    const tray = renderToStaticMarkup(withServices(<StatusTray subject={watchedAndGuessed} />));
    expect(tray).toContain("watched-icon");
    expect(tray).toContain('<span class="poster-cue__name">Watched</span>');
    expect(tray.indexOf("watched-icon")).toBeGreaterThan(tray.indexOf("uncertain-match-icon"));
    const notes = renderToStaticMarkup(withServices(<StatusNotes subject={watchedAndGuessed} />));
    expect(notes).toContain("<span>Watched</span>");
    expect(renderToStaticMarkup(withServices(<StatusTray subject={{ isWatched: false }} />))).toBe(
      "",
    );
  });

  it("mark a title still being read with its own cue, not the cannot-play warning (D304 §4)", () => {
    // The reason as the API's schema types it, so a schema without it does not type-check.
    const stillReading: components["schemas"]["UnavailableBody"]["reason"] = "stillReading";
    const reading = { unavailable: { reason: stillReading, nodeName: "devnas" } };
    for (const markup of [
      renderToStaticMarkup(withServices(<StatusTray subject={reading} />)),
      renderToStaticMarkup(withServices(<StatusNotes subject={reading} />)),
    ]) {
      expect(markup).toContain("Still reading");
      expect(markup).not.toContain("This cannot be played now");
    }
  });

  it("tell an administrator a guessed match's page has both Identified incorrectly and Identified correctly", () => {
    // The note names the page's two menu items by their own labels, so a renamed item renames it too.
    const markup = renderToStaticMarkup(
      withServices(
        <IsAdminContext value={true}>
          <StatusNotes subject={{ isGuessed: true }} />
        </IsAdminContext>,
      ),
    );
    expect(markup).toContain(i18n.t("title.identifiedIncorrectly"));
    expect(markup).toContain(i18n.t("title.identifiedCorrectly"));
  });
});
