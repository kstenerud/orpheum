// SPDX-License-Identifier: MPL-2.0

import "./browse.css";

import { reportFailure, type components } from "@orpheum/sdk";
import { memo, useCallback, useEffect, useRef, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { focusAndShow } from "./focus-navigation.ts";
import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow, unavailableText } from "./problem.ts";
import { StatusTray } from "./status-cues.tsx";
import { PosterCard, PosterDetail } from "./poster-card.tsx";
import { itemRoute } from "./router.ts";
import { EndOfList } from "./end-of-list.tsx";
import { useServices } from "./services.tsx";
import { Loaded } from "./loaded.tsx";
import { useItemsInView } from "./items-in-view.ts";
import { usePages, type Page, type Pages } from "./use-pages.ts";
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
  const [jump, setJump] = useState<LetterJump | undefined>(undefined);
  const listRef = useRef<HTMLDivElement>(null);
  const onLetter = useCallback((letter: string) => {
    setJump((last) => ({ letter, count: (last?.count ?? 0) + 1 }));
  }, []);
  return (
    <section className="browse browse__layout" aria-labelledby="browse-heading">
      <div ref={listRef}>
        <div className="browse__bar">
          <h1 id="browse-heading">
            {t(props.kind === "movie" ? "browse.films" : "browse.series")}
          </h1>
          <SortChoice
            sort={sort}
            onSort={(chosen) => {
              setJump(undefined);
              setSort(chosen);
            }}
          />
        </div>
        <BrowsePages
          key={`${props.kind}/${sort}`}
          kind={props.kind}
          sort={sort}
          jump={sort === "name" ? jump : undefined}
          listRef={listRef}
        />
      </div>
      {sort === "name" ? <AlphabetRail onLetter={onLetter} /> : null}
    </section>
  );
}

/** A letter of the alphabet rail pressed, counted, so the same letter pressed again jumps again. */
interface LetterJump {
  readonly letter: string;
  readonly count: number;
}

/** The letters of the alphabet rail, a list's names' first characters: digits and marks, then A to Z. */
const railLetters = [
  "#",
  ...Array.from({ length: 26 }, (_, index) => String.fromCodePoint(0x41 + index)),
];

/**
 * A list's alphabet beside its grid, by name only, to jump to a letter (the owner, 2026-10-25): a
 * letter moves the list to its first title, # to the first of all, those beginning with a digit or
 * a mark, a focus container of its own.
 */
function AlphabetRail(props: { readonly onLetter: (letter: string) => void }): JSX.Element {
  const { t } = useTranslation();
  return (
    <ol className="alphabet-rail" aria-label={t("browse.alphabet")} data-focus-container="">
      {railLetters.map((letter) => (
        <li key={letter}>
          <button
            type="button"
            className="alphabet-rail__letter"
            onClick={() => {
              props.onLetter(letter);
            }}
          >
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

/** One list in one order, read a page at a time, moved to a letter as the alphabet rail asks. */
function BrowsePages(props: {
  readonly kind: BrowseKind;
  readonly sort: BrowseSort;
  readonly jump: LetterJump | undefined;
  readonly listRef: RefObject<HTMLElement | null>;
}): JSX.Element {
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
  useLetterJump({ kind, jump: props.jump, pages, listRef: props.listRef });
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

/**
 * Moves a list by name to the first title at or after the letter jump names, as the server sorts
 * them (`from`), # to the first of all: it loads the list's pages up to it, as scrolling would,
 * and focuses its poster at the top of the view, so a remote goes on from there; where no title
 * comes at or after the letter, the last. The newest jump wins over one still loading.
 */
function useLetterJump(props: {
  readonly kind: BrowseKind;
  readonly jump: LetterJump | undefined;
  readonly pages: Pages<Entry>;
  readonly listRef: RefObject<HTMLElement | null>;
}): void {
  const { api } = useServices();
  const { kind, jump, listRef } = props;
  const { loadUntil, loadedSoFar } = props.pages;
  // The place of the poster to focus once it is drawn, the last's for -1, undefined for none; the
  // jump found it, the drawing of the pages after it focuses it.
  const toFocusRef = useRef<number | undefined>(undefined);
  const [found, setFound] = useState(0);
  useEffect(() => {
    if (jump === undefined) {
      return undefined;
    }
    let isCurrent = true;
    const from = jump.letter === "#" ? {} : { from: jump.letter };
    const query = { kind, sort: "name" as const, limit: 1, ...from };
    api
      .GET("/titles", { params: { query } })
      .then(async (answer) => {
        const first = dataOrThrow(answer).titles?.[0];
        const place = await loadUntil((entry) => entry.id === first?.id);
        if (isCurrent) {
          toFocusRef.current = place ?? -1;
          setFound((count) => count + 1);
        }
      })
      .catch(reportFailure);
    return () => {
      isCurrent = false;
    };
  }, [api, kind, jump, loadUntil]);
  useEffect(() => {
    const place = toFocusRef.current;
    if (place === undefined) {
      return;
    }
    const posters = listRef.current?.querySelectorAll<HTMLElement>(".poster-grid > li > a");
    const poster = place < 0 ? posters?.[posters.length - 1] : posters?.[place];
    if (poster !== undefined) {
      toFocusRef.current = undefined;
      showAtTop(poster);
    }
  }, [found, loadedSoFar, listRef]);
}

/** Focuses a poster jumped to, its row scrolled to the view's top, clear of the banner. */
function showAtTop(poster: HTMLElement): void {
  // A test's DOM may have no scrolling at all.
  if (typeof poster.scrollIntoView === "function") {
    poster.scrollIntoView({ block: "start" });
  }
  // Focused as a key would, a remote's press being one; scrolled on, gently, clear of the banner.
  focusAndShow(poster, true);
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
