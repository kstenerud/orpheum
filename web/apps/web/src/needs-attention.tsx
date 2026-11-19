// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { use, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { dateTimeText } from "./date-time-text.ts";
import { arrivalMark } from "./focus-navigation.ts";
import { i18n } from "./i18n.ts";
import { hasCopyWarning } from "./copy-warning.ts";
import { WarningIcon } from "./icons.tsx";
import { Link } from "./link.tsx";
import { needsAttentionKey, readNeedsAttention } from "./needs-attention-read.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./loaded.tsx";
import { useResource, type Resource } from "./use-resource.ts";
import {
  readUnknownFailures,
  reportedKinds,
  UnknownFailures,
  type UnknownFailuresReport,
} from "./unknown-failures.tsx";
import { useUpdate, type Update } from "./update.tsx";
import { IsAdminContext } from "./warning-sign.tsx";

/**
 * A newer version, once the server's word of one is read, leading to How to update, which no menu
 * reaches (ui-audit-2026-11-04.md §3, How to update); with the warning sign where it carries a
 * security fix. Nothing while there is none.
 */
function UpdateItem(props: { readonly newer: Update | undefined }): JSX.Element | null {
  const { t } = useTranslation();
  const { newer } = props;
  if (newer?.latest === undefined) {
    return null;
  }
  return (
    <li>
      {newer.security === undefined ? null : <WarningIcon />}{" "}
      {t("update.available", { latest: newer.latest })}{" "}
      <Link to={{ screen: "update" }}>{t("update.howTo")}</Link>
    </li>
  );
}

/** The server's word of a newer version, once read; undefined while it knows of none. */
function useNewerVersion(): Update | undefined {
  const update = useUpdate(true);
  return update.state === "loaded" && update.value?.latest !== undefined ? update.value : undefined;
}

/**
 * Every failure of unknown cause, for an admin; nothing is asked for anyone else (D511 §7), who
 * reads undefined.
 */
function useUnknownFailures(): Resource<UnknownFailuresReport | undefined> {
  const { api } = useServices();
  const isAdmin = use(IsAdminContext);
  const [report] = useResource(
    isAdmin ? "unknown-failures" : "unknown-failures-not-admin",
    async () => (isAdmin ? await readUnknownFailures(api) : undefined),
  );
  return report;
}

/** What needs the admin, as the server counts and lists it. */
type AttentionCounts = components["schemas"]["NeedsAttentionBody"];

/** The pages that have each kind of what needs the admin, with what fixes it. */
function ToPages(): JSX.Element {
  const { t } = useTranslation();
  return (
    <p>
      <Link to={{ screen: "system" }}>{t("needsAttention.toSystem")}</Link> ·{" "}
      <Link to={{ screen: "libraries" }}>{t("needsAttention.toLibraries")}</Link> ·{" "}
      <Link to={{ screen: "identify" }}>{t("needsAttention.toIdentify")}</Link>
    </p>
  );
}

/**
 * The series likely read wrongly (D306 §6), the films without sound (the owner, 2026-11-02) and
 * the libraries whose rescan gave up (the owner, 2026-11-13), each leading to its page; the server lists at most 100 of each, and how many more there are is
 * said after them (D432 §4).
 */
function ProblemTitles(props: { readonly value: AttentionCounts }): JSX.Element {
  const { t } = useTranslation();
  const { value } = props;
  const readWrongly = value.seriesReadWrongly ?? [];
  const withoutSound = value.filmsWithoutSound ?? [];
  return (
    <>
      {readWrongly.map((series) => (
        <li key={`series-${String(series.seriesId)}`}>
          <WarningIcon />{" "}
          <Link to={{ screen: "series-page", collectionId: series.seriesId }}>{series.name}</Link>
          {t("needsAttention.seriesReadWrongly", { count: series.unplacedCount })}
        </li>
      ))}
      {value.seriesReadWronglyCount > readWrongly.length ? (
        <li key="series-more">
          <WarningIcon />{" "}
          {t("needsAttention.moreSeriesReadWrongly", {
            count: value.seriesReadWronglyCount - readWrongly.length,
          })}
        </li>
      ) : null}
      {withoutSound.map((film) => (
        <li key={`film-${String(film.itemId)}`}>
          <WarningIcon /> <Link to={{ screen: "title", itemId: film.itemId }}>{film.name}</Link>
          {t("needsAttention.filmWithoutSound")}
        </li>
      ))}
      {(value.rescansGivenUp ?? []).map((library) => (
        <li key={`rescan-${String(library.libraryId)}`}>
          <WarningIcon />{" "}
          <Link to={{ screen: "library", libraryId: library.libraryId }}>{library.name}</Link>
          {t("needsAttention.rescanGaveUp")}
        </li>
      ))}
      {value.filmsWithoutSoundCount > withoutSound.length ? (
        <li key="films-more">
          <WarningIcon />{" "}
          {t("needsAttention.moreFilmsWithoutSound", {
            count: value.filmsWithoutSoundCount - withoutSound.length,
          })}
        </li>
      ) : null}
    </>
  );
}

