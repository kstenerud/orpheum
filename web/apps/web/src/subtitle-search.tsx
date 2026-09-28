// SPDX-License-Identifier: MPL-2.0

import type { Api, components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { camelKey } from "./catalogue-key.ts";
import { ApiProblem, dataOrThrow, problemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { languageName } from "./subtitles.tsx";

/** A subtitle OpenSubtitles can give, unverified, as the server lists it (D224). */
export type SubtitleCandidate = components["schemas"]["SubtitleCandidateBody"];
/** What the server lists when asked for subtitles: the candidates and the downloads left. */
export type SubtitleOffer = components["schemas"]["SubtitleCandidatesOutputBody"];

/** The languages a person can ask for, as OpenSubtitles names them (BCP 47). */
export const searchLanguages: readonly string[] = [
  "ar",
  "cs",
  "da",
  "de",
  "el",
  "en",
  "es",
  "fi",
  "fr",
  "he",
  "hu",
  "it",
  "ja",
  "ko",
  "nl",
  "no",
  "pl",
  "pt",
  "pt-BR",
  "ro",
  "ru",
  "sv",
  "tr",
  "uk",
  "zh-CN",
  "zh-TW",
];

/** The language a search starts with: the viewer's own when it can be asked for, else English. */
export function startingLanguage(viewerLanguage: string): string {
  const exact = searchLanguages.find((tag) => tag.toLowerCase() === viewerLanguage.toLowerCase());
  const primary = viewerLanguage.split("-")[0]?.toLowerCase() ?? "";
  return exact ?? searchLanguages.find((tag) => tag === primary) ?? "en";
}

/** Lists the subtitles that can be fetched for a file in a language. */
export async function listCandidates(
  api: Api,
  fileId: number,
  lang: string,
): Promise<SubtitleOffer> {
  return dataOrThrow(
    await api.GET("/files/{id}/subtitle-candidates", {
      params: { path: { id: fileId }, query: { lang } },
    }),
  );
}

/** Fetches a candidate, which spends one of the account's downloads and becomes the file's choice. */
export async function fetchCandidate(
  api: Api,
  fileId: number,
  chosen: { readonly providerFileId: number; readonly language: string },
): Promise<void> {
  dataOrThrow(
    await api.POST("/files/{id}/subtitles", {
      params: { path: { id: fileId } },
      body: { providerFileId: chosen.providerFileId, language: chosen.language },
    }),
  );
}

/** Why a search or fetch failed, in words a person can act on (D224 §4). */
export function fetchProblem(
  t: ReturnType<typeof useTranslation>["t"],
  cause: unknown,
  viewerLanguage: string,
): string {
  if (cause instanceof ApiProblem) {
    if (cause.code === "quota_exhausted") {
      const resetAt = cause.detail["resetAt"];
      const when =
        resetAt === undefined
          ? ""
          : new Intl.DateTimeFormat(viewerLanguage, { timeStyle: "short" }).format(
              new Date(resetAt),
            );
      return t("play.findSubtitles.quotaSpent", { when });
    }
    const reason = cause.detail["reason"];
    if (cause.code === "provider_error" && reason !== undefined) {
      return t(`play.findSubtitles.reason.${camelKey(reason)}`, {
        defaultValue: t("play.findSubtitles.reason.unknown"),
      });
    }
    if (cause.code === "conflict") {
      return t("play.findSubtitles.nothingToSearchBy");
    }
  }
  return problemText(t, cause);
}

/** One request at a time: whether it is running, and why it failed when it did (D224 §4). */
function useRequest(): {
  isBusy: boolean;
  problem: string | undefined;
  run: (work: Promise<void>) => void;
} {
  const { t, i18n } = useTranslation();
  const [isBusy, setIsBusy] = useState(false);
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const run = (work: Promise<void>): void => {
    setIsBusy(true);
    setProblem(undefined);
    work.then(
      () => {
        setIsBusy(false);
      },
      (cause: unknown) => {
        setIsBusy(false);
        setProblem(fetchProblem(t, cause, i18n.language));
      },
    );
  };
  return { isBusy, problem, run };
}

/**
 * Finds a subtitle a file lacks (D224): the person picks a language, sees what OpenSubtitles has,
 * those made for this very file first, each labelled unverified, with the downloads left today,
 * and fetches one. Nothing is fetched until they choose.
 */
export function FindSubtitles(props: {
  readonly fileId: number;
  readonly fetched: () => void;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  const { api } = useServices();
  const [isOpen, setIsOpen] = useState(false);
  const [language, setLanguage] = useState(() => startingLanguage(i18n.language));
  const [offer, setOffer] = useState<SubtitleOffer | undefined>(undefined);
  const { isBusy, problem, run } = useRequest();
  if (!isOpen) {
    return (
      <button
        type="button"
        onClick={() => {
          setIsOpen(true);
        }}
      >
        {t("play.findSubtitles.open")}
      </button>
    );
  }
  const search = (): void => {
    run(listCandidates(api, props.fileId, language).then(setOffer));
  };
  const choose = (candidate: SubtitleCandidate): void => {
    const chosen = { providerFileId: candidate.providerFileId, language };
    run(
      fetchCandidate(api, props.fileId, chosen).then(() => {
        setIsOpen(false);
        props.fetched();
      }),
    );
  };
  return (
    <section className="find-subtitles" aria-labelledby="find-subtitles-heading">
      <h2 id="find-subtitles-heading">{t("play.findSubtitles.heading")}</h2>
      <LanguagePicker
        language={language}
        picked={(picked) => {
          setLanguage(picked);
          setOffer(undefined);
        }}
      />{" "}
      <button type="button" onClick={search} disabled={isBusy}>
        {t("play.findSubtitles.search")}
      </button>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
      {offer === undefined ? null : (
        <CandidateList offer={offer} language={language} isBusy={isBusy} choose={choose} />
      )}
    </section>
  );
}

/** The language a search asks for, named in the viewer's language. */
function LanguagePicker(props: {
  readonly language: string;
  readonly picked: (language: string) => void;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  return (
    <label>
      {t("play.findSubtitles.language")}{" "}
      <select
        value={props.language}
        onChange={(event) => {
          props.picked(event.currentTarget.value);
        }}
      >
        {searchLanguages.map((tag) => (
          <option key={tag} value={tag}>
            {languageName(tag, i18n.language)}
          </option>
        ))}
      </select>
    </label>
  );
}

/**
 * The candidates, each unverified, with what each would cost; or, when there are none, what was
 * searched by, since a file not identified yet is searched for by its hash alone.
 */
function CandidateList(props: {
  readonly offer: SubtitleOffer;
  readonly language: string;
  readonly isBusy: boolean;
  readonly choose: (candidate: SubtitleCandidate) => void;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  const candidates = props.offer.candidates ?? [];
  const { allowance, didAskByIds } = props.offer;
  const language = languageName(props.language, i18n.language);
  const none = didAskByIds ? "play.findSubtitles.none" : "play.findSubtitles.noneForThisFile";
  return (
    <>
      <p>
        {t("play.findSubtitles.allowance", {
          remaining: allowance.remaining,
          allowed: allowance.allowed,
        })}
      </p>
      {candidates.length === 0 ? <p>{t(none, { language })}</p> : null}
      {didAskByIds ? null : <p>{t("play.findSubtitles.fileHashOnly")}</p>}
      <ul className="find-subtitles__candidates">
        {candidates.map((candidate) => (
          <li key={candidate.providerFileId}>
            <span className="find-subtitles__release">{candidate.release}</span>{" "}
            {candidate.isHashMatch ? (
              <strong>{t("play.findSubtitles.madeForThisFile")}</strong>
            ) : null}{" "}
            {candidate.isHearingImpaired ? t("play.subtitles.hearingImpaired") : null}{" "}
            {candidate.isMachineTranslated ? t("play.findSubtitles.machineTranslated") : null}{" "}
            {t("play.findSubtitles.downloads", { count: candidate.downloads })}{" "}
            <button
              type="button"
              disabled={props.isBusy || allowance.remaining <= 0}
              onClick={() => {
                props.choose(candidate);
              }}
            >
              {t("play.findSubtitles.fetch")}
            </button>
          </li>
        ))}
      </ul>
      {candidates.length === 0 ? null : <p>{t("play.findSubtitles.unverified")}</p>}
    </>
  );
}
