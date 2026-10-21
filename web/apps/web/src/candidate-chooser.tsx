// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { ApiProblem } from "@orpheum/sdk";
import { useCallback, useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { BusyGroup } from "./busy-group.tsx";
import { submitWith } from "./form-row.tsx";
import { idempotent } from "./idempotent.ts";
import { dataOrThrow, providerProblemText, throwIfRefused } from "./problem.ts";
import { RequestWait } from "./request-wait.tsx";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";

/** One of an ambiguous item's possible matches. */
export type Candidate = components["schemas"]["CandidateBody"];

/** What a search of the provider found for an item: the provider, and its candidates. */
type CandidatesAnswer = components["schemas"]["CandidatesOutputBody"];

/** A request's answer, as the SDK gives it: a refusal carries the server's error. */
type RefusableAnswer = Parameters<typeof throwIfRefused>[0];

/**
 * Why looking up or choosing a match failed, in words a person can act on: the metadata
 * provider's reason (no key, a refused key, not answering) says who can fix it.
 */
export function identifyProblemText(t: TFunction, cause: unknown): string {
  return providerProblemText(t, cause, "identify.providerReason");
}

/**
 * Why looking up or choosing a match failed, as identifyProblemText says, but a pasted link that
 * names no work of the item's kind is said as such, beside the field it was pasted in.
 */
export function chooserProblemText(t: TFunction, cause: unknown): string {
  const problems =
    cause instanceof ApiProblem ? (cause.withCode("invalid_request")?.detail?.problems ?? []) : [];
  return problems.some((problem) => problem.location === "body.link")
    ? t("identify.linkRefused")
    : identifyProblemText(t, cause);
}

/** A person's own search: another name, and a year when they know it (the owner, 2026-10-21). */
function OwnSearch(props: {
  readonly onSearch: (name: string, year: number) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [year, setYear] = useState("");
  return (
    <form
      className="form-row"
      onSubmit={submitWith(() => {
        props.onSearch(name, year === "" ? 0 : Number(year));
      })}
    >
      <label>
        {t("identify.searchName")}{" "}
        <input
          required
          value={name}
          onChange={(event) => {
            setName(event.target.value);
          }}
        />
      </label>{" "}
      <label>
        {t("identify.searchYear")}{" "}
        <input
          type="number"
          min={1870}
          max={2100}
          value={year}
          onChange={(event) => {
            setYear(event.target.value);
          }}
        />
      </label>{" "}
      <button type="submit">{t("identify.search")}</button>
    </form>
  );
}

/** A pasted address or id of the work at TMDB or IMDb, which chooses it (the owner, 2026-10-21). */
function PastedLink(props: { readonly onPaste: (link: string) => void }): JSX.Element {
  const { t } = useTranslation();
  const [link, setLink] = useState("");
  return (
    <form
      className="form-row"
      onSubmit={submitWith(() => {
        props.onPaste(link);
      })}
    >
      <label>
        {t("identify.link")}{" "}
        <input
          required
          placeholder={t("identify.linkHint")}
          value={link}
          onChange={(event) => {
            setLink(event.target.value);
          }}
        />
      </label>{" "}
      <button type="submit">{t("identify.useLink")}</button>
    </form>
  );
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
 * The chooser's requests, one at a time: what the search found, a person's own search, a pasted
 * link, and a pick of a candidate, each of which but the searches chooses the match.
 */
function useChooserRequests(
  itemId: number,
  onChosen: () => void,
): {
  readonly found: CandidatesAnswer | undefined;
  readonly requestState: ReturnType<typeof useRequest>;
  readonly search: (name: string, year: number) => void;
  readonly paste: (link: string) => void;
  readonly pick: (provider: string, candidate: Candidate) => void;
} {
  const { t } = useTranslation();
  const { api } = useServices();
  const [found, setFound] = useState<CandidatesAnswer | undefined>(undefined);
  const requestState = useRequest((cause) => chooserProblemText(t, cause));
  const { runRequest } = requestState;
  const search = useCallback(
    (name: string, year: number): void => {
      runRequest(async () => {
        const query = name === "" ? {} : { name, year };
        setFound(
          dataOrThrow(
            await api.GET("/items/{id}/candidates", { params: { path: { id: itemId }, query } }),
          ),
        );
      });
    },
    [api, itemId, runRequest],
  );
  useEffect(() => {
    // What the item's own search found, asked for once, when the chooser opens.
    search("", 0);
  }, [search]);
  const choose = (send: (idempotencyKey: string) => Promise<RefusableAnswer>): void => {
    runRequest(async (idempotencyKey) => {
      throwIfRefused(await send(idempotencyKey));
      onChosen();
    });
  };
  const paste = (link: string): void => {
    choose((idempotencyKey) =>
      api.POST("/items/{id}/match-by-link", {
        params: { path: { id: itemId } },
        body: { link },
        ...idempotent(idempotencyKey),
      }),
    );
  };
  const pick = (provider: string, candidate: Candidate): void => {
    choose((idempotencyKey) =>
      api.POST("/items/{id}/match", {
        params: { path: { id: itemId } },
        body: { provider, providerId: candidate.providerId },
        ...idempotent(idempotencyKey),
      }),
    );
  };
  return { found, requestState, search, paste, pick };
}

/**
 * An item's possible matches, and choosing one, which is permanent (D172, D301 §2): an ambiguous
 * one's, and a matched one's corrected; and, when the work is not among them, a person's own search
 * or its page at TMDB or IMDb pasted (the owner, 2026-10-21). Each asks TMDB, so each says what it
 * waits for when TMDB holds it back (D231 §12).
 */
export function CandidateChooser(props: {
  readonly itemId: number;
  readonly onChosen: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { found, requestState, search, paste, pick } = useChooserRequests(
    props.itemId,
    props.onChosen,
  );
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
          {found.candidates?.length === 0 ? <p>{t("identify.noneFound")}</p> : null}
        </BusyGroup>
      )}
      <BusyGroup isBusy={requestState.isBusy}>
        <p>{t("identify.notListed")}</p>
        <OwnSearch onSearch={search} />
        <PastedLink onPaste={paste} />
      </BusyGroup>
      <RequestWait requestState={requestState} provider="TMDB" />
      {requestState.problem === undefined ? null : <p role="alert">{requestState.problem}</p>}
    </section>
  );
}
