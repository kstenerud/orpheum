// SPDX-License-Identifier: MPL-2.0

import "./browse.css";

import { artUrl, type components } from "@orpheum/sdk";
import { useCallback, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { Link } from "./link.tsx";
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

const pageSize = 60;

/** Where an entry leads: a film's page, or a series'. */
export function entryRoute(kind: BrowseKind, id: number): Route {
  return kind === "movie" ? { screen: "title", id } : { screen: "collection", id };
}

/** A poster, or a named blank where there is none; the card's own text names the item. */
export function Poster(props: { readonly hash: string | undefined }): JSX.Element {
  const { t } = useTranslation();
  if (props.hash === undefined) {
    return (
      <span className="poster poster--blank" aria-hidden="true">
        {t("common.noPoster")}
      </span>
    );
  }
  return (
    <img
      className="poster"
      src={artUrl(props.hash, "portrait-m")}
      alt={""}
      loading="lazy"
      width={240}
      height={360}
    />
  );
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
            <Poster hash={entry.posterHash} />
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
  const load = useCallback(
    async (cursor: string): Promise<Page<Entry>> => {
      const query = { kind, sort, limit: pageSize, ...(cursor === "" ? {} : { cursor }) };
      const { data, error } = await api.GET("/titles", { params: { query } });
      if (data === undefined) {
        throw new Error(error.error.message);
      }
      return { items: data.titles ?? [], next: data.nextCursor ?? "" };
    },
    [api, kind, sort],
  );
  const pages = usePages(load);
  return (
    <BrowseView
      kind={kind}
      pages={pages.list}
      isLoadingMore={pages.isLoadingMore}
      onMore={pages.more}
    />
  );
}

/** A list's pages as they show: loading, failed with why, empty, or the grid and a way to more. */
export function BrowseView(props: {
  readonly kind: BrowseKind;
  readonly pages: Resource<Page<Entry>>;
  readonly isLoadingMore: boolean;
  readonly onMore: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { pages } = props;
  if (pages.state === "loading") {
    return <p>{t("common.loading")}</p>;
  }
  if (pages.state === "failed") {
    return <p role="alert">{t("common.failed", { message: pages.message })}</p>;
  }
  if (pages.value.items.length === 0) {
    return <p>{t("browse.empty")}</p>;
  }
  return (
    <>
      <PosterGrid kind={props.kind} entries={pages.value.items} />
      {pages.value.next === "" ? null : (
        <EndOfList onVisible={props.onMore}>
          <button type="button" onClick={props.onMore} disabled={props.isLoadingMore}>
            {t("browse.more")}
          </button>
        </EndOfList>
      )}
    </>
  );
}
