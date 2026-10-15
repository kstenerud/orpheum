// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { BusyGroup } from "./busy-group.tsx";
import { idempotent } from "./idempotent.ts";
import { dataOrThrow, providerProblemText, throwIfRefused } from "./problem.ts";
import { RequestWait } from "./request-wait.tsx";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";

/** One of an ambiguous item's possible matches. */
export type Candidate = components["schemas"]["CandidateBody"];

/** What a search of the provider found for an item: the provider, and its candidates. */
type CandidatesAnswer = components["schemas"]["CandidatesOutputBody"];

/**
 * Why looking up or choosing a match failed, in words a person can act on: the metadata
 * provider's reason (no key, a refused key, not answering) says who can fix it.
 */
export function identifyProblemText(t: TFunction, cause: unknown): string {
  return providerProblemText(t, cause, "identify.providerReason");
}

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

/**
 * An item's possible matches, and choosing one, which is permanent (D172, D301 §2): an ambiguous one's, and a matched one's corrected. Both ask TMDB,
 * so each says what it waits for when TMDB holds it back (D231 §12).
 */
export function CandidateChooser(props: {
  readonly itemId: number;
  readonly onChosen: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [found, setFound] = useState<CandidatesAnswer | undefined>(undefined);
  const requestState = useRequest((cause) => identifyProblemText(t, cause));
  const { runRequest } = requestState;
  useEffect(() => {
    runRequest(async () => {
      setFound(
        dataOrThrow(
          await api.GET("/items/{id}/candidates", { params: { path: { id: props.itemId } } }),
        ),
      );
    });
    // Asked for once, when the chooser opens: runRequest is the same every time.
  }, [api, props.itemId, runRequest]);
  const pick = (provider: string, candidate: Candidate): void => {
    runRequest(async (idempotencyKey) => {
      throwIfRefused(
        await api.POST("/items/{id}/match", {
          params: { path: { id: props.itemId } },
          body: { provider, providerId: candidate.providerId },
          ...idempotent(idempotencyKey),
        }),
      );
      props.onChosen();
    });
  };
  return (
    <section aria-label={t("identify.candidates")}>
      {found === undefined ? null : (
        <BusyGroup isBusy={requestState.isBusy}>
          <CandidateList
            candidates={found.candidates ?? []}
            onPick={(candidate) => {
              pick(found.provider ?? "tmdb", candidate);
            }}
          />
        </BusyGroup>
      )}
      <RequestWait requestState={requestState} provider="TMDB" />
      {requestState.problem === undefined ? null : <p role="alert">{requestState.problem}</p>}
    </section>
  );
}
