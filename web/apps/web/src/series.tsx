// SPDX-License-Identifier: MPL-2.0

import "./series.css";

import type { components } from "@orpheum/sdk";
import { useCallback, useId, type JSX } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { BackButton } from "./back-button.tsx";
import type { Route } from "./router.ts";
import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow, problemText, throwIfRefused, unavailableText } from "./problem.ts";
import { FilingReportButton } from "./filing-report.tsx";
import { MatchCorrection } from "./match-correction.tsx";
import { StatusNotes } from "./status-cues.tsx";
import { Poster, Still } from "./poster.tsx";
import { Link } from "./link.tsx";
import { usePageInterest } from "./page-interest.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { Remembered, useRemembered } from "./remembered.ts";
import { useResource, type Resource } from "./use-resource.ts";
import { usePages, type Page } from "./use-pages.ts";
import { EndOfList } from "./end-of-list.tsx";
import { useItemsInView } from "./items-in-view.ts";
import { groupName, orderingAired, placeCode } from "./episode-place.ts";
import { camelKey } from "./catalogue-key.ts";
import { useRequest } from "./use-request.ts";

/** A series with its seasons, as the server sends it; each season's episodes are read apart. */
export type SeriesDetail = components["schemas"]["CollectionOutputBody"];
type Season = components["schemas"]["SeasonBody"];
type Episode = components["schemas"]["EpisodeBody"];
type NextUp = components["schemas"]["NextUpBody"];
type Ordering = components["schemas"]["OrderingBody"];

/** How many of a season's episodes one read asks for: a season or two of most series. */
const episodesPerPage = 100;

/** The name of a season, a group of the ordering the series is shown in: "Season 2", "Disc 2". */
function seasonName(t: TFunction, series: SeriesDetail, season: Season): string {
  return groupName(t, series.ordering, { number: season.seasonNumber, name: season.name });
}

/**
 * A series' page: its art and what it is about, then its seasons, each leading to its episodes; or,
 * with season chosen, that season's episodes. A series of one season shows its episodes at once.
 * Every season's episodes on one page, the seasons only headings, was too long a list to find one
 * in (the owner, 2026-10-07: series, then seasons, then episodes). The seasons are the groups of the
 * ordering the person watches the series in, which a series of more than one offers to change; the
 * episodes that ordering leaves out follow them (episode-orderings.md §4, §7 Q4).
 */
export function SeriesView(props: {
  readonly series: SeriesDetail;
  readonly season?: number | undefined;
  /** Told the series' match was corrected, an admin's (D301 §2): the page reads it again. */
  readonly onMatchCorrected?: () => void;
  /** Told the person chose another ordering: the page reads the series again, at its top. */
  readonly onOrderingChosen?: () => void;
}): JSX.Element {
  const { series } = props;
  const seasons = series.seasons ?? [];
  return (
    <article className="series">
      <div className="series__head">
        <Poster sourceSha256={series.art.posterSha256} />
        <div>
          <h1>{series.name}</h1>
          {series.year === undefined ? null : <p className="series__year">{series.year}</p>}
          {series.overview === undefined ? null : <p>{series.overview}</p>}
          <StatusNotes subject={series} />
          {props.season === undefined && series.nextUp !== undefined ? (
            <NextUpPlay
              nextUp={series.nextUp}
              ordering={series.ordering}
              isOnlyGroup={seasons.length === 1}
            />
          ) : null}
          {props.season === undefined ? (
            <OrderingChoice series={series} onChosen={props.onOrderingChosen} />
          ) : null}
          {props.onMatchCorrected === undefined ? null : (
            <MatchCorrection itemId={series.id} onCorrected={props.onMatchCorrected} />
          )}
          <FilingReportButton itemId={series.id} />
        </div>
      </div>
      <SeriesSeasons series={series} season={props.season} />
    </article>
  );
}

/**
 * Below a series' head: its seasons, each leading to its episodes, or the season chosen, or the one
 * season of a series that has one; then, on its top page, the episodes its ordering leaves out.
 */
