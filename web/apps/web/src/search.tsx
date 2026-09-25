// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { useState, type JSX, type SyntheticEvent } from "react";
import { useTranslation } from "react-i18next";

import { Link } from "./link.tsx";
import type { Route } from "./router.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { useResource } from "./use-resource.ts";

/** One search result, as the server sends it. */
export type Hit = components["schemas"]["HitBody"];

/** Where a result leads: a series' page, else a title's. */
export function hitRoute(hit: Hit): Route {
  return hit.kind === "series"
    ? { screen: "collection", id: hit.id }
    : { screen: "title", id: hit.id };
}

const kindKeys = {
  movie: "search.kindMovie",
  series: "search.kindSeries",
  episode: "search.kindEpisode",
  extra: "search.kindExtra",
} as const;

/** Search results, best first, with no cap to page through. */
export function SearchResults(props: {
  readonly query: string;
  readonly hits: readonly Hit[];
}): JSX.Element {
  const { t } = useTranslation();
  if (props.hits.length === 0) {
    return <p>{t("search.none", { query: props.query })}</p>;
  }
  return (
    <ul>
      {props.hits.map((hit) => (
        <li key={`${hit.kind}/${String(hit.id)}`}>
          <Link to={hitRoute(hit)}>{hit.name}</Link>{" "}
          {hit.year === undefined ? null : <span>{hit.year}</span>}{" "}
          <span>{t(kindKeys[hit.kind])}</span>
        </li>
      ))}
    </ul>
  );
}

/** Search: a box whose query is in the address, so a search can be shared and gone back to. */
export function Search(props: { readonly query: string }): JSX.Element {
  const { t } = useTranslation();
  const { navigation } = useServices();
  const [typed, setTyped] = useState(props.query);
  const submit = (event: SyntheticEvent): void => {
    event.preventDefault();
    navigation.go({ screen: "search", query: typed.trim() });
  };
  return (
    <section aria-labelledby="search-heading">
      <h1 id="search-heading">{t("search.heading")}</h1>
      <form role="search" onSubmit={submit}>
        <label>
          {t("search.label")}
          <input
            type="search"
            value={typed}
            onChange={(event) => {
              setTyped(event.target.value);
            }}
          />
        </label>
        <button type="submit">{t("search.submit")}</button>
      </form>
      {props.query === "" ? null : <SearchFor key={props.query} query={props.query} />}
    </section>
  );
}

/** The results for one query. */
function SearchFor(props: { readonly query: string }): JSX.Element {
  const { api } = useServices();
  const [hits] = useResource(`search/${props.query}`, async () => {
    const { data, error } = await api.GET("/search", {
      params: { query: { q: props.query, limit: 100 } },
    });
    if (data === undefined) {
      throw new Error(error.error.message);
    }
    return data.results ?? [];
  });
  return (
    <Loaded
      resource={hits}
      render={(value) => <SearchResults query={props.query} hits={value} />}
    />
  );
}
