// SPDX-License-Identifier: MPL-2.0

import { BusyGroup } from "./busy-group.tsx";
import { useRequest } from "./use-request.ts";
import { idempotent } from "./idempotent.ts";
import "./continue-watching.css";

import type { Api, components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { MoreActions } from "./more-actions.tsx";
import { StatusTray } from "./status-cues.tsx";
import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow, throwIfRefused } from "./problem.ts";
import { PosterCard, PosterDetail } from "./poster-card.tsx";
import { PosterRow } from "./poster-row.tsx";
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
      <PosterRow isArrival>
        {props.rows.map((row) => (
          <ContinueWatchingRowView
            key={row.id}
            row={row}
            name={nameOf(row)}
            onHide={props.onHide}
          />
        ))}
      </PosterRow>
    </section>
  );
}

/**
 * One row: its file played at once when chosen, or, when it cannot be played now, its page, which
 * says why (D241 §3); and Hide in the card's own "…" (more-actions.tsx), over its top-right, Up from
 * the card, so Down leaves the row in one press (ui-audit-2026-11-04.md §3, Home).
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
    <PosterCard
      to={
        row.unavailable === undefined
          ? { screen: "play", fileId: row.fileId }
          : { screen: "title", itemId: row.id }
      }
      posterSha256={row.posterSha256}
      name={props.name}
      details={
        <>
          {row.unavailable === undefined ? null : (
            <PosterDetail>{t("title.unavailableNow")}</PosterDetail>
          )}
          {row.fileName === undefined ? null : <PosterDetail>{row.fileName}</PosterDetail>}
        </>
      }
      progress={
        percent === undefined
          ? undefined
          : { percent, label: t("continueWatching.watched", { percent }) }
      }
      cues={<StatusTray subject={row} />}
      actions={
        <MoreActions
          label={t("continueWatching.more", { name: props.name })}
          className="continue-watching__more"
          actions={[
            {
              key: "hide",
              label: t("continueWatching.hide"),
              onChoose: () => {
                props.onHide(row);
              },
            },
          ]}
        />
      }
    />
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