/**
 * One copy of the server serves (D437 §4): this serving copy says a copy tried to start, from where and
 * when, and was refused; a refused copy says another serves, leading to the System page, where the
 * admin has it ask again. A server older than these fields sends none: missing is neither.
 */
function OneCopyItems(props: { readonly value: AttentionCounts }): JSX.Element {
  const { t } = useTranslation();
  const { value } = props;
  return (
    <>
      {value.copyRefusedAt > 0 ? (
        <li key="copy-refused">
          <WarningIcon />{" "}
          {t("needsAttention.copyRefused", {
            from: value.copyRefusedFrom,
            at: dateTimeText(value.copyRefusedAt, i18n.language),
          })}
        </li>
      ) : null}
      {value.stoodAsideAt > 0 ? (
        <li key="stood-aside">
          <WarningIcon /> {t("needsAttention.stoodAside")}{" "}
          <Link to={{ screen: "system" }}>{t("needsAttention.askAgainOnSystem")}</Link>
        </li>
      ) : null}
    </>
  );
}

/**
 * Everything that needs the admin, in one place, which the status tray's warning sign leads to
 * (D280). A placeholder until it is designed: it counts what needs them and leads to the pages that
 * have each, the System page, the Libraries and Needs a match, and a newer version to How to update;
 * below, the failures of unknown cause in full (D511 §6).
 */
export function NeedsAttention(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  // The tray's answer shows at once, read again behind it (D478).
  const [counts] = useResource(needsAttentionKey, async () => await readNeedsAttention(api));
  const newer = useNewerVersion();
  const unknownFailures = useUnknownFailures();
  return (
    <section aria-labelledby="needs-attention-heading">
      <h1 id="needs-attention-heading">{t("needsAttention.heading")}</h1>
      <p>{t("needsAttention.placeholder")}</p>
      <Loaded
        resource={counts}
        render={(value) => {
          // Each count's words, and their own for one (the catalogue has no plural forms: en.ts); what
          // could not be read leads to the System page, which names each (the first beta install,
          // 2026-11-19, #32: "1 files or folders could not be read", naming nothing).
          const lines = [
            {
              count: value.rootCount,
              key: "needsAttention.offlineFolders",
              one: "needsAttention.offlineFoldersOne",
            },
            {
              count: value.diskCount,
              key: "needsAttention.missingDisks",
              one: "needsAttention.missingDisksOne",
            },
            {
              count: value.waitingRootCount,
              key: "needsAttention.waitingFolders",
              one: "needsAttention.waitingFoldersOne",
            },
            {
              count: value.unreadCount,
              key: "needsAttention.unreadPaths",
              one: "needsAttention.unreadPathsOne",
              isOnSystem: true,
            },
            {
              count: value.matchCount,
              key: "needsAttention.waitingMatches",
              one: "needsAttention.waitingMatchesOne",
            },
          ].filter((line) => line.count > 0);
          return lines.length +
            value.seriesReadWronglyCount +
            value.filmsWithoutSoundCount +
            (value.rescansGivenUp?.length ?? 0) ===
            0 &&
            newer === undefined &&
            !hasCopyWarning(value) &&
            reportedKinds(unknownFailures).length === 0 ? (
            <p>{t("needsAttention.nothing")}</p>
          ) : (
            <ul {...arrivalMark}>
              <UpdateItem newer={newer} />
              <OneCopyItems value={value} />
              {lines.map((line) => (
                <li key={line.key}>
                  <WarningIcon />{" "}
                  {line.count === 1 ? t(line.one) : t(line.key, { count: line.count })}
                  {line.isOnSystem === true ? (
                    <>
                      {" "}
                      <Link to={{ screen: "system" }}>{t("needsAttention.toSystem")}</Link>
                    </>
                  ) : null}
                </li>
              ))}
              <ProblemTitles value={value} />
            </ul>
          );
        }}
      />
      <UnknownFailures report={unknownFailures} />
      <ToPages />
    </section>
  );
}
