// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { describe, expect, it } from "vitest";

import { CreateLibraryPage } from "./create-library.tsx";
import { LibraryOffer } from "./library-offer.tsx";
import { guessKind, holdsTwoKinds, offerRows, stepAfter } from "./library-offer-kinds.ts";
import { createNavigation } from "./navigation.ts";
import { parseRoute, welcomeSteps } from "./router.ts";
import type { Services } from "./services.tsx";
import { buttonWithText, click, mount, refusal, servedBy, type Answers } from "./test-dom.tsx";
import { fakePlace, withServices } from "./test-services.tsx";
import { Welcome } from "./welcome.tsx";

const ignore = (): void => undefined;

/** A server with one connected node holding one media folder with these folders in it. */
function houseServing(folders: readonly string[], more: Answers = {}): Answers {
  return {
    "GET /nodes": [200, { nodes: [{ name: "local", isConnected: true, rootPaths: ["/media"] }] }],
    "GET /nodes/local/folders": [200, { path: "/media", state: "readable", folders }],
    ...more,
  };
}

/** The services opened at address, the server answering answers. */
function openedAt(address: string, answers: Answers): Services {
  return { ...servedBy(answers), navigation: createNavigation(fakePlace(address)) };
}

/** The one setting the whole-folder form reads: how often a new library is scanned. */
const libraryDefaults: Answers[string] = [
  200,
  {
    settings: [
      {
        key: "newLibraryDefaultScanEveryHours",
        kind: "integer",
        value: 24,
        startingValue: 24,
        isModified: false,
        isImportant: false,
        section: "libraries",
        description: "",
        setAt: 1,
      },
    ],
  },
];

describe("the offer of a library for each folder (D527 §3)", () => {
  it("guesses a kind from a folder's name without regard to case, and leaves the rest unticked", () => {
    expect(["Movies", "FILMS", "film"].map(guessKind)).toEqual(["film", "film", "film"]);
    expect(["TV", "tv shows", "Shows", "SERIES"].map(guessKind)).toEqual([
      "series",
      "series",
      "series",
      "series",
    ]);
    const rows = offerRows("local", "/media", ["Movies", "Holiday videos"]);
    expect(rows.map((row) => [row.path, row.kind, row.isTicked])).toEqual([
      ["/media/Movies", "film", true],
      ["/media/Holiday videos", "other", false],
    ]);
  });

  it("warns about taking the whole folder only when its folders hold films and series", () => {
    expect(holdsTwoKinds(offerRows("local", "/media", ["Movies", "TV"]))).toBe(true);
    expect(holdsTwoKinds(offerRows("local", "/media", ["Movies", "Films"]))).toBe(false);
    expect(holdsTwoKinds(offerRows("local", "/media", ["Movies", "Other"]))).toBe(false);
  });

  it("shows the offer with the warning and the folders' kinds", async () => {
    const mounted = await mount(
      withServices(
        <LibraryOffer onMade={ignore} />,
        servedBy(houseServing(["Movies", "TV Shows", "Music"])),
      ),
    );
    const text = mounted.container.textContent;
    expect(text).toContain("Your media folder holds these folders. Make a library of each?");
    expect(text).toContain("/media holds both film and series folders");
    const ticks = [...mounted.container.querySelectorAll<HTMLInputElement>("input[type=checkbox]")];
    expect(ticks.map((tick) => tick.checked)).toEqual([true, true, false]);
    mounted.unmount();
  });

  it("makes each ticked library, and names the row the server refused", async () => {
    let made = 0;
    const mounted = await mount(
      withServices(
        <LibraryOffer
          onMade={() => {
            made += 1;
          }}
        />,
        servedBy(
          houseServing(["Movies", "TV Shows"], {
            "POST /libraries": [
              [201, { id: 1, name: "Movies", kind: "film" }],
              refusal(409, "conflict"),
            ],
          }),
        ),
      ),
    );
    await click(buttonWithText(mounted.container, "Make a library of each ticked folder"));
    const alert = mounted.container.querySelector('[role="alert"]')?.textContent ?? "";
    expect(alert).toContain("“TV Shows” was not made");
    expect(mounted.container.textContent).toContain("Made");
    expect(made).toBe(0);
    mounted.unmount();
  });
});

describe("the New library page (D527 §3)", () => {
  it("opens on the offer of a library for each folder", async () => {
    const mounted = await mount(
      withServices(
        <CreateLibraryPage />,
        servedBy(
          houseServing(["Movies", "TV Shows"], {
            "GET /config": libraryDefaults,
          }),
        ),
      ),
    );
    const text = mounted.container.textContent;
    expect(text).toContain("Your media folder holds these folders. Make a library of each?");
    mounted.unmount();
  });
});

describe("the first-time pages (D527 §3)", () => {
  it("go in order: libraries, discovery, then home", () => {
    expect(stepAfter(undefined)).toEqual({ screen: "welcome", step: "discovery" });
    expect(stepAfter("discovery")).toEqual({ screen: "home" });
  });

  it("go from the libraries page to discovery, in its place, when it is skipped", async () => {
    const services = openedAt(
      "/welcome",
      houseServing(["Movies"], { "GET /config": libraryDefaults }),
    );
    const mounted = await mount(withServices(<Welcome step={undefined} />, services));
    await click(buttonWithText(mounted.container, "Skip for now"));
    expect(services.navigation.route()).toEqual({ screen: "welcome", step: "discovery" });
    mounted.unmount();
  });

  it("have no posters page: its old address is not a welcome step", () => {
    expect(welcomeSteps).toEqual(["discovery"]);
    expect(parseRoute("/welcome/posters", "")).toEqual({ screen: "not-found" });
  });

  it("end on discovery, which says the server works without one and links to System", async () => {
    const mounted = await mount(withServices(<Welcome step="discovery" />, servedBy({})));
    const text = mounted.container.textContent;
    expect(text).toContain("works fully at home without one");
    expect(text).not.toMatch(/realm/iu);
    expect(mounted.container.querySelector('a[href="/system"]')?.textContent).toBe(
      "Connect later from System",
    );
    mounted.unmount();
  });
});
