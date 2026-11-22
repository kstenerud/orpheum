// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { Libraries } from "./libraries.tsx";
import { LibraryPage } from "./library-manage.tsx";
import { currentNotice } from "./notice.ts";
import { SeriesView, type SeriesDetail } from "./series.tsx";
import { Title, type TitleDetail } from "./title.tsx";
import {
  buttonNamed,
  buttonWithText,
  click,
  mount,
  servedBy,
  settleUntil,
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

const others = {
  id: 3,
  name: "Home videos",
  kind: "other",
  metadataSource: "none",
  changeDetection: "manual",
  isWritable: false,
  createdAt: 1,
  roots: [],
} as const;

/** The rescans posted, each with its query, so that none carries a kind (D535 §5). */
const rescansPosted = (heard: readonly Heard[]): string[] =>
  heard
    .filter((each) => each.methodAndPath.startsWith("POST"))
    .map((each) => each.methodAndPath + (each.query ?? ""));

it("rescans every library, or one of any kind, in full, and offers no other rescan (D535 §4, §5)", async () => {
  const heard: Heard[] = [];
  const services = servedBy(
    {
      "GET /libraries": [
        [200, { libraries: [others] }],
        [
          200,
          {
            libraries: [{ ...others, rescan: { isScanning: true, itemCount: 0, matchedCount: 0 } }],
          },
        ],
      ],
      "GET /nodes": [200, { nodes: [] }],
      "POST /libraries/rescan": [202, { startedCount: 1 }],
      "POST /libraries/3/rescan": [
        [202, { isStarted: true }],
        [202, { isStarted: false }],
      ],
    },
    heard,
  );
  mounted = await mount(withServices(<Libraries />, services));
  expect(mounted.container.textContent).not.toContain("Full rescan");
  await click(buttonWithText(mounted.container, "Rescan everything"));
  await click(buttonWithText(mounted.container, "Start the rescan"));
  expect(mounted.container.textContent).toContain("Rescanning 1 libraries.");
  expect(mounted.container.textContent).toContain("Rescanning… scanning first.");
  // One library's own rescan is on its own page (D361 §2), a library of other videos' too.
  mounted.unmount();
  mounted = await mount(withServices(<LibraryPage libraryId={3} />, services));
  const page = mounted.container;
  expect(buttonWithText(page, "Scan now")).toBeDefined();
  expect(page.textContent).not.toContain("Full rescan");
  await click(buttonWithText(page, "Rescan"));
  await click(buttonWithText(page, "Start the rescan"));
  expect(page.textContent).toContain("Rescanning: every file in it is read again");
  await click(buttonWithText(page, "Rescan"));
  await click(buttonWithText(page, "Start the rescan"));
  expect(page.textContent).toContain("A rescan of it is already running.");
  expect(rescansPosted(heard)).toEqual([
    "POST /libraries/rescan",
    "POST /libraries/3/rescan",
    "POST /libraries/3/rescan",
  ]);
});

it("rescans a series from its page's menu, for an admin alone, and not from its dialog (D535 §3, §5)", async () => {
  const heard: Heard[] = [];
  const services = servedBy(
    {
      "POST /items/12/rescan": [
        [202, { isStarted: true }],
        [202, { isStarted: false }],
        [500, { title: "Internal Server Error", status: 500 }],
      ],
    },
    heard,
  );
  const series: SeriesDetail = {
    id: 12,
    kind: "series",
    name: "The Ledger Room",
    art: {},
    seasons: [],
  };
  let readAgain = 0;
  const corrected = (): void => {
    readAgain += 1;
  };
  mounted = await mount(
    withServices(<SeriesView series={series} onMatchCorrected={corrected} />, services),
  );
  // Nothing rarely used for anyone else: no "…" at all.
  expect(buttonNamed(mounted.container, "More actions")).toBeUndefined();
  mounted.unmount();
  const asAdmin = (): Promise<Mounted> =>
    mount(
      withServices(
        <IsAdminContext value={true}>
          <SeriesView series={series} onMatchCorrected={corrected} />
        </IsAdminContext>,
        services,
      ),
    );
  mounted = await asAdmin();
  const page = mounted.container;
  await click(buttonNamed(page, "More actions"));
  await click(buttonWithText(page, "Identified incorrectly"));
  expect(buttonWithText(page, "Rescan")).toBeUndefined();
  mounted.unmount();
  const rescanFromTheMenu = async (): Promise<HTMLElement> => {
    mounted?.unmount();
    mounted = await asAdmin();
    const opened = mounted.container;
    await click(buttonNamed(opened, "More actions"));
    await click(buttonWithText(opened, "Rescan"));
    await click(buttonWithText(opened, "Start the rescan"));
    return opened;
  };
  // Started, or one already running, the page is read again, to say it is being rescanned, and no
  // notice comes after the press (D535 §7); one that could not start is told in the notice.
  await rescanFromTheMenu();
  await settleUntil(() => readAgain === 1);
  await rescanFromTheMenu();
  await settleUntil(() => readAgain === 2);
  expect(currentNotice()).toBeUndefined();
  await rescanFromTheMenu();
  await settleUntil(() => currentNotice() !== undefined);
  expect(readAgain).toBe(2);
  expect(rescansPosted(heard)).toEqual([
    "POST /items/12/rescan",
    "POST /items/12/rescan",
    "POST /items/12/rescan",
  ]);
});

it("offers an admin Rescan in an other video's menu, and none on an extra's page (D535 §3)", async () => {
  const video: TitleDetail = {
    id: 14,
    kind: "other",
    name: "Birthday",
    identificationState: "identified",
    art: {},
    extras: [],
    versions: [{ id: 1, files: [{ id: 1, path: "/media/a.mp4", streams: [] }] }],
  };
  const heard: Heard[] = [];
  const services = servedBy(
    {
      "GET /items/14": [
        [200, video],
        [200, { ...video, isRescanning: true }],
      ],
      "GET /items/15": [200, { ...video, id: 15, kind: "extra" }],
      "POST /items/14/rescan": [202, { isStarted: true }],
      "POST /playback": [
        200,
        { delivery: "direct", url: "/u", notDirectReasons: [], mediaToken: "a" },
      ],
      "PUT /interest/page": [204, null],
      "DELETE /interest/page": [204, null],
    },
    heard,
  );
  const asAdmin = (id: number): React.JSX.Element => (
    <IsAdminContext value={true}>
      <Title id={id} />
    </IsAdminContext>
  );
  mounted = await mount(withServices(asAdmin(14), services));
  const page = mounted.container;
  await settleUntil(() => buttonNamed(page, "More actions") !== undefined);
  await click(buttonNamed(page, "More actions"));
  await click(buttonWithText(page, "Rescan"));
  expect(page.textContent).not.toContain("Rescanning…");
  await click(buttonWithText(page, "Start the rescan"));
  // The page, read again, says in its facts that it is being rescanned (D535 §7).
  await settleUntil(() => page.textContent.includes("Rescanning…"));
  expect(rescansPosted(heard)).toContain("POST /items/14/rescan");
  mounted.unmount();
  mounted = await mount(withServices(asAdmin(15), services));
  const extra = mounted.container;
  await settleUntil(() => extra.textContent.includes("Birthday"));
  expect(buttonNamed(extra, "More actions")).toBeUndefined();
});

/** The box the rescan question offers, to throw out what was made (D535 §6). */
const discardBox = (container: HTMLElement): HTMLInputElement | undefined =>
  [...container.querySelectorAll("label")]
    .find((label) => label.textContent.includes("Discard cached data (more expensive)"))
    ?.querySelector("input") ?? undefined;

it("asks before a rescan, sends nothing on Cancel, and discards what was made when ticked (D535 §6)", async () => {
  const heard: Heard[] = [];
  const services = servedBy(
    {
      "GET /libraries": [200, { libraries: [others] }],
      "GET /nodes": [200, { nodes: [] }],
      "POST /libraries/rescan": [202, { startedCount: 1 }],
      "POST /libraries/3/scan": [202, { askedRootCount: 0 }],
      "POST /libraries/3/rescan": [202, { isStarted: true }],
    },
    heard,
  );
  mounted = await mount(withServices(<LibraryPage libraryId={3} />, services));
  const page = mounted.container;
  // Scan now starts at once: no question.
  await click(buttonWithText(page, "Scan now"));
  expect(buttonWithText(page, "Start the rescan")).toBeUndefined();
  expect(discardBox(page)).toBeUndefined();
  // Rescan asks, sending nothing until Start the rescan, and nothing at all on Cancel.
  await click(buttonWithText(page, "Rescan"));
  expect(page.textContent).toContain(
    "Do a rescan?This is an expensive operation that can take hours.",
  );
  expect(discardBox(page)?.checked).toBe(false);
  expect(rescansPosted(heard).filter((each) => each.includes("rescan"))).toEqual([]);
  await click(buttonWithText(page, "Cancel"));
  expect(buttonWithText(page, "Start the rescan")).toBeUndefined();
  // Start the rescan unticked sends a rescan that keeps what was made; ticked, one that discards it.
  await click(buttonWithText(page, "Rescan"));
  await click(buttonWithText(page, "Start the rescan"));
  await click(buttonWithText(page, "Rescan"));
  await click(discardBox(page));
  expect(discardBox(page)?.checked).toBe(true);
  await click(buttonWithText(page, "Start the rescan"));
  mounted.unmount();
  mounted = await mount(withServices(<Libraries />, services));
  await click(buttonWithText(mounted.container, "Rescan everything"));
  expect(mounted.container.textContent).toContain("can take hours");
  await click(discardBox(mounted.container));
  await click(buttonWithText(mounted.container, "Start the rescan"));
  expect(rescansPosted(heard).filter((each) => each.includes("rescan"))).toEqual([
    "POST /libraries/3/rescan",
    "POST /libraries/3/rescan?discard=true",
    "POST /libraries/rescan?discard=true",
  ]);
});

it("asks before a video's rescan, without hours, and discards what was made when ticked (D535 §6)", async () => {
  const heard: Heard[] = [];
  const services = servedBy({ "POST /items/12/rescan": [202, { isStarted: true }] }, heard);
  const series: SeriesDetail = {
    id: 12,
    kind: "series",
    name: "The Ledger Room",
    art: {},
    seasons: [],
  };
  const asAdmin = (): Promise<Mounted> =>
    mount(
      withServices(
        <IsAdminContext value={true}>
          <SeriesView series={series} onMatchCorrected={() => undefined} />
        </IsAdminContext>,
        services,
      ),
    );
  mounted = await asAdmin();
  let page = mounted.container;
  await click(buttonNamed(page, "More actions"));
  await click(buttonWithText(page, "Rescan"));
  expect(page.textContent).toContain("Do a rescan?This is an expensive operation.");
  expect(page.textContent).not.toContain("hours");
  await click(buttonWithText(page, "Cancel"));
  expect(rescansPosted(heard)).toEqual([]);
  mounted.unmount();
  mounted = await asAdmin();
  page = mounted.container;
  await click(buttonNamed(page, "More actions"));
  await click(buttonWithText(page, "Rescan"));
  await click(discardBox(page));
  await click(buttonWithText(page, "Start the rescan"));
  // Start the rescan closes the question at once.
  expect(page.querySelector("dialog")).toBeNull();
  expect(page.textContent).not.toContain("Do a rescan?");
  await settleUntil(() => rescansPosted(heard).length === 1);
  expect(rescansPosted(heard)).toEqual(["POST /items/12/rescan?discard=true"]);
});
