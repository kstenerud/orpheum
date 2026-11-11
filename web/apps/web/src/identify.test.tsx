// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { CandidateList } from "./candidate-chooser.tsx";
import { shownFilter, WaitingList, WaitingTabs } from "./identify.tsx";
import { withServices } from "./test-services.tsx";

const ignore = (): void => undefined;

/** A film waiting, no search having found it. */
const waiting = {
  id: 7,
  kind: "film",
  name: "Hearthstead",
  identificationState: "unidentified",
  libraryId: 1,
} as const;

describe("identify", () => {
  it("lists each waiting item with a way to choose its match and to look again", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <WaitingList
          waitingFilter="ambiguous"
          onChanged={ignore}
          items={[
            {
              id: 7,
              kind: "series",
              name: "Glass Tide",
              identificationState: "ambiguous",
              year: 2011,
            },
          ]}
        />,
      ),
    );
    expect(markup).toContain('href="/series/7"');
    expect(markup).toContain("Choose its match");
    expect(markup).toContain("Look again");
    expect(
      renderToStaticMarkup(
        withServices(<WaitingList waitingFilter="unidentified" onChanged={ignore} items={[]} />),
      ),
    ).toContain("Nothing is waiting.");
  });

  it("offers each candidate, with its original name where it differs", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <CandidateList
          onPick={ignore}
          candidates={[
            { providerId: "101", name: "Théo: The Guardian", originalName: "Théo", year: 1994 },
            { providerId: "7", name: "Fièvre" },
          ]}
        />,
      ),
    );
    expect(markup).toContain("Théo: The Cameraman (1994)");
    expect(markup).toContain("Originally “Théo”");
    expect(markup).toContain("Fièvre (?)");
    expect(markup.match(/This one/gu)).toHaveLength(2);
  });

  it("opens on the items with several matches, or on those with none while there are no others", () => {
    const one = [waiting];
    expect(shownFilter(undefined, { ambiguous: one, unidentified: one })).toBe("ambiguous");
    expect(shownFilter(undefined, { ambiguous: [], unidentified: [] })).toBe("ambiguous");
    expect(shownFilter(undefined, { ambiguous: [], unidentified: one })).toBe("unidentified");
    // A tab a person chose stays, empty or not.
    expect(shownFilter("ambiguous", { ambiguous: [], unidentified: one })).toBe("ambiguous");
  });

  it("says on each tab how many it holds, and opens on one that holds any", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <WaitingTabs
          lists={{ ambiguous: [], unidentified: [waiting, { ...waiting, id: 8 }] }}
          chosen={undefined}
          onChoose={ignore}
          onChanged={ignore}
        />,
      ),
    );
    expect(markup).toContain('aria-pressed="false">Several matches (0)</button>');
    expect(markup).toContain('aria-pressed="true">No match (2)</button>');
  });

  // ui-audit-2026-11-04.md §3, Needs a match: arrival focuses the first item's main action.
  it("arrives on the first item's main action: Choose its match, or Look again where there are none", () => {
    const arrivals = (filter: "ambiguous" | "unidentified"): string[] => {
      const markup = renderToStaticMarkup(
        withServices(
          <WaitingList
            waitingFilter={filter}
            onChanged={ignore}
            items={[waiting, { ...waiting, id: 8, name: "Intolerance" }]}
          />,
        ),
      );
      return [...markup.matchAll(/<button[^>]*data-focus-start="arrival"[^>]*>([^<]*)</gu)].map(
        (match) => match[1] ?? "",
      );
    };
    expect(arrivals("ambiguous")).toEqual(["Choose its match"]);
    expect(arrivals("unidentified")).toEqual(["Look again"]);
  });
});