function SeriesSeasons(props: {
  readonly series: SeriesDetail;
  readonly season: number | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { series } = props;
  const seasons = series.seasons ?? [];
  const shown =
    seasons.length === 1
      ? seasons[0]
      : seasons.find((season) => season.seasonNumber === props.season);
  const isTop = props.season === undefined || seasons.length === 1;
  return (
    <>
      {shown === undefined ? (
        <SeasonList series={series} seasons={seasons} />
      ) : (
        <section aria-labelledby={`season-${String(shown.seasonNumber)}`}>
          {seasons.length === 1 ? null : (
            <Link to={{ screen: "series-page", collectionId: series.id }}>
              {t("title.allSeasons")}
            </Link>
          )}
          <h2 id={`season-${String(shown.seasonNumber)}`}>{seasonName(t, series, shown)}</h2>
          <SeasonEpisodes seriesId={series.id} season={shown.seasonNumber} />
        </section>
      )}
      {isTop && (series.notInOrderingCount ?? 0) > 0 ? (
        <section aria-labelledby="not-in-order">
          <h2 id="not-in-order">{t("ordering.notInThisOrder")}</h2>
          <NotInOrderEpisodes seriesId={series.id} />
        </section>
      ) : null}
    </>
  );
}

/**
 * The series' next episode for this person, as the server says it (D160): played at once by its
 * button, "Continue S1E3" where they are partway through it, which resumes where they were, else
 * "Play S1E4"; or, when it cannot be played now, why, as its own page says and hides its Play
 * (D241 §3). The owner, 2026-10-25: a series' page is its next episode's.
 */
export function NextUpPlay(props: {
  readonly nextUp: NextUp;
  /** The ordering it is the next of, labelled in it; aired when absent. */
  readonly ordering?: string | undefined;
  readonly isOnlyGroup?: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const { nextUp } = props;
  const episode = placeCode(t, props.ordering, { ...nextUp, isOnlyGroup: props.isOnlyGroup });
  if (nextUp.unavailable !== undefined) {
    return (
      <p className="series__absent">
        {t("title.nextUpUnavailable", { episode, why: unavailableText(t, nextUp.unavailable) })}
      </p>
    );
  }
  return (
    <p>
      <Link to={{ screen: "play", fileId: nextUp.fileId }} className="title__play">
        {nextUp.isPartway
          ? t("title.continueEpisode", { episode })
          : t("title.playEpisode", { episode })}
      </Link>
    </p>
  );
}

/**
 * The orderings a series has, to choose the one the person watches it in (D159 §1): offered only when
 * it has more than one, aired first, each by its label with its kind and how many of the series'
 * episodes it holds (or its provider lists, before it holds any). A choice keeps it and goes to the
 * series' top page, where a season of the order left means nothing (episode-orderings.md §7 Q10);
 * one chosen before its contents were fetched says it is being fetched until it is (§4).
 */
export function OrderingChoice(props: {
  readonly series: SeriesDetail;
  readonly onChosen?: (() => void) | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { api, navigation } = useServices();
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  const id = useId();
  const { series } = props;
  const orderings = series.orderings ?? [];
  if (orderings.length < 2) {
    return null;
  }
  return (
    <p className="series__ordering">
      <label htmlFor={id}>{t("ordering.label")}</label>{" "}
      <select
        id={id}
        value={series.ordering ?? orderingAired}
        disabled={isBusy}
        onChange={(event) => {
          const chosen = event.target.value;
          runRequest(async () => {
            throwIfRefused(
              await api.PUT("/collections/{id}/preferences", {
                params: { path: { id: series.id } },
                body: { episodeOrdering: chosen },
              }),
            );
            navigation.go({ screen: "series-page", collectionId: series.id });
            props.onChosen?.();
          });
        }}
      >
        {orderings.map((ordering) => (
          <option key={ordering.name} value={ordering.name}>
            {orderingName(t, ordering)}
          </option>
        ))}
      </select>
      {orderings.find((ordering) => ordering.name === series.ordering)?.isAwaitingFetch === true ? (
        <span className="series__absent"> {t("ordering.fetching")}</span>
      ) : null}
      {problem === undefined ? null : (
        <span className="series__absent">{t("ordering.failed", { why: problem })}</span>
      )}
    </p>
  );
}

/** An ordering as the choice names it: "Aired", or "DVD Order (DVD) · episodes here: 23". */
export function orderingName(t: TFunction, ordering: Ordering): string {
  if (ordering.name === orderingAired) {
    return t("ordering.aired");
  }
  const kind =
    ordering.kind === undefined
      ? t("orderingKind.authored")
      : t(`orderingKind.${camelKey(ordering.kind)}`, { defaultValue: ordering.kind });
  const label = ordering.label ?? ordering.name;
  if (ordering.episodeCount === 0 && ordering.listedCount !== undefined) {
    return t("ordering.optionListed", { label, kind, count: ordering.listedCount });
  }
  return t("ordering.option", { label, kind, count: ordering.episodeCount });
}

/** A series' seasons, each with its first episode's still and how many episodes it has. */
function SeasonList(props: {
  readonly series: SeriesDetail;
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
              collectionId: props.series.id,
              season: season.seasonNumber,
            }}
            className="series__episode"
          >
            <Still sourceSha256={season.stillSha256} className="series__still" />
            <span>{seasonName(t, props.series, season)}</span>
            <span className="series__absent">
              {t("title.episodeCount", { count: season.episodeCount })}
            </span>
          </Link>
        </li>
      ))}
    </ol>
  );
}

