// SPDX-License-Identifier: MPL-2.0

import { use, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { attentionLine } from "./attention-lines.ts";
import { AttentionList } from "./attention-list.tsx";
import {
  attentionPageName,
  attentionPageOrder,
  attentionPageRoute,
  itemsOf,
  type AttentionItem,
  type AttentionPageKey,
} from "./attention-pages.ts";
import { WarningIcon } from "./icons.tsx";
import { Link } from "./link.tsx";
import {
  needsAttentionKey,
  readNeedsAttention,
  type NeedsAttentionCounts,
} from "./needs-attention-read.ts";
import { Page } from "./page.tsx";
import { Section } from "./section.tsx";
import { useServices } from "./services.tsx";
import { Loaded } from "./loaded.tsx";
import { StatusNote } from "./status-note.tsx";
import { useResource, type Resource } from "./use-resource.ts";
import {
  readUnknownFailures,
  reportedKinds,
  UnknownFailures,
  type UnknownFailuresReport,
} from "./unknown-failures.tsx";
import { IsAdminContext } from "./warning-sign.tsx";

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

/**
 * The series likely read wrongly (D306 §6) and the films without sound (the owner, 2026-11-02),
 * each by name, leading to its page: the server lists at most 100 of each, and how many more there
 * are is said after them (D432 §4). Titles to check's own lines, after its counts.
 */
function ProblemTitles(props: { readonly value: NeedsAttentionCounts }): JSX.Element {
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

/** One page's section: its name with the warning icon, the way to it, then its lines. */
function PageSection(props: {
  readonly page: AttentionPageKey;
  readonly value: NeedsAttentionCounts;
  readonly unknownFailures: Resource<UnknownFailuresReport | undefined>;
}): JSX.Element {
  const { t } = useTranslation();
  const name = t(`nav.${attentionPageName[props.page]}`);
  return (
    <Section
      heading={name}
      mark="warning"
      actions={
        <Link to={attentionPageRoute[props.page]}>{t("attention.goTo", { page: name })}</Link>
      }
    >
      <AttentionList items={itemsOf(props.value.pages, props.page)}>
        {props.page === "titlesToCheck" ? <ProblemTitles value={props.value} /> : null}
      </AttentionList>
      {props.page === "system" ? <UnknownFailures report={props.unknownFailures} /> : null}
    </Section>
  );
}

/**
 * The pages something needs the admin on, in the menu's order: those with an item of a kind this app
 * knows, and System's while failures of unknown cause are reported too.
 */
function pagesNeeding(
  value: NeedsAttentionCounts,
  unknownFailures: Resource<UnknownFailuresReport | undefined>,
  isKnown: (item: AttentionItem) => boolean,
): readonly AttentionPageKey[] {
  return attentionPageOrder.filter(
    (page) =>
      itemsOf(value.pages, page).some(isKnown) ||
      (page === "system" && reportedKinds(unknownFailures).length > 0),
  );
}

/**
 * Everything that needs the admin, in one place, which the status tray's warning sign leads to
 * (D280), by the page it is fixed on (D538 §12): one section to a page, in the menu's order, its
 * name and the way to it, then one line to each thing, a link to the exact place, the problems
 * first; the failures of unknown cause in full in System's (D511 §6). With nothing, it says so.
 */
export function NeedsAttention(): JSX.Element {
  const { t, i18n } = useTranslation();
  const { api } = useServices();
  // The tray's answer shows at once, read again behind it (D478).
  const [counts] = useResource(needsAttentionKey, async () => await readNeedsAttention(api));
  const unknownFailures = useUnknownFailures();
  return (
    <Page heading={t("needsAttention.heading")} job={t("attention.job")}>
      <Loaded
        resource={counts}
        render={(value) => {
          const pages = pagesNeeding(
            value,
            unknownFailures,
            (item) => attentionLine(t, item, i18n.language) !== undefined,
          );
          return pages.length === 0 ? (
            <StatusNote told={{ kind: "success", text: t("needsAttention.nothing") }} />
          ) : (
            <>
              {pages.map((page) => (
                <PageSection
                  key={page}
                  page={page}
                  value={value}
                  unknownFailures={unknownFailures}
                />
              ))}
            </>
          );
        }}
      />
    </Page>
  );
}
