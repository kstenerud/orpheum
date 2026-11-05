// SPDX-License-Identifier: MPL-2.0

import "./poster-card.css";
import "./watched-mark.css";

import type { Api, components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { Icon } from "./icon.tsx";
import { dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";

/** A title's watched mark as it stood before a mark by hand, which Undo puts back (D370 §4). */
export type WatchedState = components["schemas"]["WatchedStateBody"];

/**
 * The watched check (D370 §2): a white tick, drawn by Icon, less intrusive than a colour ("Maybe a
 * white checkmark is less intrusive and would stop people complaining?", the owner). Decoration beside
 * words that say it.
 */
export function WatchedIcon(): JSX.Element {
  return (
    <Icon className="watched-icon">
      <path className="watched-icon__tick" d="M5 12.5l4.5 4.5L19 7.5" />
    </Icon>
  );
}

/**
 * The check on a row of a list that is not posters, a season's or an episode's (D370 §2): on the
 * dark patch a poster's tray draws it on, named "Watched" for a screen reader.
 */
export function WatchedRowMark(): JSX.Element {
  const { t } = useTranslation();
  return (
    <span className="watched-row-mark">
      <WatchedIcon />
      <span className="poster-cue__name">{t("watched.watched")}</span>
    </span>
  );
}

/** The check on a title's page, with the word, among its status notes (D370 §2). */
export function WatchedNote(): JSX.Element {
  const { t } = useTranslation();
  return (
    <p className="status-note">
      <span className="watched-row-mark">
        <WatchedIcon />
      </span>
      <span>{t("watched.watched")}</span>
    </p>
  );
}

/**
 * What a person marks by hand on a page (D370 §4): one film, episode or other video; a whole series,
 * every episode it has now in the ordering they watch it in; or one season of it. A series' or a
 * season's name says, to a screen reader, what its button marks.
 */
export type WatchedTarget =
  | { readonly kind: "title"; readonly id: number }
  | { readonly kind: "series"; readonly id: number; readonly name: string }
  | {
      readonly kind: "season";
      readonly id: number;
      readonly season: number;
      readonly name: string;
    };

/** Marks target watched, or unwatched, and returns each title's mark as it stood before. */
export async function markWatched(
  api: Api,
  target: WatchedTarget,
  isWatched: boolean,
): Promise<readonly WatchedState[]> {
  switch (target.kind) {
    case "title":
    case "series": {
      // One route marks an item of either kind (D425 §8).
      const options = { params: { path: { id: target.id } } };
      const answer = isWatched
        ? await api.PUT("/items/{id}/watched", options)
        : await api.DELETE("/items/{id}/watched", options);
      return dataOrThrow(answer).previous ?? [];
    }
    case "season": {
      const options = { params: { path: { id: target.id, group: target.season } } };
      const answer = isWatched
        ? await api.PUT("/items/{id}/groups/{group}/watched", options)
        : await api.DELETE("/items/{id}/groups/{group}/watched", options);
      return dataOrThrow(answer).previous ?? [];
    }
  }
}

/** Puts marks back as a mark by hand's answer gave them: its Undo. */
export async function restoreWatched(api: Api, states: readonly WatchedState[]): Promise<void> {
  throwIfRefused(await api.PUT("/me/watched", { body: { states: [...states] } }));
}

/** What a screen reader says a series' or a season's button marks; undefined for one title's. */
function labelOf(
  t: ReturnType<typeof useTranslation>["t"],
  target: WatchedTarget,
  isWatched: boolean,
): string | undefined {
  switch (target.kind) {
    case "title":
      return undefined;
    case "series":
      return isWatched
        ? t("watched.markSeriesUnwatchedLabel", { name: target.name })
        : t("watched.markSeriesWatchedLabel", { name: target.name });
    case "season":
      return isWatched
        ? t("watched.markSeasonUnwatchedLabel", { name: target.name })
        : t("watched.markSeasonWatchedLabel", { name: target.name });
  }
}

/** What a mark by hand did, said at once, and its Undo (D370 §4). */
function MarkedUndo(props: {
  readonly isMarkedWatched: boolean;
  readonly isBusy: boolean;
  readonly onUndo: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <output>
      {" "}
      {props.isMarkedWatched ? t("watched.markedWatched") : t("watched.markedUnwatched")}{" "}
      <button type="button" disabled={props.isBusy} onClick={props.onUndo}>
        {t("watched.undo")}
      </button>
    </output>
  );
}

/**
 * "Mark as watched", or "Mark as unwatched" once it is, on a title's, a season's or a series' page
 * (D370 §4): a button like any other, reached by a remote's arrows, and no question: the change shows
 * at once, with an Undo that puts every mark back as it was, as Continue Watching's Hide does (D188).
 */
export function MarkWatched(props: {
  readonly target: WatchedTarget;
  /** Whether it is watched now, as the page has it. */
  readonly isWatched: boolean;
  /** Told the marks changed: the page reads them again. */
  readonly onChanged: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  const [undoable, setUndoable] = useState<
    { readonly previous: readonly WatchedState[]; readonly isMarkedWatched: boolean } | undefined
  >(undefined);
  const { target, isWatched, onChanged } = props;
  const mark = (): void => {
    runRequest(async () => {
      try {
        const previous = await markWatched(api, target, !isWatched);
        setUndoable({ previous, isMarkedWatched: !isWatched });
      } finally {
        onChanged();
      }
    });
  };
  const undo = (previous: readonly WatchedState[]): void => {
    runRequest(async () => {
      try {
        await restoreWatched(api, previous);
        setUndoable(undefined);
      } finally {
        onChanged();
      }
    });
  };
  return (
    <p className="watched-marking">
      <button
        type="button"
        disabled={isBusy}
        aria-label={labelOf(t, target, isWatched)}
        onClick={mark}
      >
        {isWatched ? t("watched.markUnwatched") : t("watched.markWatched")}
      </button>
      {undoable === undefined ? null : (
        <MarkedUndo
          isMarkedWatched={undoable.isMarkedWatched}
          isBusy={isBusy}
          onUndo={() => {
            undo(undoable.previous);
          }}
        />
      )}
      {problem === undefined ? null : (
        <span className="watched-marking__failed"> {t("watched.failed", { why: problem })}</span>
      )}
    </p>
  );
}
