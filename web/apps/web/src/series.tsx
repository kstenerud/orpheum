// SPDX-License-Identifier: MPL-2.0

import "./series.css";

import type { components } from "@orpheum/sdk";
import { use, useCallback, useEffect, useRef, useState, type JSX, type MouseEvent } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { episodeReachedOf, letGoOfEpisodeReached, type EpisodeReached } from "./episode-reached.ts";
import { restoreWhenDrawn } from "./focus-memory.ts";
import { focusAndShow } from "./focus-navigation.ts";
import { routeHref } from "./router.ts";
import { UpTrail, useLibraryContext } from "./up-trail.tsx";
import { libraryCrumb } from "./upward.ts";
import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow, problemText, throwIfRefused, unavailableText } from "./problem.ts";
import { seriesOrThrow } from "./item-detail.ts";
import { identifiedCorrectlyAction, identifiedIncorrectlyAction } from "./match-correction.tsx";
import { ModalDialog } from "./modal-dialog.tsx";
import { MoreActions, type MoreAction } from "./more-actions.tsx";
import { IsAdminContext } from "./warning-sign.tsx";
import { StatusNotes, StatusTray } from "./status-cues.tsx";
import { TitlePeople } from "./credits.tsx";
import { PosterCard, PosterDetail } from "./poster-card.tsx";
import { PosterRow } from "./poster-row.tsx";
import { Poster, Still } from "./poster.tsx";
import { Link } from "./link.tsx";
import { ActionChooser, currentLabelOf, type ActionChoice } from "./route-chooser.tsx";
import { usePageInterest } from "./page-interest.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { TitleHead } from "./title-head.tsx";
import { rememberedInTheTab, useRemembered } from "./remembered.ts";
import { useResource, type Resource } from "./use-resource.ts";
import { usePages, type Page } from "./use-pages.ts";
import { EndOfList } from "./end-of-list.tsx";
import { useItemsInView } from "./items-in-view.ts";
import { groupName, orderingAired, placeCode } from "./episode-place.ts";
import { camelKey } from "./catalogue-key.ts";
import { useRequest } from "./use-request.ts";
import { useWarmsUpOnPress } from "./warm-on-press.ts";
import { MarkWatched, WatchedRowMark } from "./watched-mark.tsx";

/** A series with its seasons, as the server sends it; each season's episodes are read apart. */
export type SeriesDetail = components["schemas"]["SeriesBody"];
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
 * A series' page, one page whatever season it shows (D367 §9): its first screen in the title template
 * (title-head.tsx, ui-audit-2026-11-04.md R8), its actions row Continue or Play its next episode
 * first, the page's arrival, then Mark as watched and the "…"; then its seasons as a row of posters
 * (D379), then one season's episodes, the season chosen in the row, which
 * changes the view and adds no step to the history; with none chosen, the season of the person's next episode, else the first. Every season's
 * episodes on one page, the seasons only headings, was too long a list to find one in (the owner,
 * 2026-10-07). The seasons are the groups of the ordering the person watches the series in, which a
 * series of more than one offers to change, under the "…" of what is rarely used; the episodes that
 * ordering leaves out follow them (episode-orderings.md §4, §7 Q4).
 */
