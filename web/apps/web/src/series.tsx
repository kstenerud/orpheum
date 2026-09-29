// SPDX-License-Identifier: MPL-2.0

import "./series.css";

import { artUrl, type components } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { dataOrThrow } from "./problem.ts";
import { Poster } from "./poster.tsx";
import { Link } from "./link.tsx";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { useResource } from "./use-resource.ts";

/** A series with its episodes, as the server sends it. */
export type SeriesDetail = components["schemas"]["CollectionOutputBody"];
type Episode = components["schemas"]["EpisodeBody"];

/** Episodes grouped by season, in aired order, specials (season 0) last. */
export function bySeason(episodes: readonly Episode[]): [number, Episode[]][] {
  const seasons = new Map<number, Episode[]>();
  for (const episode of episodes) {
    seasons.set(episode.seasonNumber, [...(seasons.get(episode.seasonNumber) ?? []), episode]);
  }
  // Specials are season 0; they sort after every numbered season.
  const order = (season: number): number => (season === 0 ? Number.MAX_SAFE_INTEGER : season);
  return [...seasons.entries()].sort(([first], [second]) => order(first) - order(second));
}

/** A series' page: its art and what it is about, then its seasons, each episode with its still. */
export function SeriesView(props: { readonly series: SeriesDetail }): JSX.Element {
  const { t } = useTranslation();
  const { series } = props;
  return (
    <article className="series">
      <div className="series__head">
        <Poster hash={series.art.posterHash} />
        <div>
          <h1>{series.name}</h1>
          {series.year === undefined ? null : <p className="series__year">{series.year}</p>}
          {series.overview === undefined ? null : <p>{series.overview}</p>}
        </div>
      </div>
      {bySeason(series.episodes ?? []).map(([season, episodes]) => (
        <section key={season} aria-labelledby={`season-${String(season)}`}>
          <h2 id={`season-${String(season)}`}>
            {season === 0 ? t("title.specials") : t("title.season", { season })}
          </h2>
          <ol className="series__episodes">
            {episodes.map((episode) => (
              <li
                key={episode.id}
                className={episode.hasLiveFile ? undefined : "series__episode--absent"}
              >
                <Link to={{ screen: "title", id: episode.id }} className="series__episode">
                  {episode.stillHash === undefined ? (
                    <span className="series__still" aria-hidden="true" />
                  ) : (
                    <img
                      className="series__still"
                      src={artUrl(episode.stillHash, "landscape-s")}
                      alt={""}
                      loading="lazy"
                    />
                  )}
                  <span>
                    {t("title.episodeLabel", { number: episode.episodeNumber, name: episode.name })}
                  </span>
                  {episode.hasLiveFile ? null : (
                    <span className="series__absent">{t("title.unavailable")}</span>
                  )}
                </Link>
              </li>
            ))}
          </ol>
        </section>
      ))}
    </article>
  );
}

/** A series' page, loaded by its id. */
export function Series(props: { readonly id: number }): JSX.Element {
  const { api } = useServices();
  const [series] = useResource(`series/${String(props.id)}`, async () => {
    const data = dataOrThrow(
      await api.GET("/collections/{id}", {
        params: { path: { id: props.id } },
      }),
    );
    return data;
  });
  return <Loaded resource={series} render={(value) => <SeriesView series={value} />} />;
}
