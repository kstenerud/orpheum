// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { JSX } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { createNavigation } from "./navigation.ts";
import { Series, type SeriesDetail } from "./series.tsx";
import { click, mount, servedBy, settleUntil, type Answers, type Mounted } from "./test-dom.tsx";
import { fakePlace, withServices, type FakePlace } from "./test-services.tsx";
import { Title, type TitleDetail } from "./title.tsx";

const film: TitleDetail = {
  id: 10,
  kind: "movie",
  name: "Metropolis",
  identificationState: "identified",
  art: {},
  versions: [],
  extras: [],
  libraries: [
    { id: 3, name: "Films" },
    { id: 4, name: "Kids" },
  ],
};

const episode: TitleDetail = {
  ...film,
  id: 12,
  kind: "episode",
  name: "Pilot",
  episode: { seriesId: 5, seriesName: "Long Run", seasonNumber: 2, episodeNumber: 1 },
  libraries: [],
};

const extra: TitleDetail = {
  ...film,
  id: 14,
  kind: "extra",
  name: "Making Metropolis",
  filmOfExtra: { id: 10, name: "Metropolis" },
};

const series: SeriesDetail = {
  id: 5,
  kind: "series",
  name: "Long Run",
  art: {},
  seasons: [1, 2].map((seasonNumber) => ({ seasonNumber, episodeCount: 1 })),
  libraries: [{ id: 7, name: "Shows" }],
};

/** The stand-in server's answers for a title's or a series' page. */
function pageAnswers(detail: TitleDetail | SeriesDetail): Answers {
  const id = String(detail.id);
  const path = detail.kind === "series" ? `/collections/${id}` : `/items/${id}`;
  return {
    [`GET ${path}`]: [200, detail],
    "GET /collections/5/seasons/1": [200, { episodes: [] }],
    "GET /collections/5/seasons/2": [200, { episodes: [] }],
    "PUT /interest/page": [204, null],
    "DELETE /interest/page": [204, null],
  };
}

let shown: Mounted | undefined;

afterEach(() => {
  shown?.unmount();
  shown = undefined;
  vi.restoreAllMocks();
});

/** Shows page at place's address, with answers, once its trail is drawn. */
async function showAt(place: FakePlace, answers: Answers, page: JSX.Element): Promise<HTMLElement> {
  const services = { ...servedBy(answers), navigation: createNavigation(place) };
  shown = await mount(withServices(page, services));
  const { container } = shown;
  await settleUntil(
    () => container.querySelector(".up-trail li[aria-current]")?.textContent !== "",
  );
  return container;
}

/** The trail's words, its parts joined as they read. */
function trailOf(container: HTMLElement): string {
  return [...container.querySelectorAll(".up-trail li")]
    .map((each) => each.textContent)
    .join(" › ");
}

/** The trail's link to the ancestor named label. */
function crumb(container: HTMLElement, label: string): HTMLAnchorElement | undefined {
  return [...container.querySelectorAll<HTMLAnchorElement>(".up-trail a")].find(
    (link) => link.textContent === label,
  );
}