/** An episode's id, by which a season shows an episode read twice once. */
const episodeId = (episode: Episode): number => episode.id;

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
  const pages = usePages(loadPage, useBackgroundChangesPoller(), episodeId);
  const watchItemAt = useItemsInView(pages.showRange);
  return (
    <SeasonView
      loadedEpisodes={pages.loadedSoFar}
      isLoadingMore={pages.isLoadingMore}
      onLoadMore={pages.loadMore}
      watchItemAt={watchItemAt}
    />
  );
}

/**
 * The episodes the ordering the person watches a series in leaves out, read a page at a time, each
 * labelled at its aired place: listed, so nothing disappears by a viewing choice (episode-orderings.md
 * §7 Q4).
 */
function NotInOrderEpisodes(props: { readonly seriesId: number }): JSX.Element {
  const { api } = useServices();
  const { seriesId } = props;
  const loadPage = useCallback(
    async (cursor: string): Promise<Page<Episode>> => {
      const query = { limit: episodesPerPage, ...(cursor === "" ? {} : { cursor }) };
      const notInPage = dataOrThrow(
        await api.GET("/collections/{id}/not-in-order", {
          params: { path: { id: seriesId }, query },
        }),
      );
      return { items: notInPage.episodes ?? [], nextCursor: notInPage.nextCursor ?? "" };
    },
    [api, seriesId],
  );
  const pages = usePages(loadPage, useBackgroundChangesPoller(), episodeId);
  const watchItemAt = useItemsInView(pages.showRange);
  return (
    <SeasonView
      loadedEpisodes={pages.loadedSoFar}
      isLoadingMore={pages.isLoadingMore}
      onLoadMore={pages.loadMore}
      watchItemAt={watchItemAt}
      isAtAiredPlaces
    />
  );
}

/** A season's pages as they show: loading, failed with why, or its episodes and a way to more. */
export function SeasonView(props: {
  readonly loadedEpisodes: Resource<Page<Episode>>;
  readonly isLoadingMore: boolean;
  readonly onLoadMore: () => void;
  readonly watchItemAt?: ((index: number) => (element: Element | null) => void) | undefined;
  /** Whether its episodes are labelled at their aired places, "S1E2", not by their number alone. */
  readonly isAtAiredPlaces?: boolean;
}): JSX.Element {
  return (
    <Loaded
      resource={props.loadedEpisodes}
      render={(page) => (
        <>
          <EpisodeList
            episodes={page.items}
            watchItemAt={props.watchItemAt}
            isAtAiredPlaces={props.isAtAiredPlaces}
          />
          {page.nextCursor === "" ? null : (
            <EndOfList onMore={props.onLoadMore} isLoadingMore={props.isLoadingMore} />
          )}
        </>
      )}
    />
  );
}

/** One season's episodes, each with its still, those not on disk or unavailable said so. */
function EpisodeList(props: {
  readonly episodes: readonly Episode[];
  readonly watchItemAt?: ((index: number) => (element: Element | null) => void) | undefined;
  readonly isAtAiredPlaces?: boolean | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <ol className="series__episodes">
      {props.episodes.map((episode, index) => (
        <li
          key={episode.id}
          ref={props.watchItemAt?.(index)}
          className={
            episode.hasLiveFile && episode.unavailable === undefined
              ? undefined
              : "series__item--absent"
          }
        >
          <Link to={{ screen: "title", itemId: episode.id }} className="series__episode">
            <Still sourceSha256={episode.stillSha256} className="series__still" />
            <span>
              {t("title.episodeLabel", {
                number:
                  props.isAtAiredPlaces === true
                    ? t("title.episodeCode", {
                        season: episode.seasonNumber,
                        number: episode.episodeNumber,
                      })
                    : episode.episodeNumber,
                name: episode.name,
              })}
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

/**
 * The page above a series' page: with a season chosen, the series' seasons, unless it has one season,
 * whose episodes its own page shows; else the series.
 */
export function seriesUp(id: number, season: number | undefined, series?: SeriesDetail): Route {
  const isOneSeason = series !== undefined && (series.seasons ?? []).length === 1;
  return season === undefined || isOneSeason
    ? { screen: "series" }
    : { screen: "series-page", collectionId: id };
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
  // A season's page is that season's look; a series of one season shows its episodes on the
  // series' own page, whose look is the series', the same episodes.
  usePageInterest(props.id, props.season);
  return (
    <>
      <BackButton
        up={seriesUp(props.id, props.season, series.state === "loaded" ? series.value : undefined)}
      />
      <Loaded
        resource={series}
        render={(value) => (
          <SeriesView
            series={value}
            season={props.season}
            onMatchCorrected={reload}
            onOrderingChosen={reload}
          />
        )}
      />
    </>
  );
}
