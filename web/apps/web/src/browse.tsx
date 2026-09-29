// SPDX-License-Identifier: MPL-2.0

import "./browse.css";

import type { components } from "@orpheum/sdk";
import { useCallback, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { dataOrThrow, problemText } from "./problem.ts";
import { ContinueWatching } from "./continue-watching.tsx";
import { Link } from "./link.tsx";
import { Poster } from "./poster.tsx";
import type { Route } from "./router.ts";
import { EndOfList } from "./end-of-list.tsx";
import { useServices } from "./services.tsx";
import { usePages, type Page } from "./use-pages.ts";
import type { Resource } from "./use-resource.ts";

/** One film or series in a list, as the server sends it. */
export type Entry = components["schemas"]["EntryBody"];

/** Which list: films or series. */
export type BrowseKind = "movie" | "series";

/** The order a list comes in. */
export type BrowseSort = "name" | "added";

const entriesPerPage = 60;

/** Where an entry leads: a film's page, or a series'. */
export function entryRoute(kind: BrowseKind, id: number): Route {
  return kind === "movie" ? { screen: "title", id } : { screen: "series-page", id };
}

/** A grid of films or series, each a poster with its name and year under it. */
export function PosterGrid(props: {
  readonly kind: BrowseKind;
  readonly entries: readonly Entry[];
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <ul className="poster-grid">
      {props.entries.map((entry) => (
        <li key={entry.id}>
          <Link to={entryRoute(props.kind, entry.id)} className="poster-grid__card">
            <Poster hash={entry.posterSha256} />
            <span className="poster-grid__name">{entry.name}</span>
            {entry.year === undefined ? null : (
              <span className="poster-grid__year">{t("browse.year", { year: entry.year })}</span>
            )}
          </Link>
        </li>
      ))}
    </ul>
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
      <BrowsePages key={`${props.kind}/${sort}`} kind={props.kind} sort={sort} />
    </section>
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
  const pages = usePages(loadPage);
  return (
    <BrowseView
      kind={kind}
      loadedEntries={pages.pagesSoFar}
      isLoadingMore={pages.isLoadingMore}
      onLoadMore={pages.loadMore}
    />
  );
}

/** A list's pages as they show: loading, failed with why, empty, or the grid and a way to more. */
export function BrowseView(props: {
  readonly kind: BrowseKind;
  readonly loadedEntries: Resource<Page<Entry>>;
  readonly isLoadingMore: boolean;
  readonly onLoadMore: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { loadedEntries } = props;
  if (loadedEntries.state === "loading") {
    return <p>{t("common.loading")}</p>;
  }
  if (loadedEntries.state === "failed") {
    return <p role="alert">{problemText(t, loadedEntries.cause)}</p>;
  }
  if (loadedEntries.value.items.length === 0) {
    return <p>{t("browse.empty")}</p>;
  }
  return (
    <>
      <PosterGrid kind={props.kind} entries={loadedEntries.value.items} />
      {loadedEntries.value.nextCursor === "" ? null : (
        <EndOfList onVisible={props.onLoadMore}>
          <button type="button" onClick={props.onLoadMore} disabled={props.isLoadingMore}>
            {t("common.showMore")}
          </button>
        </EndOfList>
      )}
    </>
  );
}
