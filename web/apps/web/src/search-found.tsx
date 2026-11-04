// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { PersonFace } from "./credits.tsx";
import { Link } from "./link.tsx";
import { dataOrThrow, problemText } from "./problem.ts";
import { recentSearches } from "./recent-searches.ts";
import { itemRoute } from "./router.ts";
import { useServices } from "./services.tsx";

type GroupHit = components["schemas"]["GroupHitBody"];
type FoundGroup = components["schemas"]["GroupBody"];

/** The groups a search finds, in the order shown (search-by-more §4.2): titles, people, genres. */
type GroupName = "titles" | "people" | "genres";

const groupNames: readonly GroupName[] = ["titles", "people", "genres"];

/** The list a genre found opens, filtered by it: Films, or Series from Series' own search. */
export type GenreList = "films" | "series";

/** How long typing rests before what is typed is asked for: each letter of a word is not a search. */
export const searchRestMs = 200;

/** How many of each group a page of the search's answer holds. */
export const foundPerPage = 8;

/** What one search found: the search, and each group's hits so far with its next page's cursor. */
interface Found {
  readonly query: string;
  readonly groups: Readonly<Record<GroupName, FoundGroup>>;
}

/** A search's answer, or the failure of its asking; for one search alone. */
type Answer =
  | { readonly query: string; readonly found: Found }
  | { readonly query: string; readonly cause: unknown };

/**
 * What is shown, with page of group added when it is still found, the answer it is a page of: not
 * once a newer search was answered, nor twice, a second press of More before the first page came
 * asking for the same page, which finds what is shown already grown.
 */
function withPage(
  shown: Answer | undefined,
  found: Found,
  paged: { readonly group: GroupName; readonly page: FoundGroup },
): Answer | undefined {
  const { group, page } = paged;
  if (shown === undefined || !("found" in shown) || shown.found !== found) {
    return shown;
  }
  const grown: FoundGroup = {
    hits: [...(found.groups[group].hits ?? []), ...(page.hits ?? [])],
    ...(page.nextCursor === undefined ? {} : { nextCursor: page.nextCursor }),
  };
  const groups = { ...found.groups, [group]: grown };
  return { query: found.query, found: { query: found.query, groups } };
}

/**
 * What a search finds, asked for once typing rests, in groups (search-by-more §4.2), and a way to
 * ask for one group's next page. What was shown stays until the answer to a newer search replaces
 * it, so the results do not blink at each letter; an answer to a search typed over is dropped, so a
 * slower older answer never replaces a newer one.
 */
function useGroupedSearch(query: string): {
  readonly answer: Answer | undefined;
  readonly more: (group: GroupName) => void;
} {
  const { api } = useServices();
  const [answer, setAnswer] = useState<Answer | undefined>(undefined);
  useEffect(() => {
    if (query === "") {
      return undefined;
    }
    const asking = new AbortController();
    const timer = setTimeout(() => {
      api
        .GET("/search/grouped", {
          params: { query: { query, limit: foundPerPage } },
          signal: asking.signal,
        })
        .then(dataOrThrow)
        .then(
          (groups) => {
            if (!asking.signal.aborted) {
              setAnswer({ query, found: { query, groups } });
            }
          },
          (cause: unknown) => {
            if (!asking.signal.aborted) {
              setAnswer({ query, cause });
            }
          },
        );
    }, searchRestMs);
    return () => {
      clearTimeout(timer);
      asking.abort();
    };
  }, [api, query]);
  const more = (group: GroupName): void => {
    if (answer === undefined || !("found" in answer)) {
      return;
    }
    const { found } = answer;
    const cursor = found.groups[group].nextCursor;
    if (cursor === undefined || cursor === "") {
      return;
    }
    void api
      .GET("/search/grouped", {
        params: { query: { query: found.query, limit: foundPerPage, group, cursor } },
      })
      .then(dataOrThrow)
      .then(
        (page) => {
          setAnswer((shown) => withPage(shown, found, { group, page: page[group] }));
        },
        () => {
          // The page already shown stays; More is there to press again.
        },
      );
  };
  return { answer, more };
}

/**
 * Under the search's field (search-by-more §4.2, §4.5): while it is empty, the device's recent
 * searches, each putting itself back in the field; else what it finds, in groups, Titles, People
 * then Genres, each with More for its next page. A title opens its page, a person theirs, and a
 * genre Films (or Series) filtered by it; opening one tells onOpen.
 */
