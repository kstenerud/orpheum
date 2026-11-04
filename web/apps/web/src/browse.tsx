// SPDX-License-Identifier: MPL-2.0

import "./browse.css";

import type { components } from "@orpheum/sdk";
import { memo, useCallback, useMemo, useRef, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import {
  AlphabetRail,
  useAlphabetRailLetters,
  useLetterJump,
  useLetterJumpState,
  useResetJumpOnChange,
  type LetterJump,
} from "./alphabet-rail.tsx";
import { arrivalMark } from "./focus-navigation.ts";
import { dateText } from "./date-time-text.ts";
import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow, unavailableText } from "./problem.ts";
import { StatusTray } from "./status-cues.tsx";
import { PosterCard, PosterDetail } from "./poster-card.tsx";
import { Still } from "./poster.tsx";
import { itemRoute, routeHref, type ListFilters, type ListState } from "./router.ts";
import { EndOfList } from "./end-of-list.tsx";
import { useServices } from "./services.tsx";
import { orderOf, orderQuery, type SortChoice } from "./sort-choices.ts";
import {
  chipsOf,
  criteriaKey,
  filtersQuery,
  hasFilters,
  type ListCriteria,
} from "./list-filters.ts";
import {
  ListCriteriaButton,
  ListingTools,
  useListCriteria,
  type ShownCriteria,
} from "./listing-tools.tsx";
import { Loaded } from "./loaded.tsx";
import { useListLoadsToFocusNoted } from "./input-layer.tsx";
import { useItemsInView } from "./items-in-view.ts";
import { useTilesInView } from "./tile-interest.ts";
import { usePages, type Page } from "./use-pages.ts";
import { useResource, type Resource } from "./use-resource.ts";

/** One film, series or other video in a list, as the server sends it. */
export type Entry = components["schemas"]["EntryBody"];

/** Which list: films or series. */
export type BrowseKind = "movie" | "series";

/** The sorts Films and Series offer, the first their default (D366 §1). */
const browseSorts = ["name", "released", "added"] as const;

/**
 * How many posters a page loads, a best guess: a few screens of the grid, few enough to answer at
 * once; sixty divides into rows of 2, 3, 4, 5, 6 and 10, so a full page ends on a full row.
 */
const entriesPerPage = 60;

/**
 * A grid of films, series or other videos, each a poster or a 16:9 tile with its name and year or
 * date under it. kind, given, is every entry's (Films, Series, a home page row); left out, each
 * entry's own says it, for a list a library's kind does not fix (the Libraries page, D349 §9.1).
 */
export function PosterGrid(props: {
  readonly kind?: BrowseKind | undefined;
  readonly entries: readonly Entry[];
  /** Watches each poster, by its place, for the list to know which are in view (usePages). */
  readonly watchItemAt?: ((index: number) => (element: Element | null) => void) | undefined;
}): JSX.Element {
  // The tiles in view say so, which raises the work their pictures need (D362 §4).
  const watchAt = useTilesInView(
    useMemo(() => props.entries.map(entryId), [props.entries]),
    props.watchItemAt,
  );
  return (
    <ul className="poster-grid" data-focus-container="" {...arrivalMark}>
      {props.entries.map((entry, index) => (
        <GridPoster key={entry.id} kind={props.kind} entry={entry} watch={watchAt(index)} />
      ))}
    </ul>
  );
}

/**
 * One poster with its name, year and cues. A poll that changes one entry draws that one again, as
 * usePages keeps each unchanged entry the very one shown before.
 */
const GridPoster = memo(function GridPoster(props: {
  readonly kind: BrowseKind | undefined;
  readonly entry: Entry;
  readonly watch: ((element: Element | null) => void) | undefined;
}): JSX.Element {
  const { entry, kind, watch } = props;
  return <EntryPoster kind={kind} entry={entry} watch={watch} />;
});

/**
 * A film's, a series' or an other video's poster as every list shows it: its page, its year or
 * date, its status cues, and how far in it is watched when partway. kind, left out, is the entry's
 * own.
 */