export function SeriesView(props: {
  readonly series: SeriesDetail;
  /** The season chosen, by the address or by an episode returned from; undefined for the default. */
  readonly season?: number | undefined;
  /** Told the series' match was corrected, an admin's (D301 §2): the page reads it again. */
  readonly onMatchCorrected?: () => void;
  /** Told the person chose another ordering: the page reads the series again, at its top. */
  readonly onOrderingChosen?: () => void;
  /** Told its episodes were marked watched or unwatched by hand (D370 §4): the page reads it again. */
  readonly onWatchedChanged?: () => void;
}): JSX.Element {
  const { series } = props;
  const seasons = series.seasons ?? [];
  const [marks, onWatchedChanged] = useMarksCounted(props.onWatchedChanged);
  const shown = shownSeasonOf(series, props.season);
  // One that cannot be played now is said among the notes; one that can, the actions row's first.
  const { nextUpPlay, isNextUpPlayable, warmsUp } = useNextUpPlay(series, seasons.length === 1);
  return (
    <article className="series" onClickCapture={warmsUp}>
      <TitleHead
        backdropSha256={series.art.backdropSha256}
        art={<Poster sourceSha256={series.art.posterSha256} />}
        name={series.name}
        facts={series.year === undefined ? null : <span>{series.year}</span>}
        notes={
          <>
            <StatusNotes subject={series} />
            {isNextUpPlayable ? null : nextUpPlay}
            <OrderingFetching series={series} />
          </>
        }
        actions={
          <>
            {isNextUpPlayable ? nextUpPlay : null}
            <SeriesMark series={series} onChanged={onWatchedChanged} />
          </>
        }
        hasMainAction={isNextUpPlayable}
        more={
          <SeriesMoreActions
            series={series}
            onMatchCorrected={props.onMatchCorrected}
            onOrderingChosen={props.onOrderingChosen}
          />
        }
        overview={series.overview}
      />
      {shown === undefined || seasons.length === 1 ? null : (
        <SeasonRow series={series} shown={shown} />
      )}
      <SeriesSeasons
        series={series}
        season={props.season}
        marks={marks}
        onWatchedChanged={onWatchedChanged}
      />
      <TitlePeople credits={series.credits} />
    </article>
  );
}

/**
 * A series' rarely used actions, under its "…" (the owner, 2026-10-29): the order its episodes are
 * shown in, where it has more than one, and an admin's "Identified incorrectly", and, while its match
 * is a guess, "Identified correctly" (D301 §4).
 */
