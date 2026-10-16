// SPDX-License-Identifier: MPL-2.0

import "./series.css";

import { artUrl, type components } from "@orpheum/sdk";
import { useCallback, type JSX } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow, unavailableText } from "./problem.ts";
import { MatchCorrection } from "./match-correction.tsx";
import { StatusNotes } from "./status-cues.tsx";
import { Poster } from "./poster.tsx";
import { Link } from "./link.tsx";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { Remembered, useRemembered } from "./remembered.ts";
import { useResource, type Resource } from "./use-resource.ts";
import { usePages, type Page } from "./use-pages.ts";
import { EndOfList } from "./end-of-list.tsx";

/** A series with its seasons, as the server sends it; each season's episodes are read apart. */
export type SeriesDetail = components["schemas"]["CollectionOutputBody"];
type Season = components["schemas"]["SeasonBody"];
type Episode = components["schemas"]["EpisodeBody"];

/** How many of a season's episodes one read asks for: a season or two of most series. */
const episodesPerPage = 100;

/** The name of a season: "Season 2", or "Specials" for season 0. */
function seasonName(t: TFunction, season: number): string {
  return season === 0 ? t("title.specials") : t("title.season", { season });
}

/**
 * A series' page: its art and what it is about, then its seasons, each leading to its episodes; or,
 * with season chosen, that season's episodes. A series of one season shows its episodes at once.
 * Every season's episodes on one page, the seasons only headings, was too long a list to find one
 * in (the owner, 2026-10-07: series, then seasons, then episodes).
 */
