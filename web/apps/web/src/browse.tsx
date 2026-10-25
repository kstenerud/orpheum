// SPDX-License-Identifier: MPL-2.0

import "./browse.css";

import type { components } from "@orpheum/sdk";
import { memo, useCallback, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow, unavailableText } from "./problem.ts";
import { ContinueWatching } from "./continue-watching.tsx";
import { StatusTray } from "./status-cues.tsx";
import { PosterCard, PosterDetail } from "./poster-card.tsx";
import { itemRoute } from "./router.ts";
import { EndOfList } from "./end-of-list.tsx";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { useItemsInView } from "./items-in-view.ts";
import { usePages, type Page } from "./use-pages.ts";
import { useResource, type Resource } from "./use-resource.ts";

/** One film or series in a list, as the server sends it. */
export type Entry = components["schemas"]["EntryBody"];

/** Which list: films or series. */
export type BrowseKind = "movie" | "series";

/** The order a list comes in. */
export type BrowseSort = "name" | "added";

/**
 * How many posters a page loads, a best guess: a few screens of the grid, few enough to answer at
 * once; sixty divides into rows of 2, 3, 4, 5, 6 and 10, so a full page ends on a full row.
 */
const entriesPerPage = 60;

/** A grid of films or series, each a poster with its name and year under it. */
export function PosterGrid(props: {
  readonly kind: BrowseKind;
  readonly entries: readonly Entry[];
  /** Watches each poster, by its place, for the list to know which are in view (usePages). */
  readonly watchItemAt?: ((index: number) => (element: Element | null) => void) | undefined;
}): JSX.Element {
  return (
    <ul className="poster-grid" data-focus-container="">
      {props.entries.map((entry, index) => (
        <GridPoster
          key={entry.id}
          kind={props.kind}
          entry={entry}
          watch={props.watchItemAt?.(index)}
        />
      ))}
    </ul>
  );
}

/**
 * One poster with its name, year and cues. A poll that changes one entry draws that one again, as
 * usePages keeps each unchanged entry the very one shown before.
 */
const GridPoster = memo(function GridPoster(props: {
  readonly kind: BrowseKind;
  readonly entry: Entry;
  readonly watch: ((element: Element | null) => void) | undefined;
}): JSX.Element {
  const { entry, kind, watch } = props;
  return <EntryPoster kind={kind} entry={entry} watch={watch} />;
});

/**
 * A film's or series' poster as every list shows it: its page, its year, its status cues, and how
 * far in it is watched when partway.
 */
