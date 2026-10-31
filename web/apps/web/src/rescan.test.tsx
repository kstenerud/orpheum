// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { Libraries } from "./libraries.tsx";
import { LibraryPage } from "./library-manage.tsx";
import { SeriesView, type SeriesDetail } from "./series.tsx";
import {
  buttonNamed,
  buttonWithText,
  click,
  mount,
  servedBy,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { IsAdminContext } from "./warning-sign.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const films = {
  id: 3,
  name: "Films",
  kind: "movies",
  metadataSource: "tmdb",
  changeDetection: "manual",
  isWritable: false,
  createdAt: 1,
  roots: [],
} as const;

const posts = (heard: readonly Heard[]): string[] =>
  heard.filter((each) => each.methodAndPath.startsWith("POST")).map((each) => each.methodAndPath);

it("rescans every library, or one, from the Libraries page, and says what started (D357 §1, §5)", async () => {
  const heard: Heard[] = [];
  const services = servedBy(
    {
      "GET /libraries": [
        [200, { libraries: [films] }],
        [
          200,
          {
            libraries: [{ ...films, rescan: { isScanning: true, itemCount: 0, matchedCount: 0 } }],
          },
        ],
      ],
      "GET /nodes": [200, { nodes: [] }],
      "POST /libraries/rescan": [202, { startedCount: 1 }],
      "POST /libraries/3/rescan": [202, { isStarted: false }],
    },
    heard,
  );
  mounted = await mount(withServices(<Libraries />, services));
  await click(buttonWithText(mounted.container, "Rescan everything"));
  expect(mounted.container.textContent).toContain("Rescanning 1 libraries of films or series.");
  expect(mounted.container.textContent).toContain("Rescanning… scanning first.");
  // One library's own rescan is on its own page (D361 §2).
  mounted.unmount();
  mounted = await mount(withServices(<LibraryPage libraryId={3} />, services));
  const page = mounted.container;
  await click(buttonWithText(page, "Rescan"));
  expect(page.textContent).toContain("A rescan of it is already running.");
  expect(posts(heard)).toEqual(["POST /libraries/rescan", "POST /libraries/3/rescan"]);
});

it("rescans one film or series from its page, for an admin alone (D357 §1)", async () => {
  const heard: Heard[] = [];
  const services = servedBy(
    {
      "POST /items/12/rescan": [
        [202, { isStarted: true }],
        [202, { isStarted: false }],
      ],
    },
    heard,
  );
  const series: SeriesDetail = { id: 12, kind: "series", name: "The Ledger Room", art: {}, seasons: [] };
  const corrected = (): void => undefined;
  mounted = await mount(
    withServices(<SeriesView series={series} onMatchCorrected={corrected} />, services),
  );
  // Nothing rarely used for anyone else: no "…" at all.
  expect(buttonNamed(mounted.container, "More actions")).toBeUndefined();
  mounted.unmount();
  mounted = await mount(
    withServices(
      <IsAdminContext value={true}>
        <SeriesView series={series} onMatchCorrected={corrected} />
      </IsAdminContext>,
      services,
    ),
  );
  const page = mounted.container;
  await click(buttonNamed(page, "More actions"));
  await click(buttonWithText(page, "Identified incorrectly"));
  await click(buttonWithText(page, "Rescan"));
  expect(page.textContent).toContain("its files are scanned, then it is matched again from them.");
  await click(buttonWithText(page, "Rescan"));
  expect(page.textContent).toContain("A rescan of it, or of its library, is already running.");
  expect(posts(heard)).toEqual(["POST /items/12/rescan", "POST /items/12/rescan"]);
});

/** The rescans posted, each with its query: "?full=true" for a full one. */
const rescansPosted = (heard: readonly Heard[]): string[] =>
  heard
    .filter((each) => each.methodAndPath.startsWith("POST"))
    .map((each) => each.methodAndPath + (each.query ?? ""));

it("offers a full rescan beside each rescan, everything's, a library's and a film's, and says what started (D392 §1)", async () => {
  const heard: Heard[] = [];
  const services = servedBy(
    {
      "GET /libraries": [200, { libraries: [films] }],
      "GET /nodes": [200, { nodes: [] }],
      "POST /libraries/rescan": [202, { startedCount: 1 }],
      "POST /libraries/3/rescan": [
        [202, { isStarted: true }],
        [202, { isStarted: false }],
      ],
      "POST /items/12/rescan": [202, { isStarted: true }],
    },
    heard,
  );
  mounted = await mount(withServices(<Libraries />, services));
  await click(buttonWithText(mounted.container, "Full rescan of everything"));
  expect(mounted.container.textContent).toContain("Rescanning 1 libraries of films or series.");
  mounted.unmount();
  mounted = await mount(withServices(<LibraryPage libraryId={3} />, services));
  await click(buttonWithText(mounted.container, "Full rescan"));
  expect(mounted.container.textContent).toContain(
    "Full rescan: every file in it is read again, then each film and series in it is matched again.",
  );
  await click(buttonWithText(mounted.container, "Full rescan"));
  expect(mounted.container.textContent).toContain("A rescan of it is already running.");
  mounted.unmount();
  const series: SeriesDetail = { id: 12, kind: "series", name: "The Ledger Room", art: {}, seasons: [] };
  mounted = await mount(
    withServices(
      <IsAdminContext value={true}>
        <SeriesView series={series} onMatchCorrected={() => undefined} />
      </IsAdminContext>,
      services,
    ),
  );
  const page = mounted.container;
  await click(buttonNamed(page, "More actions"));
  await click(buttonWithText(page, "Identified incorrectly"));
  await click(buttonWithText(page, "Full rescan"));
  expect(page.textContent).toContain(
    "Full rescan: its files are read again, then it is matched again from them.",
  );
  expect(rescansPosted(heard)).toEqual([
    "POST /libraries/rescan?full=true",
    "POST /libraries/3/rescan?full=true",
    "POST /libraries/3/rescan?full=true",
    "POST /items/12/rescan?full=true",
  ]);
});
