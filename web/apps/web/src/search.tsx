// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { dataOrThrow } from "./problem.ts";
import { submitWith } from "./form-row.tsx";
import { Link } from "./link.tsx";
import { itemRoute } from "./router.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./loaded.tsx";
import { useResource } from "./use-resource.ts";

/** One search result, as the server sends it. */
export type SearchHit = components["schemas"]["HitBody"];

const kindKeys = {
  movie: "search.kindMovie",
  series: "search.kindSeries",
  episode: "search.kindEpisode",
  extra: "search.kindExtra",
  other: "search.kindOther",
} as const;

/** Search results, best first, with no cap to page through. */
export function SearchResults(props: {
  readonly query: string;
  readonly hits: readonly SearchHit[];
}): JSX.Element {
  const { t } = useTranslation();
  if (props.hits.length === 0) {
    return <p>{t("search.none", { query: props.query })}</p>;
  }
  return (
    <ul>
      {props.hits.map((hit) => (
        <li key={`${hit.kind}/${String(hit.id)}`}>
          <Link to={itemRoute(hit.kind, hit.id)}>{hit.name}</Link>{" "}
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
  const [typedQuery, setTypedQuery] = useState(props.query);
  const submit = submitWith(() => {
    navigation.go({ screen: "search", query: typedQuery.trim() });
  });
  return (
    <section aria-labelledby="search-heading">
      <h1 id="search-heading">{t("search.heading")}</h1>
      <form role="search" onSubmit={submit}>
        <label>
          {t("search.label")}
          <input
            type="search"
            value={typedQuery}
            onChange={(event) => {
              setTypedQuery(event.target.value);
            }}
          />
        </label>
        <button type="submit">{t("search.submit")}</button>
      </form>
      {props.query === "" ? null : <QueryResults key={props.query} query={props.query} />}
    </section>
  );
}

/** The results for one query. */
function QueryResults(props: { readonly query: string }): JSX.Element {
  const { api } = useServices();
  const [hits] = useResource(`search/${props.query}`, async () => {
    const searchBody = dataOrThrow(
      await api.GET("/search", {
        params: { query: { query: props.query, limit: 100 } },
      }),
    );
    return searchBody.results ?? [];
  });
  return (
    <Loaded
      resource={hits}
      render={(value) => <SearchResults query={props.query} hits={value} />}
    />
  );
}