export function SearchFound(props: {
  readonly typed: string;
  readonly genreList: GenreList;
  readonly onRecent: (search: string) => void;
  readonly onOpen: () => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  const query = props.typed.trim();
  const { answer, more } = useGroupedSearch(query);
  if (query === "") {
    return <RecentSearches onRecent={props.onRecent} />;
  }
  if (answer === undefined) {
    return null;
  }
  if ("cause" in answer) {
    return (
      <p className="search-found" role="alert">
        {problemText(t, answer.cause)}
      </p>
    );
  }
  const { groups } = answer.found;
  const headings: Readonly<Record<GroupName, string>> = {
    titles: t("found.titles"),
    people: t("found.people"),
    genres: t("found.genres"),
  };
  const isNone = groupNames.every((group) => (groups[group].hits ?? []).length === 0);
  if (isNone) {
    return <p className="search-found">{t("found.none")}</p>;
  }
  return (
    <div className="search-found">
      {groupNames.map((group) => (
        <FoundGroupList
          key={group}
          group={group}
          heading={headings[group]}
          found={groups[group]}
          genreList={props.genreList}
          onMore={() => {
            more(group);
          }}
          onOpen={props.onOpen}
        />
      ))}
    </div>
  );
}

/** One group of what a search found, under its heading, More after it while it has more; none when empty. */
function FoundGroupList(props: {
  readonly group: GroupName;
  readonly heading: string;
  readonly found: FoundGroup;
  readonly genreList: GenreList;
  readonly onMore: () => void;
  readonly onOpen: () => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  const hits = props.found.hits ?? [];
  if (hits.length === 0) {
    return null;
  }
  const headingId = `search-found-${props.group}`;
  const hasMore = props.found.nextCursor !== undefined && props.found.nextCursor !== "";
  return (
    <section className="search-found__group" aria-labelledby={headingId}>
      <h3 id={headingId}>{props.heading}</h3>
      <ul className="search-found__list" data-focus-container="">
        {hits.map((hit) => (
          <li key={`${hit.kind ?? ""}/${String(hit.id)}`}>
            <FoundHit
              group={props.group}
              hit={hit}
              genreList={props.genreList}
              onOpen={props.onOpen}
            />
          </li>
        ))}
      </ul>
      {hasMore ? (
        <button type="button" className="search-found__more" onClick={props.onMore}>
          {t("found.more")}
        </button>
      ) : null}
    </section>
  );
}

/** One hit, leading where its group's hits lead: a title's page, a person's, or a genre's list. */
function FoundHit(props: {
  readonly group: GroupName;
  readonly hit: GroupHit;
  readonly genreList: GenreList;
  readonly onOpen: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { hit } = props;
  switch (props.group) {
    case "titles":
      return (
        <Link
          to={itemRoute(hit.kind ?? "movie", hit.id)}
          className="search-found__hit"
          onFollow={props.onOpen}
        >
          <span className="search-found__name">{hit.name}</span>
          {hit.year === undefined ? null : (
            <span className="search-found__detail">{t("browse.year", { year: hit.year })}</span>
          )}
        </Link>
      );
    case "people":
      return (
        <Link
          to={{ screen: "credited-person", personId: hit.id }}
          className="search-found__hit"
          onFollow={props.onOpen}
        >
          <PersonFace name={hit.name} />
          <span className="search-found__name">{hit.name}</span>
        </Link>
      );
    case "genres":
      return (
        <Link
          to={{ screen: props.genreList, genre: hit.id }}
          className="search-found__hit"
          onFollow={props.onOpen}
        >
          <span className="search-found__name">{hit.name}</span>
          {hit.titleCount === undefined ? null : (
            <span className="search-found__detail">
              {hit.titleCount === 1
                ? t("found.oneTitle")
                : t("found.titleCount", { count: hit.titleCount })}
            </span>
          )}
        </Link>
      );
  }
}

/** The device's recent searches, newest first, each putting itself back in the field; none when none. */
function RecentSearches(props: {
  readonly onRecent: (search: string) => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  // Read as the field empties: a search kept since the dialog opened shows the next time it does.
  const [recent] = useState(() => recentSearches());
  if (recent.length === 0) {
    return null;
  }
  return (
    <section className="search-found search-found__group" aria-labelledby="search-found-recent">
      <h3 id="search-found-recent">{t("found.recent")}</h3>
      <ul className="search-found__recent" data-focus-container="">
        {recent.map((search) => (
          <li key={search}>
            <button
              type="button"
              onClick={() => {
                props.onRecent(search);
              }}
            >
              {search}
            </button>
          </li>
        ))}
      </ul>
    </section>
  );
}
