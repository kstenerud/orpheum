// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { CandidateChooser } from "./candidate-chooser.tsx";
import {
  bodyField,
  buttonWithText,
  enter,
  mount,
  servedBy,
  submit,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** The chooser's form whose button says label. */
function formOf(label: string): HTMLFormElement | null | undefined {
  if (mounted === undefined) {
    return undefined;
  }
  return buttonWithText(mounted.container, label)?.closest("form");
}

describe("the candidate chooser, when the work is not among the candidates", () => {
  it("searches for a name and year a person typed, and lists what it found to choose from", async () => {
    const heard: Heard[] = [];
    mounted = await mount(
      withServices(
        <CandidateChooser itemId={7} onChosen={() => undefined} />,
        servedBy(
          {
            "GET /items/7/candidates": [
              [200, { provider: "tmdb", candidates: [] }],
              [
                200,
                { provider: "tmdb", candidates: [{ providerId: "9285", name: "Harbour Lights" }] },
              ],
            ],
          },
          heard,
        ),
      ),
    );
    expect(mounted.container.textContent).toContain("The search found nothing.");
    const form = formOf("Search");
    const [name, year] = [...(form?.querySelectorAll("input") ?? [])];
    await enter(name, "Harbour Lights");
    await enter(year, "1969");
    await submit(form);
    expect(heard.at(-1)?.query).toBe("?name=Harbour Lights&year=1969");
    expect(buttonWithText(mounted.container, "This one")).toBeDefined();
    expect(mounted.container.textContent).toContain("Harbour Lights");
  });

  it("matches the work a pasted page names, and says so beside it when it names none", async () => {
    const heard: Heard[] = [];
    let chosen = 0;
    mounted = await mount(
      withServices(
        <CandidateChooser
          itemId={7}
          onChosen={() => {
            chosen += 1;
          }}
        />,
        servedBy(
          {
            "GET /items/7/candidates": [200, { provider: "tmdb", candidates: [] }],
            "POST /items/7/match-by-link": [
              [
                422,
                {
                  error: {
                    code: "invalid_request",
                    message: "a note for developers",
                    detail: { problems: [{ location: "body.link", message: "no such work" }] },
                  },
                },
              ],
              [204, null],
            ],
          },
          heard,
        ),
      ),
    );
    const form = formOf("Use this match");
    await enter(form?.querySelector("input"), "Harbour Lights");
    await submit(form);
    expect(mounted.container.querySelector('[role="alert"]')?.textContent).toContain(
      "That is not a TMDB or IMDb page or id",
    );
    await enter(form?.querySelector("input"), "https://www.imdb.com/title/tt0456789/");
    await submit(form);
    expect(bodyField(heard.at(-1) ?? { methodAndPath: "", body: undefined }, "link")).toBe(
      "https://www.imdb.com/title/tt0456789/",
    );
    expect(chosen).toBe(1);
  });
});
