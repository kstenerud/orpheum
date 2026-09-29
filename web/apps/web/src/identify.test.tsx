// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { CandidateList, Identify, WaitingList } from "./identify.tsx";
import { withServices } from "./test-services.tsx";

const ignore = (): void => undefined;

describe("identify", () => {
  it("lists each waiting item with a way to choose its match and to look again", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <WaitingList
          waiting="ambiguous"
          onChanged={ignore}
          items={[
            {
              id: 7,
              kind: "series",
              name: "Glass Tide",
              identificationState: "ambiguous",
              libraryId: 1,
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
        withServices(<WaitingList waiting="unidentified" onChanged={ignore} items={[]} />),
      ),
    ).toContain("Nothing is waiting.");
  });

  it("offers each candidate, with its original name where it differs", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <CandidateList
          onPick={ignore}
          candidates={[
            { providerId: "101", name: "Variety", originalName: "Variety", year: 1994 },
            { providerId: "7", name: "Léon Morin, Priest" },
          ]}
        />,
      ),
    );
    expect(markup).toContain("Variety (1994)");
    expect(markup).toContain("Originally “Variety”");
    expect(markup).toContain("Léon Morin, Priest (?)");
    expect(markup.match(/This one/gu)).toHaveLength(2);
  });

  it("starts with the items that have several matches", () => {
    const markup = renderToStaticMarkup(withServices(<Identify />));
    expect(markup).toContain('aria-pressed="true">Several matches');
  });
});
