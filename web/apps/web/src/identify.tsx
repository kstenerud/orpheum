// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { idempotent } from "./idempotent.ts";
import { dateTimeText } from "./date-time-text.ts";
import { arrivalMark } from "./focus-navigation.ts";
import { i18n } from "./i18n.ts";
import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow } from "./problem.ts";
import { Link } from "./link.tsx";
import { itemRoute } from "./router.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./loaded.tsx";
import { CandidateChooser, identifyProblemText } from "./candidate-chooser.tsx";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";

/** A film or series waiting for a match, as the server sends it. */
export type WaitingItem = components["schemas"]["ItemBody"];
/** Which items to show: those with several matches, or those with none. */
export type WaitingFilter = "ambiguous" | "unidentified";

/**
 * The items waiting, each with a way to choose its match or have it looked up again; the first
 * item's is where the page's focus arrives (ui-audit-2026-11-04.md §3, Needs a match).
 */
export function WaitingList(props: {
  readonly items: readonly WaitingItem[];
  readonly waitingFilter: WaitingFilter;
  readonly onChanged: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  if (props.items.length === 0) {
    return <p>{t("identify.none")}</p>;
  }
  return (
    <ul>
      {props.items.map((item, index) => (
        <li key={item.id}>
          <WaitingRow
            item={item}
            waitingFilter={props.waitingFilter}
            isArrival={index === 0}
            onChanged={props.onChanged}
          />
        </li>
      ))}
    </ul>
  );
}

/**
 * One waiting item: its name and year, and what can be done about it; its main action, Choose its
 * match where it has several, else Look again, is the page's arrival where isArrival.
 */
function WaitingRow(props: {
  readonly item: WaitingItem;
  readonly waitingFilter: WaitingFilter;
  readonly isArrival: boolean;
  readonly onChanged: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { item } = props;
  const [isChoosing, setIsChoosing] = useState(false);
  const { note, refresh, refreshing } = useLookAgain(item);
  const isChoosable = props.waitingFilter === "ambiguous";
  const arrival = props.isArrival ? arrivalMark : {};
  return (
    <div>
      <Link to={itemRoute(item.kind, item.id)}>{item.name}</Link>{" "}
      {item.year === undefined ? null : <span>{item.year}</span>}{" "}
      {isChoosable ? (
        <button
          type="button"
          {...arrival}
          aria-expanded={isChoosing}
          onClick={() => {
            setIsChoosing(!isChoosing);
          }}
        >
          {t("identify.choose")}
        </button>
      ) : null}{" "}
      <button
        type="button"
        {...(isChoosable ? {} : arrival)}
        disabled={refreshing.isBusy}
        onClick={refresh}
      >
        {t("identify.refresh")}
      </button>
      {note === "" ? null : <output>{note}</output>}
      {refreshing.problem === undefined ? null : <output>{refreshing.problem}</output>}
      {isChoosing ? <CandidateChooser itemId={item.id} onChosen={props.onChanged} /> : null}
    </div>
  );
}

/** Looking an item up again, and what the last look said. */
function useLookAgain(item: WaitingItem): {
  readonly note: string;
  readonly refresh: () => void;
  readonly refreshing: ReturnType<typeof useRequest>;
} {
  const { t } = useTranslation();
  const { api } = useServices();
  const [note, setNote] = useState("");
  const refreshing = useRequest((cause) => identifyProblemText(t, cause));
  const refresh = (): void => {
    setNote("");
    refreshing.runRequest(async (idempotencyKey) => {
      const answer = dataOrThrow(
        await api.POST("/items/{id}/refresh", {
          params: { path: { id: item.id } },
          ...idempotent(idempotencyKey),
        }),
      );
      // While TMDB is not answering, the refresh says what it waits for, and since when (D231 §12).
      const waiting = answer.waitingFor;
      setNote(
        waiting === undefined
          ? t("identify.refreshed")
          : t("identify.refreshedWaiting", {
              name: waiting.name,
              since: dateTimeText(waiting.downAt, i18n.language),
            }),
      );
    });
  };
  return { note, refresh, refreshing };
}

/** The two lists of what waits, by why. */
interface WaitingLists {
  readonly ambiguous: readonly WaitingItem[];
  readonly unidentified: readonly WaitingItem[];
}

/**
 * The tab to show: the one chosen, or, until one is, the first with anything in it, so the page
 * never opens on an empty list while the other has what the count said (the owner, 2026-10-21).
 */
export function shownFilter(chosen: WaitingFilter | undefined, lists: WaitingLists): WaitingFilter {
  if (chosen !== undefined) {
    return chosen;
  }
  return lists.ambiguous.length === 0 && lists.unidentified.length > 0
    ? "unidentified"
    : "ambiguous";
}

/** The tabs, each with how many it holds, and the list of the one shown. */
export function WaitingTabs(props: {
  readonly lists: WaitingLists;
  readonly chosen: WaitingFilter | undefined;
  readonly onChoose: (filter: WaitingFilter) => void;
  readonly onChanged: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const shown = shownFilter(props.chosen, props.lists);
  return (
    <>
      <fieldset>
        <legend>{t("common.show")}</legend>
        {(["ambiguous", "unidentified"] as const).map((choice) => (
          <button
            key={choice}
            type="button"
            aria-pressed={shown === choice}
            onClick={() => {
              props.onChoose(choice);
            }}
          >
            {t(`identify.${choice}`, { count: props.lists[choice].length })}
          </button>
        ))}
      </fieldset>
      <WaitingList items={props.lists[shown]} waitingFilter={shown} onChanged={props.onChanged} />
    </>
  );
}

/** Needs a match: the films and series with several possible matches, or none. */
export function Identify(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [chosen, setChosen] = useState<WaitingFilter | undefined>(undefined);
  const [lists, reload] = useResource(
    "items/waiting",
    async (): Promise<WaitingLists> => {
      const [ambiguous, unidentified] = await Promise.all(
        (["ambiguous", "unidentified"] as const).map(async (identificationState) => {
          const itemsBody = dataOrThrow(
            await api.GET("/items", { params: { query: { identificationState } } }),
          );
          return itemsBody.items ?? [];
        }),
      );
      return { ambiguous: ambiguous ?? [], unidentified: unidentified ?? [] };
    },
    useBackgroundChangesPoller(),
  );
  return (
    <section aria-labelledby="identify-heading">
      <h1 id="identify-heading">{t("identify.heading")}</h1>
      <Loaded
        resource={lists}
        render={(value) => (
          <WaitingTabs lists={value} chosen={chosen} onChoose={setChosen} onChanged={reload} />
        )}
      />
    </section>
  );
}