export function EntryPoster(props: {
  readonly kind?: BrowseKind | undefined;
  readonly entry: Entry;
  readonly watch?: ((element: Element | null) => void) | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { entry } = props;
  const kind = props.kind ?? entry.kind;
  const percent = watchedPercentOf(entry);
  return (
    <PosterCard
      to={itemRoute(kind, entry.id)}
      posterSha256={entry.posterSha256}
      noPosterStillSha256={entry.noPosterStillSha256}
      art={
        entry.kind === "other" ? (
          <Still sourceSha256={entry.landscapeSha256} className="poster-card__tile" />
        ) : undefined
      }
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

/**
 * The lines under a title's name: a film's or series' year, an other video's recorded date (D349
 * §9.1), and that it cannot play now.
 */
export function EntryDetails(props: { readonly entry: Entry }): JSX.Element {
  const { t, i18n } = useTranslation();
  const { entry } = props;
  return (
    <>
      {entry.year === undefined ? null : (
        <PosterDetail>{t("browse.year", { year: entry.year })}</PosterDetail>
      )}
      {entry.recorded === undefined ? null : (
        <PosterDetail>{dateText(entry.recorded.recordedAt, i18n.language)}</PosterDetail>
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
 * Films' or Series' text, chips and sort, criteria of the list itself (the owner, 2026-10-30), in the
 * address, so Back from a title opened from the list returns to it (the owner, 2026-11-02), all of
 * them one pop-up's (D417).
 */
function useBrowseCriteria(kind: BrowseKind, state: ListState): ShownCriteria {
  const { navigation } = useServices();
  return useListCriteria(
    {
      query: state.query ?? "",
      choice: browseChoiceOf(state),
      filters: chipsOf(state),
    },
    kind,
    (next) => {
      navigation.replace({
        screen: kind === "movie" ? "films" : "series",
        ...listStateOf(next),
      });
    },
  );
}

/**
 * Films or series, by name, release date or date added, either way (D366), with no pagination to
 * click through: the next page loads as the end comes into view, or on the button that is always
 * there for it.
 */
export function Browse(props: {
  readonly kind: BrowseKind;
  readonly state: ListState;
}): JSX.Element {
  const { t } = useTranslation();
  const [jump, onLetter, clearJump] = useLetterJumpState();
  const criteria = useBrowseCriteria(props.kind, props.state);
  const { listed } = criteria;
  const { choice, filters } = listed;
  const search = listed.query.trim();
  const listRef = useRef<HTMLDivElement>(null);
  const isNamed = choice.sort === "name";
  const listKey = `${props.kind}/${criteriaKey(listed)}`;
  useResetJumpOnChange(listKey, clearJump);
  return (
    <section className="browse browse__layout" aria-labelledby="browse-heading">
      <div ref={listRef}>
        <div className="browse__bar">
          <h1 id="browse-heading">{t(props.kind === "movie" ? "nav.films" : "nav.series")}</h1>
          {/* Its tools on the bar's line after its name, so Up from the list's first row meets them,
              whichever poster it leaves, and Up again the banner (the owner, 2026-11-04: Up goes to
              the list's tools before the banner). */}
          <ListingTools>
            <ListCriteriaButton criteria={criteria} sorts={browseSorts} />
          </ListingTools>
        </div>
        <BrowsePages
          key={listKey}
          kind={props.kind}
          choice={choice}
          search={search}
          filters={filters}
          jump={isNamed ? jump : undefined}
          listRef={listRef}
        />
      </div>
      {isNamed ? (
        <BrowseAlphabetRail
          kind={props.kind}
          search={search}
          filters={filters}
          isZToA={choice.order === "descending"}
          onLetter={onLetter}
        />
      ) : null}
    </section>
  );
}

/**
 * Films' or Series' alphabet rail, only shown sorted by name (D349 §9): its letters are only
 * those some title in the whole list (every library the person reaches, less any left out, D349
 * §8) begins with, asked for again with the list's own background poll, through
 * alphabet-rail.tsx's one mechanism, shared with the Libraries page.
 */
function BrowseAlphabetRail(props: {
  readonly kind: BrowseKind;
  readonly search: string;
  readonly filters: ListFilters;
  readonly isZToA: boolean;
  readonly onLetter: (letter: string) => void;
}): JSX.Element {
  const letters = useAlphabetRailLetters({
    kind: props.kind,
    library: undefined,
    sort: "name",
    search: props.search,
    filters: props.filters,
  });
  return <AlphabetRail onLetter={props.onLetter} letters={letters} isZToA={props.isZToA} />;
}

/**
 * The sort an address names, run its way: by name unless it names one Films and Series offer, its
 * own way unless it names the other.
 */
function browseChoiceOf(state: ListState): SortChoice {
  const sort = browseSorts.find((offered) => offered === state.sort) ?? "name";
  return { sort, order: orderOf(sort, state.order) };
}

/**
 * What the address holds for a list's criteria: the sort, its way, the text trimmed and each chip,
 * each left out at the list's own.
 */
export function listStateOf(criteria: ListCriteria): ListState {
  const { choice, filters } = criteria;
  const query = criteria.query.trim();
  return {
    ...(choice.sort === "name" ? {} : { sort: choice.sort }),
    ...orderQuery(choice),
    ...(query === "" ? {} : { query }),
    ...chipsOf(filters),
  };
}

/** An entry's id, by which a list shows an entry read twice once, and its tile says it is in view. */
const entryId = (entry: Entry): number => entry.id;

/** One list in one order, read a page at a time, moved to a letter as the alphabet rail asks. */
function BrowsePages(props: {
  readonly kind: BrowseKind;
  readonly choice: SortChoice;
  /** The list's search, "" for none (GET /titles' query). */
  readonly search: string;
  /** The list's chips, none chosen when empty (GET /titles' genre, decade and unwatched). */
  readonly filters: ListFilters;
  readonly jump: LetterJump | undefined;
  readonly listRef: RefObject<HTMLElement | null>;
}): JSX.Element {
  const { api } = useServices();
  const { kind, search, filters } = props;
  const { sort, order } = props.choice;
  const { genre, decade, isUnwatchedOnly } = filters;
  const loadPage = useCallback(
    async (cursor: string): Promise<Page<Entry>> => {
      const query = {
        kind,
        sort,
        ...orderQuery({ sort, order }),
        limit: entriesPerPage,
        ...(search === "" ? {} : { query: search }),
        ...filtersQuery({ genre, decade, isUnwatchedOnly }),
        ...(cursor === "" ? {} : { cursor }),
      };
      const titlesPage = dataOrThrow(await api.GET("/titles", { params: { query } }));
      return { items: titlesPage.titles ?? [], nextCursor: titlesPage.nextCursor ?? "" };
    },
    [api, kind, sort, order, search, genre, decade, isUnwatchedOnly],
  );
  const pages = usePages(loadPage, useBackgroundChangesPoller(), entryId);
  const watchItemAt = useItemsInView(pages.showRange);
  useListLoadsToFocusNoted(
    pages,
    useCallback((entry: Entry) => routeHref(itemRoute(kind, entry.id)), [kind]),
  );
  useLetterJump({
    kind,
    sort: "name",
    isZToA: order === "descending",
    library: undefined,
    search,
    filters,
    jump: props.jump,
    pages,
    listRef: props.listRef,
  });
  return (
    <BrowseView
      kind={kind}
      search={search}
      isNarrowed={hasFilters(filters)}
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
  /** The list's search, "" for none: an empty list then says nothing matches it. */
  readonly search?: string;
  /** Whether chips narrow the list: an empty list then says nothing matches them. */
  readonly isNarrowed?: boolean;
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
          <EmptyList search={props.search ?? ""} isNarrowed={props.isNarrowed === true} />
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
 * Why a list is empty: nothing its search matches; nothing scanned yet; or, for someone whose role the admin has given no
 * library, that they have been given none (D243 §4), which only the admin can change.
 */
function EmptyList(props: { readonly search: string; readonly isNarrowed: boolean }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [me] = useResource("me", async () => dataOrThrow(await api.GET("/me")));
  const isGivenNoLibrary = me.state === "loaded" && me.value.hasNoLibrary;
  if (props.search !== "") {
    return <p className="browse__empty">{t("search.none", { query: props.search })}</p>;
  }
  if (props.isNarrowed) {
    return <p className="browse__empty">{t("chips.none")}</p>;
  }
  return (
    <p className="browse__empty">{t(isGivenNoLibrary ? "browse.noLibrary" : "browse.empty")}</p>
  );
}
