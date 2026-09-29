// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
import { entryRoute } from "./browse.tsx";
import { Link } from "./link.tsx";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { useResource } from "./use-resource.ts";

/** A film or series waiting for a match, as the server sends it. */
export type WaitingItem = components["schemas"]["ItemBody"];
/** One of an ambiguous item's possible matches. */
export type Candidate = components["schemas"]["CandidateBody"];

/** Which items to show: those with several matches, or those with none. */
export type Waiting = "ambiguous" | "unidentified";

/** The possible matches for one item, each a button that chooses it. */
export function CandidateList(props: {
  readonly candidates: readonly Candidate[];
  readonly onPick: (candidate: Candidate) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <ul>
      {props.candidates.map((candidate) => (
        <li key={candidate.providerId}>
          <span>
            {t("identify.candidate", { name: candidate.name, year: candidate.year ?? "?" })}
          </span>{" "}
          {candidate.originalName === undefined ||
          candidate.originalName === candidate.name ? null : (
            <span>{t("identify.originalName", { name: candidate.originalName })}</span>
          )}{" "}
          <button
            type="button"
            onClick={() => {
              props.onPick(candidate);
            }}
          >
            {t("identify.pick")}
          </button>
        </li>
      ))}
    </ul>
  );
}

/** The items waiting, each with a way to choose its match or have it looked up again. */
export function WaitingList(props: {
  readonly items: readonly WaitingItem[];
  readonly waiting: Waiting;
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
          <WaitingRow item={item} waiting={props.waiting} onChanged={props.onChanged} />
        </li>
      ))}
    </ul>
  );
}

/** One waiting item: its name and year, and what can be done about it. */
function WaitingRow(props: {
  readonly item: WaitingItem;
  readonly waiting: Waiting;
  readonly onChanged: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { item } = props;
  const [isChoosing, setIsChoosing] = useState(false);
  const [note, setNote] = useState("");
  const refresh = (): void => {
    api
      .POST("/items/{id}/refresh", { params: { path: { id: item.id } } })
      .then(throwIfRefused)
      .then(
        () => {
          setNote(t("identify.refreshed"));
        },
        (cause: unknown) => {
          setNote(problemText(t, cause));
        },
      );
  };
  const kind = item.kind === "series" ? "series" : "movie";
  return (
    <div>
      <Link to={entryRoute(kind, item.id)}>{item.name}</Link>{" "}
      {item.year === undefined ? null : <span>{item.year}</span>}{" "}
      {props.waiting === "ambiguous" ? (
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
      <button type="button" onClick={refresh}>
        {t("identify.refresh")}
      </button>
      {note === "" ? null : <output>{note}</output>}
      {isChoosing ? <Candidates id={item.id} onChosen={props.onChanged} /> : null}
    </div>
  );
}

/** An ambiguous item's possible matches, and choosing one, which is permanent (D172). */
function Candidates(props: { readonly id: number; readonly onChosen: () => void }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [problem, setProblem] = useState("");
  const [found] = useResource(`candidates/${String(props.id)}`, async () => {
    const data = dataOrThrow(
      await api.GET("/items/{id}/candidates", {
        params: { path: { id: props.id } },
      }),
    );
    return data;
  });
  const pick = (provider: string, candidate: Candidate): void => {
    api
      .POST("/items/{id}/match", {
        params: { path: { id: props.id } },
        body: { provider, providerId: candidate.providerId },
      })
      .then(throwIfRefused)
      .then(props.onChosen, (cause: unknown) => {
        setProblem(problemText(t, cause));
      });
  };
  return (
    <section aria-label={t("identify.candidates")}>
      <Loaded
        resource={found}
        render={(value) => (
          <CandidateList
            candidates={value.candidates ?? []}
            onPick={(candidate) => {
              pick(value.provider ?? "tmdb", candidate);
            }}
          />
        )}
      />
      {problem === "" ? null : <p role="alert">{problem}</p>}
    </section>
  );
}

/** Needs a match: the films and series with several possible matches, or none. */
export function Identify(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [waiting, setWaiting] = useState<Waiting>("ambiguous");
  const [items, reload] = useResource(`items/${waiting}`, async () => {
    const data = dataOrThrow(
      await api.GET("/items", {
        params: { query: { identificationState: waiting } },
      }),
    );
    return data.items ?? [];
  });
  return (
    <section aria-labelledby="identify-heading">
      <h1 id="identify-heading">{t("identify.heading")}</h1>
      <fieldset>
        <legend>{t("files.show")}</legend>
        {(["ambiguous", "unidentified"] as const).map((choice) => (
          <button
            key={choice}
            type="button"
            aria-pressed={waiting === choice}
            onClick={() => {
              setWaiting(choice);
            }}
          >
            {t(`identify.${choice}`)}
          </button>
        ))}
      </fieldset>
      <Loaded
        resource={items}
        render={(value) => <WaitingList items={value} waiting={waiting} onChanged={reload} />}
      />
    </section>
  );
}