export function EntryPoster(props: {
  readonly kind: BrowseKind;
  readonly entry: Entry;
  readonly watch?: ((element: Element | null) => void) | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { entry, kind } = props;
  const percent = watchedPercentOf(entry);
  return (
    <PosterCard
      to={itemRoute(kind, entry.id)}
      posterSha256={entry.posterSha256}
      name={entry.name}
      details={<EntryDetails entry={entry} />}
      isDimmed={entry.unavailable !== undefined}
      cues={<StatusTray subject={entry} />}
      progress={
        percent === undefined
          ? undefined
          : { percent, label: t("continueWatching.watched", { percent }) }
      }
      watch={props.watch}
    />
  );
}

/**
 * How far in a film or series is watched, a whole percent, as the server says it: a film by its
 * partway file, a series by its episodes finished of those it has (the owner, 2026-10-25: "For
 * series, the progress bar at the bottom shows how far along they have watched"); none for one not
 * begun.
 */
export function watchedPercentOf(entry: Entry): number | undefined {
  if (entry.watchedPercent !== undefined) {
    return entry.watchedPercent;
  }
  const { episodeCount, finishedEpisodeCount } = entry;
  if (episodeCount === undefined || finishedEpisodeCount === undefined || episodeCount === 0) {
    return undefined;
  }
  return Math.round((finishedEpisodeCount * 100) / episodeCount);
}

/** The lines under a film's or series' name: its year, and that it cannot play now. */
export function EntryDetails(props: { readonly entry: Entry }): JSX.Element {
  const { t } = useTranslation();
  const { entry } = props;
  return (
    <>
      {entry.year === undefined ? null : (
        <PosterDetail>{t("browse.year", { year: entry.year })}</PosterDetail>
      )}
      {entry.unavailable === undefined ? null : (
        <PosterDetail title={unavailableText(t, entry.unavailable)}>
          {t("title.unavailableNow")}
        </PosterDetail>
      )}
    </>
  );
}

/**
 * Films or series, by name or newest first, with no pagination to click through: the next page
 * loads as the end comes into view, or on the button that is always there for it.
 */
export function Browse(props: { readonly kind: BrowseKind }): JSX.Element {
  const { t } = useTranslation();
  const [sort, setSort] = useState<BrowseSort>("name");
  return (
    <section className="browse" aria-labelledby="browse-heading">
      {/* The home screen opens on what the person stopped in the middle of (D188). */}
      {props.kind === "movie" ? <ContinueWatching /> : null}
      <div className="browse__bar">
        <h1 id="browse-heading">{t(props.kind === "movie" ? "browse.films" : "browse.series")}</h1>
        <SortChoice sort={sort} onSort={setSort} />
      </div>
      <div className="browse__layout">
        <div>
          <BrowsePages key={`${props.kind}/${sort}`} kind={props.kind} sort={sort} />
        </div>
        {sort === "name" ? <AlphabetRail /> : null}
      </div>
    </section>
  );
}

/** The letters of the alphabet rail, a list's names' first characters: digits and marks, then A to Z. */
const railLetters = [
  "#",
  ...Array.from({ length: 26 }, (_, index) => String.fromCodePoint(0x41 + index)),
];

/**
 * A list's alphabet beside its grid, by name only, to jump to a letter: a prototype, drawn but not
 * yet working (the owner, 2026-10-25: "It doesn't need to actually work yet, but I want to see how
 * it looks on-screen"). Its letters are buttons that do nothing yet, so a remote's moves into and
 * out of it can be tried (the owner, 2026-10-25), a focus container of their own.
 */
function AlphabetRail(): JSX.Element {
  const { t } = useTranslation();
  return (
    <ol className="alphabet-rail" aria-label={t("browse.alphabet")} data-focus-container="">
      {railLetters.map((letter) => (
        <li key={letter}>
          <button type="button" className="alphabet-rail__letter">
            {letter}
          </button>
        </li>
      ))}
    </ol>
  );
}

/** The order to list in. */
export function SortChoice(props: {
  readonly sort: BrowseSort;
  readonly onSort: (sort: BrowseSort) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <label>
      {t("browse.sort")}
      <select
        value={props.sort}
        onChange={(event) => {
          props.onSort(event.target.value === "added" ? "added" : "name");
        }}
      >
        <option value="name">{t("browse.byName")}</option>
        <option value="added">{t("browse.byAdded")}</option>
      </select>
    </label>
  );
}

/** An entry's id, by which a list shows an entry read twice once. */
const entryId = (entry: Entry): number => entry.id;

/** One list in one order, read a page at a time. */
function BrowsePages(props: { readonly kind: BrowseKind; readonly sort: BrowseSort }): JSX.Element {
  const { api } = useServices();
  const { kind, sort } = props;
  const loadPage = useCallback(
    async (cursor: string): Promise<Page<Entry>> => {
      const query = { kind, sort, limit: entriesPerPage, ...(cursor === "" ? {} : { cursor }) };
      const titlesPage = dataOrThrow(await api.GET("/titles", { params: { query } }));
      return { items: titlesPage.titles ?? [], nextCursor: titlesPage.nextCursor ?? "" };
    },
    [api, kind, sort],
  );
  const pages = usePages(loadPage, useBackgroundChangesPoller(), entryId);
  const watchItemAt = useItemsInView(pages.showRange);
  return (
    <BrowseView
      kind={kind}
      loadedEntries={pages.loadedSoFar}
      isLoadingMore={pages.isLoadingMore}
      onLoadMore={pages.loadMore}
      watchItemAt={watchItemAt}
    />
  );
}

/** A list's pages as they show: loading, failed with why, empty, or the grid and a way to more. */
export function BrowseView(props: {
  readonly kind: BrowseKind;
  readonly loadedEntries: Resource<Page<Entry>>;
  readonly isLoadingMore: boolean;
  readonly onLoadMore: () => void;
  readonly watchItemAt?: ((index: number) => (element: Element | null) => void) | undefined;
}): JSX.Element {
  return (
    <Loaded
      resource={props.loadedEntries}
      render={(page) =>
        page.items.length === 0 ? (
          <EmptyList />
        ) : (
          <>
            <PosterGrid kind={props.kind} entries={page.items} watchItemAt={props.watchItemAt} />
            {page.nextCursor === "" ? null : (
              <EndOfList onMore={props.onLoadMore} isLoadingMore={props.isLoadingMore} />
            )}
          </>
        )
      }
    />
  );
}

/**
 * Why a list is empty: nothing scanned yet, or, for someone whose role the admin has given no
 * library, that they have been given none (D243 §4), which only the admin can change.
 */
function EmptyList(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [me] = useResource("me", async () => dataOrThrow(await api.GET("/me")));
  const isGivenNoLibrary = me.state === "loaded" && me.value.hasNoLibrary;
  return (
    <p className="browse__empty">{t(isGivenNoLibrary ? "browse.noLibrary" : "browse.empty")}</p>
  );
}
