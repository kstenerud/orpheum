// SPDX-License-Identifier: MPL-2.0

import type { Api, components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { dataOrThrow, throwIfRefused } from "./problem.ts";
import { Poster } from "./poster.tsx";
import { Link } from "./link.tsx";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";

/** One row of Continue Watching, as the server sends it. */
export type InProgress = components["schemas"]["InProgressBody"];

/** Reads the person's Continue Watching. */
export async function loadContinueWatching(api: Api): Promise<InProgress[]> {
  const data = dataOrThrow(await api.GET("/home"));
  return data.continueWatching ?? [];
}

/** Hides a work from Continue Watching, or with isUndo, brings it back (D188). */
export async function changeHidden(api: Api, itemId: number, isUndo: boolean): Promise<void> {
  const options = { params: { path: { id: itemId } } };
  throwIfRefused(
    isUndo
      ? await api.DELETE("/items/{id}/hide", options)
      : await api.POST("/items/{id}/hide", options),
  );
}

/** How much of the file is watched, as a whole percent, or undefined when its length is unknown. */
export function watchedPercent(
  row: Pick<InProgress, "positionMs" | "durationMs">,
): number | undefined {
  if (row.durationMs <= 0) {
    return undefined;
  }
  return Math.min(100, Math.round((row.positionMs * 100) / row.durationMs));
}

/** An episode's series, place and own name, for naming its row. */
export interface EpisodeNaming {
  readonly series: string;
  readonly season: number;
  readonly number: number;
  readonly episode: string;
}

/** What a row is called: a film's name, or an episode's series, place and name. */
export function rowName(
  row: Pick<InProgress, "name" | "episode">,
  episodeName: (episode: EpisodeNaming) => string,
): string {
  if (row.episode === undefined) {
    return row.name;
  }
  const { seriesName, seasonNumber: season, episodeNumber: number } = row.episode;
  return episodeName({ series: seriesName, season, number, episode: row.name });
}

/**
 * The rows, each playing its file at once when chosen (D188), with a way to hide it; and after a
 * hide, a way to undo it.
 */
export function ContinueWatchingView(props: {
  readonly rows: readonly InProgress[];
  readonly hidden: string | undefined;
  readonly onHide: (row: InProgress) => void;
  readonly onUndo: () => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  if (props.rows.length === 0 && props.hidden === undefined) {
    return null;
  }
  const name = (row: InProgress): string =>
    rowName(row, (episode) => t("continueWatching.episode", { ...episode }));
  return (
    <section className="continue-watching" aria-labelledby="continue-watching-heading">
      <h2 id="continue-watching-heading">{t("continueWatching.heading")}</h2>
      {props.hidden === undefined ? null : (
        <output className="continue-watching__hidden">
          {t("continueWatching.hidden", { name: props.hidden })}{" "}
          <button type="button" onClick={props.onUndo}>
            {t("continueWatching.undo")}
          </button>
        </output>
      )}
      <ul className="continue-watching__rows">
        {props.rows.map((row) => {
          const percent = watchedPercent(row);
          return (
            <li key={row.id} className="continue-watching__row">
              <Link to={{ screen: "play", fileId: row.fileId }} className="continue-watching__play">
                <Poster hash={row.posterSha256} />
                <span className="continue-watching__name">{name(row)}</span>
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
              <button
                type="button"
                onClick={() => {
                  props.onHide(row);
                }}
                aria-label={t("continueWatching.hideLabel", { name: name(row) })}
              >
                {t("continueWatching.hide")}
              </button>
            </li>
          );
        })}
      </ul>
    </section>
  );
}

/** Continue Watching, loaded from the server, with hide and undo. */
export function ContinueWatching(): JSX.Element | null {
  const { api } = useServices();
  const [rows, reload] = useResource("continue-watching", () => loadContinueWatching(api));
  const [hiddenRow, setHiddenRow] = useState<InProgress | undefined>(undefined);
  if (rows.state !== "loaded") {
    return null; // the lists below say when the server cannot be reached
  }
  const change = (row: InProgress, isUndo: boolean): void => {
    changeHidden(api, row.id, isUndo).then(
      () => {
        setHiddenRow(isUndo ? undefined : row);
        reload();
      },
      () => {
        reload();
      },
    );
  };
  return (
    <ContinueWatchingView
      rows={rows.value}
      hidden={hiddenRow?.name}
      onHide={(row) => {
        change(row, false);
      }}
      onUndo={() => {
        if (hiddenRow !== undefined) {
          change(hiddenRow, true);
        }
      }}
    />
  );
}
