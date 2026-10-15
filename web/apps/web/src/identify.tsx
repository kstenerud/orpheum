// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { idempotent } from "./idempotent.ts";
import { dateTimeText } from "./date-time-text.ts";
import { i18n } from "./i18n.ts";
import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow } from "./problem.ts";
import { Link } from "./link.tsx";
import { itemRoute } from "./router.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { CandidateChooser, identifyProblemText } from "./candidate-chooser.tsx";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";

/** A film or series waiting for a match, as the server sends it. */
export type WaitingItem = components["schemas"]["ItemBody"];
/** Which items to show: those with several matches, or those with none. */
export type WaitingFilter = "ambiguous" | "unidentified";

/** The items waiting, each with a way to choose its match or have it looked up again. */
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
      {props.items.map((item) => (
        <li key={item.id}>
          <WaitingRow item={item} waitingFilter={props.waitingFilter} onChanged={props.onChanged} />
        </li>
      ))}
    </ul>
  );
}

/** One waiting item: its name and year, and what can be done about it. */
function WaitingRow(props: {
  readonly item: WaitingItem;
  readonly waitingFilter: WaitingFilter;
  readonly onChanged: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { item } = props;
  const [isChoosing, setIsChoosing] = useState(false);
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
  return (
    <div>
      <Link to={itemRoute(item.kind, item.id)}>{item.name}</Link>{" "}
      {item.year === undefined ? null : <span>{item.year}</span>}{" "}
      {props.waitingFilter === "ambiguous" ? (
        <button
          type="button"
          aria-expanded={isChoosing}
          onClick={() => {
            setIsChoosing(!isChoosing);
          }}
        >
          {t("identify.choose")}
        </button>
      ) : null}{" "}
      <button type="button" disabled={refreshing.isBusy} onClick={refresh}>
        {t("identify.refresh")}
      </button>
      {note === "" ? null : <output>{note}</output>}
      {refreshing.problem === undefined ? null : <output>{refreshing.problem}</output>}
      {isChoosing ? <CandidateChooser itemId={item.id} onChosen={props.onChanged} /> : null}
    </div>
  );
}

/** Needs a match: the films and series with several possible matches, or none. */
export function Identify(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [waitingFilter, setWaitingFilter] = useState<WaitingFilter>("ambiguous");
  const [items, reload] = useResource(
    `items/${waitingFilter}`,
    async () => {
      const itemsBody = dataOrThrow(
        await api.GET("/items", {
          params: { query: { identificationState: waitingFilter } },
        }),
      );
      return itemsBody.items ?? [];
    },
    useBackgroundChangesPoller(),
  );
  return (
    <section aria-labelledby="identify-heading">
      <h1 id="identify-heading">{t("identify.heading")}</h1>
      <fieldset>
        <legend>{t("common.show")}</legend>
        {(["ambiguous", "unidentified"] as const).map((choice) => (
          <button
            key={choice}
            type="button"
            aria-pressed={waitingFilter === choice}
            onClick={() => {
              setWaitingFilter(choice);
            }}
          >
            {t(`identify.${choice}`)}
          </button>
        ))}
      </fieldset>
      <Loaded
        resource={items}
        render={(value) => (
          <WaitingList items={value} waitingFilter={waitingFilter} onChanged={reload} />
        )}
      />
    </section>
  );
}
