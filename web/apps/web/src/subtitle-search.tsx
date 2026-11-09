// SPDX-License-Identifier: MPL-2.0

import type { Api, components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { idempotent } from "./idempotent.ts";
import { timeText } from "./date-time-text.ts";
import { ApiProblem, dataOrThrow, providerProblemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { RequestWait } from "./request-wait.tsx";
import { ActionChooser, currentLabelOf, type ActionChoice } from "./route-chooser.tsx";
import { useRequest } from "./use-request.ts";
import { languageName } from "./language-name.ts";

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
  language: string,
): Promise<SubtitleOffer> {
  return dataOrThrow(
    await api.GET("/files/{id}/subtitle-candidates", {
      params: { path: { id: fileId }, query: { language } },
    }),
  );
}

/** Fetches a candidate, which spends one of the account's downloads and becomes the file's choice. */
export async function fetchCandidate(
  api: Api,
  chosen: { readonly fileId: number; readonly providerFileId: number; readonly language: string },
  idempotencyKey: string,
): Promise<void> {
  const { fileId } = chosen;
  dataOrThrow(
    await api.POST("/files/{id}/subtitles", {
      params: { path: { id: fileId } },
      body: { providerFileId: chosen.providerFileId, language: chosen.language },
      ...idempotent(idempotencyKey),
    }),
  );
}

/** Why a search or fetch failed, in words a person can act on (D224 §4). */
export function subtitleProblemText(
  t: ReturnType<typeof useTranslation>["t"],
  cause: unknown,
  viewerLanguage: string,
): string {
  if (cause instanceof ApiProblem) {
    const quotaExhausted = cause.withCode("quotaExhausted");
    if (quotaExhausted !== undefined) {
      const downloadsResetAt = quotaExhausted.detail?.downloadsResetAt;
      const resetTime =
        downloadsResetAt === undefined ? "" : timeText(downloadsResetAt, viewerLanguage);
      return t("play.findSubtitles.quotaSpent", { when: resetTime });
    }
    if (cause.code === "conflict") {
      return t("play.findSubtitles.nothingToSearchBy");
    }
  }
  return providerProblemText(t, cause, "play.findSubtitles.reason");
}

/**
 * Finds a subtitle a file lacks (D224): the person picks a language, sees what OpenSubtitles has,
 * those made for this very file first, each labelled unverified, with the downloads left today,
 * and fetches one. Nothing is fetched until they choose.
 */
export function FindSubtitles(props: {
  readonly fileId: number;
  readonly onFetched: () => void;
  /** Whether it opens at once, on its language and search: the player's 💬 asked for it (D407 §8). */
  readonly isOpenAtOnce?: boolean;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  const { api } = useServices();
  const [isOpen, setIsOpen] = useState(props.isOpenAtOnce === true);
  const [language, setLanguage] = useState(() => startingLanguage(i18n.language));
  const [offer, setOffer] = useState<SubtitleOffer | undefined>(undefined);
  // One request at a time, and why it failed when it did (D224 §4).
  const requestState = useRequest((cause) => subtitleProblemText(t, cause, i18n.language));
  const { isBusy, problem, runRequest } = requestState;
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
    runRequest(async () => {
      setOffer(await listCandidates(api, props.fileId, language));
    });
  };
  const choose = (candidate: SubtitleCandidate): void => {
    const chosen = { providerFileId: candidate.providerFileId, language };
    runRequest(async (idempotencyKey) => {
      await fetchCandidate(api, { fileId: props.fileId, ...chosen }, idempotencyKey);
      setIsOpen(false);
      props.onFetched();
    });
  };
  return (
    <section className="subtitle-search" aria-labelledby="subtitle-search-heading">
      <h2 id="subtitle-search-heading">{t("play.findSubtitles.heading")}</h2>
      <LanguagePicker
        language={language}
        onPicked={(picked) => {
          setLanguage(picked);
          setOffer(undefined);
        }}
      />{" "}
      <button type="button" onClick={search} disabled={isBusy}>
        {t("play.findSubtitles.search")}
      </button>
      <RequestWait requestState={requestState} provider="OpenSubtitles" />
      {problem === undefined ? null : <p role="alert">{problem}</p>}
      {offer === undefined ? null : (
        <CandidateList offer={offer} language={language} isBusy={isBusy} onChoose={choose} />
      )}
    </section>
  );
}

/**
 * The language a search asks for, named in the viewer's language: the app's chooser
 * (route-chooser.tsx), which a remote works, where a native select was, which a television's
 * browser does not (the 2026-10-27 widget audit; plans/toolkit.md step 6).
 */
function LanguagePicker(props: {
  readonly language: string;
  readonly onPicked: (language: string) => void;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  const { onPicked } = props;
  const options = searchLanguages.map((tag): ActionChoice => ({
    key: tag,
    label: languageName(tag, i18n.language),
    isCurrent: tag === props.language,
    onChoose: () => {
      onPicked(tag);
    },
  }));
  return (
    <ActionChooser
      fieldLabel={t("play.findSubtitles.language")}
      current={currentLabelOf(options)}
      options={options}
    />
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
  readonly onChoose: (candidate: SubtitleCandidate) => void;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  const candidates = props.offer.candidates ?? [];
  const { allowance, didAskByIds } = props.offer;
  const languageLabel = languageName(props.language, i18n.language);
  const noCandidatesKey = didAskByIds
    ? "play.findSubtitles.none"
    : "play.findSubtitles.noneForThisFile";
  return (
    <>
      <p>
        {t("play.findSubtitles.allowance", {
          remaining: allowance.remainingDownloads,
          allowed: allowance.allowedDownloads,
        })}
      </p>
      {candidates.length === 0 ? <p>{t(noCandidatesKey, { language: languageLabel })}</p> : null}
      {didAskByIds ? null : <p>{t("play.findSubtitles.fileHashOnly")}</p>}
      <ul className="subtitle-search__candidates">
        {candidates.map((candidate) => (
          <li key={candidate.providerFileId}>
            <span className="subtitle-search__release">{candidate.releaseName}</span>{" "}
            {candidate.isOpensubtitlesHashMatch ? (
              <strong>{t("play.findSubtitles.madeForThisFile")}</strong>
            ) : null}{" "}
            {candidate.isHearingImpaired ? t("play.subtitles.hearingImpaired") : null}{" "}
            {candidate.isMachineTranslated ? t("play.findSubtitles.machineTranslated") : null}{" "}
            {t("play.findSubtitles.downloads", { count: candidate.downloadCount })}{" "}
            <button
              type="button"
              disabled={props.isBusy || allowance.remainingDownloads <= 0}
              onClick={() => {
                props.onChoose(candidate);
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