describe("a title's trail (D367 §8)", () => {
  it("shows where an episode is, each part a link, its series the parent, and no Back", async () => {
    const page = await showAt(fakePlace("/titles/12"), pageAnswers(episode), <Title id={12} />);
    expect(trailOf(page)).toBe("Series › Long Run › S2E1 Pilot");
    expect(crumb(page, "Long Run")?.getAttribute("href")).toBe("/series/5?season=2");
    expect(page.querySelector(".back-button")).toBeNull();
  });

  it("goes back to the series behind it, however far, adding no entry (§7)", async () => {
    const place = fakePlace("/series/5?season=2");
    const navigation = createNavigation(place);
    navigation.go({ screen: "title", itemId: 13 });
    navigation.go({ screen: "title", itemId: 12 });
    const page = await showAt(place, pageAnswers(episode), <Title id={12} />);
    await click(crumb(page, "Long Run"));
    expect(place.location.pathname + place.location.search).toBe("/series/5?season=2");
    // Back after Up never returns down: the entries Up passed over are ahead, not behind.
    expect(createNavigation(place).behind()).toEqual([]);
  });

  it("goes up from an episode opened from outside to its series at its season, in its place", async () => {
    const place = fakePlace("/titles/12");
    const page = await showAt(place, pageAnswers(episode), <Title id={12} />);
    await click(crumb(page, "Long Run"));
    expect(place.addresses).toEqual(["/series/5?season=2"]);
  });

  it("goes up from a film opened from outside to the first library listing it, by name (§3)", async () => {
    const place = fakePlace("/titles/10");
    const page = await showAt(place, pageAnswers(film), <Title id={10} />);
    expect(trailOf(page)).toBe("Films › Metropolis");
    await click(crumb(page, "Films"));
    expect(place.addresses).toEqual(["/browse?library=3"]);
  });

  it("goes back up to the library the person came through, as they left it (§3)", async () => {
    const place = fakePlace("/browse?library=4&sort=added");
    createNavigation(place).go({ screen: "title", itemId: 10 });
    const page = await showAt(place, pageAnswers(film), <Title id={10} />);
    expect(trailOf(page)).toBe("Kids › Metropolis");
    await click(crumb(page, "Kids"));
    expect(place.location.search).toBe("?library=4&sort=added");
    expect(place.addresses).toHaveLength(2);
  });

  it("puts an extra under its film (§2)", async () => {
    const place = fakePlace("/titles/14");
    const page = await showAt(place, pageAnswers(extra), <Title id={14} />);
    expect(trailOf(page)).toBe("Films › Metropolis › Making Metropolis");
    await click(crumb(page, "Metropolis"));
    expect(place.addresses).toEqual(["/titles/10"]);
  });

  it("is there, and goes up, while its title cannot be read", async () => {
    const place = fakePlace("/titles/10");
    const services = {
      ...servedBy({
        ...pageAnswers(film),
        "GET /items/10": [404, { error: { code: "not_found" } }],
      }),
      navigation: createNavigation(place),
    };
    shown = await mount(withServices(<Title id={10} />, services));
    await click(crumb(shown.container, "Films"));
    expect(place.addresses).toEqual(["/films"]);
  });

  it("is one link up on a television, one stop for focus", async () => {
    vi.spyOn(Navigator.prototype, "userAgent", "get").mockReturnValue(
      "Mozilla/5.0 (Linux; Android 9; AFTSSS) AppleWebKit/537.36 (KHTML, like Gecko) Silk/138.19.9 like Chrome/138.0.7204.244 Safari/537.36",
    );
    const place = fakePlace("/titles/12");
    const services = { ...servedBy(pageAnswers(episode)), navigation: createNavigation(place) };
    shown = await mount(withServices(<Title id={12} />, services));
    const { container } = shown;
    await settleUntil(() => container.querySelector(".up-trail__up")?.textContent === "‹ Long Run");
    expect(container.querySelectorAll(".up-trail a")).toHaveLength(1);
  });
});

describe("a series' trail", () => {
  it("goes up from a series opened from outside to the first library listing it", async () => {
    const place = fakePlace("/series/5?season=2");
    const page = await showAt(place, pageAnswers(series), <Series id={5} season={2} />);
    expect(trailOf(page)).toBe("Shows › Long Run");
    await click(crumb(page, "Shows"));
    expect(place.addresses).toEqual(["/browse?library=7"]);
  });

  it("goes back to Series behind it, the list the person came through", async () => {
    const place = fakePlace("/series");
    createNavigation(place).go({ screen: "series-page", collectionId: 5 });
    const page = await showAt(place, pageAnswers(series), <Series id={5} />);
    expect(trailOf(page)).toBe("Series › Long Run");
    await click(crumb(page, "Series"));
    expect(place.location.pathname).toBe("/series");
  });
});
