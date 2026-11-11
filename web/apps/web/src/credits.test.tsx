// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

/**
 * A title's people and a credited person's page (search-by-more §4.4): a film's and a series' page
 * show their cast in order, each one's part under their name, then their directors and writers, each
 * leading to their page, which lists their titles, each leading to its own.
 */

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it } from "vitest";

import { CreditedPerson, initialsOf, type Credits } from "./credits.tsx";
import { SeriesView, type SeriesDetail } from "./series.tsx";
import { mount, servedBy, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { TitleView, type TitleDetail } from "./title.tsx";

const credits: Credits = {
  cast: [
    { personId: 41, name: "Ada Wexcombe", character: "Teodor Vane" },
    { personId: 42, name: "Oren de Talisk", character: "Silas Marrow" },
    { personId: 43, name: "Ivo Brannick" },
  ],
  directors: [{ personId: 50, name: "Mira Holloway-Pent", pictureRef: "/holloway.jpg" }],
  writers: [{ personId: 50, name: "Mira Holloway-Pent" }],
};

const uncredited: TitleDetail = {
  id: 10,
  kind: "film",
  name: "Metropolis",
  identificationState: "identified",
  art: {},
  versions: null,
  extras: null,
};

const heat: TitleDetail = { ...uncredited, credits };

const longRun: SeriesDetail = {
  id: 6,
  kind: "series",
  name: "Long Run",
  art: {},
  seasons: [{ seasonNumber: 1, episodeCount: 1 }],
  credits: {
    cast: [{ personId: 44, name: "Ann Lead", character: "The Lead" }],
    directors: null,
    writers: null,
  },
};

/** The names a page's people links show, in order, with where each leads. */
function creditsIn(container: ParentNode): string[] {
  return [...container.querySelectorAll("a.credit")].map(
    (link) =>
      `${link.querySelector(".credit__name")?.textContent ?? ""}|${link.querySelector(".credit__character")?.textContent ?? ""}|${link.getAttribute("href") ?? ""}`,
  );
}

/** Markup parsed into a document fragment, to query. */
function parsed(markup: string): ParentNode {
  const template = document.createElement("template");
  template.innerHTML = markup;
  return template.content;
}

describe("a title's people", () => {
  it("shows a film's cast in billing order, each part under the name, then its directors and writers, each leading to their page", () => {
    const page = parsed(renderToStaticMarkup(withServices(<TitleView title={heat} />)));
    expect(creditsIn(page)).toEqual([
      "Ada Wexcombe|Teodor Vane|/people/41",
      "Oren de Talisk|Silas Marrow|/people/42",
      "Ivo Brannick||/people/43",
      "Mira Holloway-Pent||/people/50",
      "Mira Holloway-Pent||/people/50",
    ]);
    expect([...page.querySelectorAll(".credits h2")].map((heading) => heading.textContent)).toEqual(
      ["Cast", "Directed by", "Written by"],
    );
    // No picture is served yet: initials hold the face's place, and no image is asked for.
    expect(page.querySelector(".credit .person-face")?.textContent).toBe("AW");
    expect(page.querySelector(".credits img")).toBeNull();
  });

  it("shows nothing of people for a title that credits no one", () => {
    const page = parsed(renderToStaticMarkup(withServices(<TitleView title={uncredited} />)));
    expect(page.querySelector(".credits")).toBeNull();
  });

  it("shows a series' cast on its page, leaving out the groups it has none in", () => {
    const page = parsed(renderToStaticMarkup(withServices(<SeriesView series={longRun} />)));
    expect(creditsIn(page)).toEqual(["Ann Lead|The Lead|/people/44"]);
    expect([...page.querySelectorAll(".credits h2")].map((heading) => heading.textContent)).toEqual(
      ["Cast"],
    );
  });

  it("makes initials of a name's first and last words", () => {
    expect(initialsOf("Oren de Talisk")).toBe("OT");
    expect(initialsOf("Cher")).toBe("C");
    expect(initialsOf("  ")).toBe("");
  });
});

describe("a credited person's page", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("shows their name and their titles in the server's order, newest first, each leading to its page with what they did", async () => {
    mounted = await mount(
      withServices(
        <CreditedPerson id={50} />,
        servedBy({
          "GET /people/50": [200, { id: 50, name: "Mira Holloway-Pent" }],
          "GET /items": [
            200,
            {
              items: [
                {
                  id: 7,
                  kind: "series",
                  name: "Brass Arena",
                  year: 2020,
                  libraryId: 1,
                  identificationState: "identified",
                  credit: { jobs: ["writer"] },
                },
                {
                  id: 10,
                  kind: "film",
                  name: "Metropolis",
                  year: 1927,
                  libraryId: 1,
                  identificationState: "identified",
                  credit: { jobs: ["director", "cast"], character: "A guard" },
                },
              ],
            },
          ],
        }),
      ),
    );
    const { container } = mounted;
    expect(container.querySelector("h1")?.textContent).toBe("Mira Holloway-Pent");
    expect(
      [...container.querySelectorAll(".credited-person__title")].map(
        (link) => `${link.getAttribute("href") ?? ""} ${link.textContent}`,
      ),
    ).toEqual(["/series/7 Brass Arena2020Writer", "/titles/10 Metropolis1927Director, as A guard"]);
  });
});
