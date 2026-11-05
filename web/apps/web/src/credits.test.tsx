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
    { personId: 41, name: "Ansel Vardo", character: "Mira Holloway-Pent" },
    { personId: 42, name: "Ada Wexcombe", character: "Tamsin Vell" },
    { personId: 43, name: "Ivo Brannick" },
  ],
  directors: [{ personId: 50, name: "Oren Talisk", pictureRef: "/mann.jpg" }],
  writers: [{ personId: 50, name: "Oren Talisk" }],
};

const uncredited: TitleDetail = {
  id: 10,
  kind: "movie",
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
      "Ansel Vardo|Mira Holloway-Pent|/people/41",
      "Ada Wexcombe|Tamsin Vell|/people/42",
      "Ivo Brannick||/people/43",
      "Oren Talisk||/people/50",
      "Oren Talisk||/people/50",
    ]);
    expect([...page.querySelectorAll(".credits h2")].map((heading) => heading.textContent)).toEqual(
      ["Cast", "Directed by", "Written by"],
    );
    // No picture is served yet: initials hold the face's place, and no image is asked for.
    expect(page.querySelector(".credit .person-face")?.textContent).toBe("AP");
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
    expect(initialsOf("Ada Wexcombe")).toBe("RN");
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
          "GET /people/50": [
            200,
            {
              id: 50,
              name: "Oren Talisk",
              titles: [
                {
                  id: 7,
                  kind: "series",
                  name: "Crime Ledger",
                  year: 2020,
                  jobs: [{ job: "writer" }],
                },
                {
                  id: 10,
                  kind: "movie",
                  name: "Metropolis",
                  year: 1995,
                  jobs: [{ job: "director" }, { job: "cast", character: "A guard" }],
                },
              ],
            },
          ],
        }),
      ),
    );
    const { container } = mounted;
    expect(container.querySelector("h1")?.textContent).toBe("Oren Talisk");
    expect(
      [...container.querySelectorAll(".credited-person__title")].map(
        (link) => `${link.getAttribute("href") ?? ""} ${link.textContent}`,
      ),
    ).toEqual(["/series/7 Crime Ledger2020Writer", "/titles/10 Metropolis1995Director, as A guard"]);
  });
});