function SeriesMoreActions(props: {
  readonly series: SeriesDetail;
  readonly onMatchCorrected: (() => void) | undefined;
  readonly onOrderingChosen: (() => void) | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const isAdmin = use(IsAdminContext);
  const { series, onMatchCorrected } = props;
  const actions: MoreAction[] = [];
  if ((series.orderings ?? []).length > 1) {
    actions.push({
      key: "ordering",
      label: t("ordering.label"),
      dialog: (close) => (
        <OrderingDialog series={series} onClose={close} onChosen={props.onOrderingChosen} />
      ),
    });
  }
  if (isAdmin) {
    const onCorrected = onMatchCorrected ?? (() => undefined);
    actions.push(
      identifiedIncorrectlyAction(t, {
        itemId: series.id,
        isWork: onMatchCorrected !== undefined,
        onCorrected,
      }),
    );
    if (series.isGuessed === true) {
      actions.push(identifiedCorrectlyAction(t, { itemId: series.id, onConfirmed: onCorrected }));
    }
  }
  return <MoreActions actions={actions} />;
}

/** The dialog the "…"'s Order opens: the choice of ordering, closed once one is chosen. */
function OrderingDialog(props: {
  readonly series: SeriesDetail;
  readonly onClose: () => void;
  readonly onChosen: (() => void) | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const heading = t("ordering.label");
  return (
    <ModalDialog label={heading} onClose={props.onClose}>
      <h2>{heading}</h2>
      <OrderingChoice
        series={props.series}
        onChosen={() => {
          props.onClose();
          props.onChosen?.();
        }}
      />
    </ModalDialog>
  );
}

/**
 * That the ordering chosen is being fetched, on the page, until it is (episode-orderings.md §4):
 * its choice is in a dialog gone once chosen.
 */
function OrderingFetching(props: { readonly series: SeriesDetail }): JSX.Element | null {
  const { t } = useTranslation();
  const { series } = props;
  const chosen = (series.orderings ?? []).find((ordering) => ordering.name === series.ordering);
  return chosen?.isAwaitingFetch === true ? (
    <p className="series__absent">{t("ordering.fetching")}</p>
  ) : null;
}

/**
 * How many marks by hand a series' page has made, and what it is told of each: a mark changes the
 * episodes' rows too, so each reads the season's episodes again (D370 §4). Undefined is no marking.
 */
function useMarksCounted(
  onWatchedChanged: (() => void) | undefined,
): readonly [number, (() => void) | undefined] {
  const [marks, setMarks] = useState(0);
  if (onWatchedChanged === undefined) {
    return [marks, undefined];
  }
  return [
    marks,
    (): void => {
      setMarks((count) => count + 1);
      onWatchedChanged();
    },
  ];
}

/** A whole series' Mark as watched, in its head (D370 §4). */
function SeriesMark(props: {
  readonly series: SeriesDetail;
  readonly onChanged: (() => void) | undefined;
}): JSX.Element | null {
  const { series, onChanged } = props;
  return onChanged === undefined ? null : (
    <MarkWatched
      target={{ kind: "series", id: series.id, name: series.name }}
      isWatched={series.isWatched === true}
      onChanged={onChanged}
    />
  );
}

/**
 * A season's Mark as watched, over the season the page shows (D370 §4); none for a series of one
 * season, marked whole from its head.
 */
function SeasonMark(props: {
  readonly series: SeriesDetail;
  readonly season: Season;
  readonly onChanged: (() => void) | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { series, season, onChanged } = props;
  if ((series.seasons ?? []).length === 1 || onChanged === undefined) {
    return null;
  }
  return (
    <MarkWatched
      target={{
        kind: "season",
        id: series.id,
        season: season.seasonNumber,
        name: seasonName(t, series, season),
      }}
      isWatched={season.isWatched === true}
      onChanged={onChanged}
    />
  );
}

/**
 * The season a series' page shows: the one chosen, where the series has it; else the season of the
 * person's next episode; else the first. Undefined for a series with no season.
 */
function shownSeasonOf(series: SeriesDetail, chosen: number | undefined): Season | undefined {
  const seasons = series.seasons ?? [];
  const numbered = (number: number | undefined): Season | undefined =>
    seasons.find((season) => season.seasonNumber === number);
  return numbered(chosen) ?? numbered(series.nextUp?.groupNumber) ?? seasons[0];
}

/**
 * Below the row of seasons: the season shown, chosen in the row, with its Mark as watched at the
 * heading's end, out of the way of Down from the row into the episodes (D379 §3); then the episodes
 * its ordering leaves out.
 */
function SeriesSeasons(props: {
  readonly series: SeriesDetail;
  readonly season: number | undefined;
  /** How many marks by hand the page has made, by which its episodes are read again. */
  readonly marks: number;
  readonly onWatchedChanged: (() => void) | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { series } = props;
  const shown = shownSeasonOf(series, props.season);
  return (
    <>
      {shown === undefined ? null : (
        <section aria-labelledby={`season-${String(shown.seasonNumber)}`}>
          <div className="series__season-head">
            <h2 id={`season-${String(shown.seasonNumber)}`}>{seasonName(t, series, shown)}</h2>
            <SeasonMark series={series} season={shown} onChanged={props.onWatchedChanged} />
          </div>
          <SeasonEpisodes key={props.marks} seriesId={series.id} season={shown.seasonNumber} />
        </section>
      )}
      {(series.notInOrderingCount ?? 0) > 0 ? (
        <section aria-labelledby="not-in-order">
          <h2 id="not-in-order">{t("ordering.notInThisOrder")}</h2>
          <NotInOrderEpisodes seriesId={series.id} />
        </section>
      ) : null}
    </>
  );
}

/**
 * A series' next episode's Play (NextUpPlay), where it has one; whether it can be played now; and
 * what warms the episode up as that Play is pressed, for the page to hear (warm-on-press.ts).
 */
function useNextUpPlay(
  series: SeriesDetail,
  isOnlyGroup: boolean,
): {
  readonly nextUpPlay: JSX.Element | null;
  readonly isNextUpPlayable: boolean;
  readonly warmsUp: (event: MouseEvent<HTMLElement>) => void;
} {
  const { nextUp } = series;
  const isNextUpPlayable = nextUp !== undefined && nextUp.unavailable === undefined;
  const warmsUp = useWarmsUpOnPress((fileId) => isNextUpPlayable && nextUp.fileId === fileId);
  const nextUpPlay =
    nextUp === undefined ? null : (
      <NextUpPlay nextUp={nextUp} ordering={series.ordering} isOnlyGroup={isOnlyGroup} />
    );
  return { nextUpPlay, isNextUpPlayable, warmsUp };
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
    <Link to={{ screen: "play", fileId: nextUp.fileId }} className="title__play">
      {nextUp.isPartway
        ? t("title.continueEpisode", { episode })
        : t("title.playEpisode", { episode })}
    </Link>
  );
}

/**
 * The orderings a series has, to choose the one the person watches it in (D159 §1): offered only when
 * it has more than one, aired first, each by its label with its kind and how many of the series'
 * episodes it holds (or its provider lists, before it holds any). A choice keeps it and shows the
 * series at its default season, a season of the order left meaning nothing (episode-orderings.md §7
 * Q10); one chosen before its contents were fetched is said to be fetched on the page
 * (OrderingFetching, §4).
 */
export function OrderingChoice(props: {
  readonly series: SeriesDetail;
  readonly onChosen?: (() => void) | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { api, navigation } = useServices();
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  const { series } = props;
  const orderings = series.orderings ?? [];
  if (orderings.length < 2) {
    return null;
  }
  const choose = (chosen: string): void => {
    if (isBusy) {
      return;
    }
    runRequest(async () => {
      throwIfRefused(
        await api.PUT("/items/{id}/preferences", {
          params: { path: { id: series.id } },
          body: { episodeOrdering: chosen },
        }),
      );
      // Another ordering is a view of the same series: no step in the history (D367 §9).
      navigation.replace({ screen: "series-page", collectionId: series.id });
      props.onChosen?.();
    });
  };
  const shown = series.ordering ?? orderingAired;
  const options = orderings.map((ordering): ActionChoice => ({
    key: ordering.name,
    label: orderingName(t, ordering),
    isCurrent: ordering.name === shown,
    onChoose: () => {
      choose(ordering.name);
    },
  }));
  return (
    <div className="series__ordering">
      <ActionChooser
        fieldLabel={t("ordering.label")}
        current={currentLabelOf(options)}
        options={options}
      />
      {problem === undefined ? null : (
        <span className="series__absent">{t("ordering.failed", { why: problem })}</span>
      )}
    </div>
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

/**
 * The poster a season's card shows: its own (D379 §1), else, where it has no still to show in its
 * place, its series'; undefined shows its still, or a named blank where it has none either.
 */
function seasonPosterOf(series: SeriesDetail, season: Season): string | undefined {
  return (
    season.posterSha256 ?? (season.stillSha256 === undefined ? series.art.posterSha256 : undefined)
  );
}

/**
 * A series' seasons as a row of posters above the episodes (D379 §1), in the server's order, the
 * specials last; each by its name and how many episodes it holds, with the check once its person
 * watched every one (D370 §3). A season shows its own poster; one with none shows its still in its
 * poster's place (D362 §2), with no "No poster" caption, else the series' poster. Choosing one is a
 * view of the series, which replaces the history's entry (D379 §2, D367 §9). The row is one focus
 * container: a remote's Left and Right move along it, Down leaves it for the season's episodes (D379
 * §3).
 */
function SeasonRow(props: { readonly series: SeriesDetail; readonly shown: Season }): JSX.Element {
  const { t } = useTranslation();
  const { series, shown } = props;
  const rowRef = useRef<HTMLUListElement>(null);
  // The season shown in view, where the row scrolls sideways past it: the row alone scrolls, as
  // scrollIntoView would scroll the page down to the row too, past the series' name above it on a
  // short screen (the Fire TV, 2026-11-02).
  useEffect(() => {
    const row = rowRef.current;
    const current = row?.querySelector('[aria-current="page"]');
    if (row === null || !(current instanceof HTMLElement)) {
      return;
    }
    const [rowBox, box] = [row.getBoundingClientRect(), current.getBoundingClientRect()];
    if (box.left < rowBox.left) {
      row.scrollLeft -= rowBox.left - box.left;
    } else if (box.right > rowBox.right) {
      row.scrollLeft += box.right - rowBox.right;
    }
  }, [shown.seasonNumber]);
  return (
    <PosterRow listRef={rowRef} className="series__seasons" label={t("title.seasons")}>
      {(series.seasons ?? []).map((season) => (
        <PosterCard
          key={season.seasonNumber}
          to={{ screen: "series-page", collectionId: series.id, season: season.seasonNumber }}
          isCurrent={season.seasonNumber === shown.seasonNumber}
          isReplacing
          posterSha256={seasonPosterOf(series, season)}
          noPosterStillSha256={season.stillSha256}
          isStillEnough
          name={seasonName(t, series, season)}
          details={
            <PosterDetail>
              {t("title.seasonEpisodeCount", { count: season.episodeCount })}
            </PosterDetail>
          }
          cues={<StatusTray subject={{ isWatched: season.isWatched }} />}
        />
      ))}
    </PosterRow>
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
        await api.GET("/items/{id}/groups/{group}", {
          params: { path: { id: seriesId, group: season }, query },
        }),
      );
      return { items: seasonPage.episodes ?? [], nextCursor: seasonPage.nextCursor ?? "" };
    },
    [api, seriesId, season],
  );
  const pages = usePages(loadPage, useBackgroundChangesPoller(), { idOf: episodeId });
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
        await api.GET("/items/{id}/not-in-order", {
          params: { path: { id: seriesId }, query },
        }),
      );
      return { items: notInPage.episodes ?? [], nextCursor: notInPage.nextCursor ?? "" };
    },
    [api, seriesId],
  );
  const pages = usePages(loadPage, useBackgroundChangesPoller(), { idOf: episodeId });
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

/**
 * One season's episodes, each with its still, those not on disk or unavailable said so, and the
 * check on each its person watched (D370 §2), over its still's corner as on a poster's tray (the
 * owner, 2026-10-28: "I think the checkmark should be over the screenshot").
 */
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
            <span className="series__still-frame">
              <Still sourceSha256={episode.stillSha256} className="series__still" />
              {episode.isWatched === true ? <WatchedRowMark /> : null}
            </span>
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

/** The series' pages this tab has shown. */
const rememberedSeries = rememberedInTheTab<SeriesDetail>();

/**
 * Returning to a series from one of its episodes (D367 §9): the season of the episode last reached,
 * its page or its playing, in place of the season the entry held, and that episode focused once
 * drawn; the season the address names otherwise. Returns the season to show.
 */
function useSeasonReturnedTo(seriesId: number, season: number | undefined): number | undefined {
  const { navigation } = useServices();
  const [arrival] = useState<EpisodeReached | undefined>(() => episodeReachedOf(seriesId));
  // The arrival's season, shown until the address holds it; then the address's, the row's.
  const [arriving, setArriving] = useState(arrival);
  if (arriving !== undefined && arriving.season === season) {
    setArriving(undefined);
  }
  useEffect(() => {
    if (arrival === undefined) {
      return undefined;
    }
    letGoOfEpisodeReached(seriesId, arrival);
    const at = arrival.season;
    navigation.replace(
      at === undefined
        ? { screen: "series-page", collectionId: seriesId }
        : { screen: "series-page", collectionId: seriesId, season: at },
    );
    const episode = routeHref({ screen: "title", itemId: arrival.episodeId });
    return restoreWhenDrawn({ href: episode, ordinal: 0 }, document.body, (element) => {
      focusAndShow(element, false);
    });
  }, [arrival, seriesId, navigation]);
  return arriving === undefined ? season : arriving.season;
}

/** A series' page, loaded by its id, at the season chosen or returned to. */
export function Series(props: {
  readonly id: number;
  readonly season?: number | undefined;
}): JSX.Element {
  const { api } = useServices();
  const [loaded, reload] = useResource(
    `series/${String(props.id)}`,
    async () => {
      return seriesOrThrow(
        dataOrThrow(
          await api.GET("/items/{id}", {
            params: { path: { id: props.id } },
          }),
        ),
      );
    },
    useBackgroundChangesPoller(),
  );
  const series = useRemembered(`series/${String(props.id)}`, loaded, rememberedSeries);
  const season = useSeasonReturnedTo(props.id, props.season);
  // A season chosen is that season's look; the default the series' own.
  usePageInterest(props.id, season);
  const context = useLibraryContext();
  const value = series.state === "loaded" ? series.value : undefined;
  return (
    <>
      <UpTrail
        upward={{
          crumbs: [libraryCrumb("series", value?.libraries ?? undefined, context)],
          here: value?.name ?? "",
        }}
      />
      <Loaded
        resource={series}
        render={(shown) => (
          <SeriesView
            series={shown}
            season={season}
            onMatchCorrected={reload}
            onOrderingChosen={reload}
            onWatchedChanged={reload}
          />
        )}
      />
    </>
  );
}