export function SeriesView(props: {
  readonly series: SeriesDetail;
  readonly season?: number | undefined;
  /** Told the series' match was corrected, an admin's (D301 §2): the page reads it again. */
  readonly onMatchCorrected?: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { series } = props;
  const seasons = series.seasons ?? [];
  const shown =
    seasons.length === 1
      ? seasons[0]
      : seasons.find((season) => season.seasonNumber === props.season);
  return (
    <article className="series">
      <div className="series__head">
        <Poster sourceSha256={series.art.posterSha256} />
        <div>
          <h1>{series.name}</h1>
          {series.year === undefined ? null : <p className="series__year">{series.year}</p>}
          {series.overview === undefined ? null : <p>{series.overview}</p>}
          <StatusNotes subject={series} />
          {props.onMatchCorrected === undefined ? null : (
            <MatchCorrection itemId={series.id} onCorrected={props.onMatchCorrected} />
          )}
        </div>
      </div>
      {shown === undefined ? (
        <SeasonList seriesId={series.id} seasons={seasons} />
      ) : (
        <section aria-labelledby={`season-${String(shown.seasonNumber)}`}>
          {seasons.length === 1 ? null : (
            <Link to={{ screen: "series-page", collectionId: series.id }}>
              {t("title.allSeasons")}
            </Link>
          )}
          <h2 id={`season-${String(shown.seasonNumber)}`}>{seasonName(t, shown.seasonNumber)}</h2>
          <SeasonEpisodes seriesId={series.id} season={shown.seasonNumber} />
        </section>
      )}
    </article>
  );
}

/** A series' seasons, each with its first episode's still and how many episodes it has. */
function SeasonList(props: {
  readonly seriesId: number;
  readonly seasons: readonly Season[];
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <ol className="series__episodes">
      {props.seasons.map((season) => (
        <li key={season.seasonNumber}>
          <Link
            to={{
              screen: "series-page",
              collectionId: props.seriesId,
              season: season.seasonNumber,
            }}
            className="series__episode"
          >
            {season.stillSha256 === undefined ? (
              <span className="series__still" aria-hidden="true" />
            ) : (
              <img
                className="series__still"
                src={artUrl(season.stillSha256, "landscape-s")}
                alt={""}
                loading="lazy"
              />
            )}
            <span>{seasonName(t, season.seasonNumber)}</span>
            <span className="series__absent">
              {t("title.episodeCount", { count: season.episodeCount })}
            </span>
          </Link>
        </li>
      ))}
    </ol>
  );
}

/** One season's episodes, read a page at a time (D306 §9). */
function SeasonEpisodes(props: {
  readonly seriesId: number;
  readonly season: number;
}): JSX.Element {
  const { api } = useServices();
  const { seriesId, season } = props;
  const loadPage = useCallback(
    async (cursor: string): Promise<Page<Episode>> => {
      const query = { limit: episodesPerPage, ...(cursor === "" ? {} : { cursor }) };
      const seasonPage = dataOrThrow(
        await api.GET("/collections/{id}/seasons/{season}", {
          params: { path: { id: seriesId, season }, query },
        }),
      );
      return { items: seasonPage.episodes ?? [], nextCursor: seasonPage.nextCursor ?? "" };
    },
    [api, seriesId, season],
  );
  const pages = usePages(loadPage, useBackgroundChangesPoller());
  return (
    <SeasonView
      loadedEpisodes={pages.loadedSoFar}
      isLoadingMore={pages.isLoadingMore}
      onLoadMore={pages.loadMore}
    />
  );
}

/** A season's pages as they show: loading, failed with why, or its episodes and a way to more. */
export function SeasonView(props: {
  readonly loadedEpisodes: Resource<Page<Episode>>;
  readonly isLoadingMore: boolean;
  readonly onLoadMore: () => void;
}): JSX.Element {
  return (
    <Loaded
      resource={props.loadedEpisodes}
      render={(page) => (
        <>
          <EpisodeList episodes={page.items} />
          {page.nextCursor === "" ? null : (
            <EndOfList onMore={props.onLoadMore} isLoadingMore={props.isLoadingMore} />
          )}
        </>
      )}
    />
  );
}

/** One season's episodes, each with its still, those not on disk or unavailable said so. */
function EpisodeList(props: { readonly episodes: readonly Episode[] }): JSX.Element {
  const { t } = useTranslation();
  return (
    <ol className="series__episodes">
      {props.episodes.map((episode) => (
        <li
          key={episode.id}
          className={
            episode.hasLiveFile && episode.unavailable === undefined
              ? undefined
              : "series__item--absent"
          }
        >
          <Link to={{ screen: "title", itemId: episode.id }} className="series__episode">
            {episode.stillSha256 === undefined ? (
              <span className="series__still" aria-hidden="true" />
            ) : (
              <img
                className="series__still"
                src={artUrl(episode.stillSha256, "landscape-s")}
                alt={""}
                loading="lazy"
              />
            )}
            <span>
              {t("title.episodeLabel", { number: episode.episodeNumber, name: episode.name })}
            </span>
            {episode.hasLiveFile ? null : (
              <span className="series__absent">{t("title.notOnDisk")}</span>
            )}
            {episode.unavailable === undefined ? null : (
              <span className="series__absent">{unavailableText(t, episode.unavailable)}</span>
            )}
          </Link>
        </li>
      ))}
    </ol>
  );
}

/** The series' pages this tab has shown. */
const rememberedSeries = new Remembered<SeriesDetail>();

/** A series' page, loaded by its id: its seasons, or the season chosen. */
export function Series(props: {
  readonly id: number;
  readonly season?: number | undefined;
}): JSX.Element {
  const { api } = useServices();
  const [loaded, reload] = useResource(
    `series/${String(props.id)}`,
    async () => {
      return dataOrThrow(
        await api.GET("/collections/{id}", {
          params: { path: { id: props.id } },
        }),
      );
    },
    useBackgroundChangesPoller(),
  );
  const series = useRemembered(`series/${String(props.id)}`, loaded, rememberedSeries);
  return (
    <Loaded
      resource={series}
      render={(value) => (
        <SeriesView series={value} season={props.season} onMatchCorrected={reload} />
      )}
    />
  );
}
