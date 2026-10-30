// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { dataOrThrow } from "./problem.ts";
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

/**
 * The results for one query, in a library (the Libraries page's own search field, D349 §9.1) or,
 * with none, every library the person reaches.
 */
export function QueryResults(props: {
  readonly query: string;
  readonly library?: number | undefined;
  /** Only hits of this kind: All Films' search finds films, All Series' series. */
  readonly kind?: SearchHit["kind"] | undefined;
}): JSX.Element {
  const { api } = useServices();
  const { query, library, kind } = props;
  const [hits] = useResource(`search/${String(library)}/${String(kind)}/${query}`, async () => {
    const searchBody = dataOrThrow(
      await api.GET("/search", {
        params: { query: { query, limit: 100, ...(library === undefined ? {} : { library }) } },
      }),
    );
    const results = searchBody.results ?? [];
    return kind === undefined ? results : results.filter((hit) => hit.kind === kind);
  });
  return (
    <Loaded resource={hits} render={(value) => <SearchResults query={query} hits={value} />} />
  );
}
