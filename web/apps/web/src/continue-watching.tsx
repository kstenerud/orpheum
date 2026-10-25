// SPDX-License-Identifier: MPL-2.0

import { BusyGroup } from "./busy-group.tsx";
import { useRequest } from "./use-request.ts";
import { idempotent } from "./idempotent.ts";
import "./continue-watching.css";

import type { Api, components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { UnplayableSign } from "./unplayable-sign.tsx";
import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow, throwIfRefused } from "./problem.ts";
import { Poster } from "./poster.tsx";
import { Link } from "./link.tsx";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";
import { placeCode, type OrderingPlace } from "./episode-place.ts";

/** One row of Continue Watching, as the server sends it. */
export type ContinueWatchingRow = components["schemas"]["InProgressBody"];

/** Reads the person's Continue Watching. */
export async function loadContinueWatching(api: Api): Promise<ContinueWatchingRow[]> {
  const home = dataOrThrow(await api.GET("/home"));
  return home.continueWatching ?? [];
}

/** Hides a work from Continue Watching when isHidden, or brings it back (D188). */
export async function setItemHidden(
  api: Api,
  item: { readonly itemId: number; readonly isHidden: boolean },
  idempotencyKey: string,
): Promise<void> {
  const { itemId, isHidden } = item;
  const options = { params: { path: { id: itemId } } };
  throwIfRefused(
    isHidden
      ? await api.POST("/items/{id}/hide", { ...options, ...idempotent(idempotencyKey) })
      : await api.DELETE("/items/{id}/hide", options),
  );
}

/** How much of the file is watched, as a whole percent, or undefined when its length is unknown. */
export function watchedPercent(
  row: Pick<ContinueWatchingRow, "positionMs" | "durationMs">,
): number | undefined {
  if (row.durationMs <= 0) {
    return undefined;
  }
  return Math.min(100, Math.round((row.positionMs * 100) / row.durationMs));
}

/** An episode's series, place and own name, for naming its row. */
export interface EpisodeNaming {
  readonly seriesName: string;
  readonly seasonNumber: number;
  readonly episodeNumber: number;
  readonly episodeName: string;
  /**
   * Its place in the ordering the person watches its series in, "Disc 2 · 3", when that is not
   * aired and holds it (episode-orderings.md §7 Q8); else the aired season and number name it.
   */
  readonly place?: string | undefined;
}

/** What a row is called: a film's name, or an episode's series, place and name. */
export function rowName(
  row: Pick<ContinueWatchingRow, "name" | "episode">,
  formatEpisodeName: (episode: EpisodeNaming) => string,
  formatPlace: (ordering: string, place: OrderingPlace) => string = () => "",
): string {
  if (row.episode === undefined) {
    return row.name;
  }
  const { seriesName, seasonNumber, episodeNumber, ordering, inOrdering } = row.episode;
  const place =
    ordering === undefined || inOrdering === undefined
      ? undefined
      : formatPlace(ordering, inOrdering);
  return formatEpisodeName({
    seriesName,
    seasonNumber,
    episodeNumber,
    episodeName: row.name,
    place,
  });
}

/**
 * The rows, each playing its file at once when chosen (D188), with a way to hide it; and after a
 * hide, a way to undo it.
 */
export function ContinueWatchingView(props: {
  readonly rows: readonly ContinueWatchingRow[];
  readonly hiddenName: string | undefined;
  readonly onHide: (row: ContinueWatchingRow) => void;
  readonly onUndo: () => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  if (props.rows.length === 0 && props.hiddenName === undefined) {
    return null;
  }
  const nameOf = (row: ContinueWatchingRow): string =>
    rowName(
      row,
      (episode) =>
        episode.place === undefined
          ? t("continueWatching.episode", { ...episode })
          : t("continueWatching.episodeInOrdering", { ...episode }),
      (ordering, place) => placeCode(t, ordering, place),
    );
  return (
    <section className="continue-watching" aria-labelledby="continue-watching-heading">
      <h2 id="continue-watching-heading">{t("continueWatching.heading")}</h2>
      {props.hiddenName === undefined ? null : (
        <output className="continue-watching__hidden">
          {t("continueWatching.hidden", { name: props.hiddenName })}{" "}
          <button type="button" onClick={props.onUndo}>
            {t("continueWatching.undo")}
          </button>
        </output>
      )}
      <ul className="continue-watching__rows">
        {props.rows.map((row) => (
          <ContinueWatchingRowView
            key={row.id}
            row={row}
            name={nameOf(row)}
            onHide={props.onHide}
          />
        ))}
      </ul>
    </section>
  );
}

/**
 * One row: its file played at once when chosen, or, when it cannot be played now, its page, which
 * says why (D241 §3); and a way to hide it.
 */
function ContinueWatchingRowView(props: {
  readonly row: ContinueWatchingRow;
  readonly name: string;
  readonly onHide: (row: ContinueWatchingRow) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { row } = props;
  const percent = watchedPercent(row);
  return (
    <li className="continue-watching__row">
      <Link
        to={
          row.unavailable === undefined
            ? { screen: "play", fileId: row.fileId }
            : { screen: "title", itemId: row.id }
        }
        className="continue-watching__play"
      >
        <Poster sourceSha256={row.posterSha256} />
        <span className="continue-watching__name">{props.name}</span>
        {row.unavailable === undefined ? null : (
          <span className="continue-watching__file">{t("title.unavailableNow")}</span>
        )}
        {row.fileName === undefined ? null : (
          <span className="continue-watching__file">{row.fileName}</span>
        )}
        {percent === undefined ? null : (
          <progress
            value={percent}
            max={100}
            aria-label={t("continueWatching.watched", { percent })}
          />
        )}
      </Link>
      {row.unavailable === undefined ? null : <UnplayableSign unavailable={row.unavailable} />}
      <button
        type="button"
        onClick={() => {
          props.onHide(row);
        }}
        aria-label={t("continueWatching.hideLabel", { name: props.name })}
      >
        {t("continueWatching.hide")}
      </button>
    </li>
  );
}

/** Continue Watching, loaded from the server, with hide and undo. */
export function ContinueWatching(): JSX.Element | null {
  const { api } = useServices();
  const [rows, reload] = useResource(
    "continue-watching",
    () => loadContinueWatching(api),
    useBackgroundChangesPoller(),
  );
  const [hiddenRow, setHiddenRow] = useState<ContinueWatchingRow | undefined>(undefined);
  // A refusal shows the rows as the server has them, as success does.
  const hiding = useRequest(() => "");
  if (rows.state !== "loaded") {
    return null; // the lists below say when the server cannot be reached
  }
  const setRowHidden = (row: ContinueWatchingRow, isHidden: boolean): void => {
    hiding.runRequest(async (idempotencyKey) => {
      try {
        await setItemHidden(api, { itemId: row.id, isHidden }, idempotencyKey);
        setHiddenRow(isHidden ? row : undefined);
      } finally {
        reload(); // a refusal shows the row as the server has it
      }
    });
  };
  return (
    <BusyGroup isBusy={hiding.isBusy}>
      <ContinueWatchingView
        rows={rows.value}
        hiddenName={hiddenRow?.name}
        onHide={(row) => {
          setRowHidden(row, true);
        }}
        onUndo={() => {
          if (hiddenRow !== undefined) {
            setRowHidden(hiddenRow, false);
          }
        }}
      />
    </BusyGroup>
  );
}
