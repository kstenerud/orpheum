// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { orderingAired } from "./episode-place.ts";
import { camelKey } from "./catalogue-key.ts";
import { ModalDialog } from "./modal-dialog.tsx";
import { problemText, throwIfRefused } from "./problem.ts";
import { ActionChooser, currentLabelOf, type ActionChoice } from "./route-chooser.tsx";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";

/**
 * A series' orderings on its page (episode-orderings.md §4): the choice of the one a person watches it
 * in, an admin's of the one its files are numbered in (D210 §1, b2.md Phase B), each in a dialog the
 * page's "…" opens, and how an ordering is named in both; kept apart from series.tsx so it fits in one
 * read.
 */

type SeriesDetail = components["schemas"]["SeriesBody"];
type Ordering = components["schemas"]["OrderingBody"];

/** The dialog the "…"'s Order opens: the choice of ordering, closed once one is chosen. */
export function OrderingDialog(props: {
  readonly series: SeriesDetail;
  readonly onClose: () => void;
  readonly onChosen: (() => void) | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const heading = t("ordering.label");
  return (
    <ModalDialog
      label={heading}
      heading={heading}
      onClose={props.onClose}
      footer={<CancelChoice onClose={props.onClose} />}
    >
      <OrderingChoice
        series={props.series}
        onChosen={() => {
          props.onClose();
          props.onChosen?.();
        }}
      />
    </ModalDialog>
  );
}

/**
 * The dialog the "…"'s "Files are numbered in" opens, an admin's alone: the choice of the ordering
 * the series' files' numbers are in, closed once one is chosen, the page then read again.
 */
export function FilesOrderingDialog(props: {
  readonly series: SeriesDetail;
  readonly onClose: () => void;
  readonly onChosen: (() => void) | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const heading = t("ordering.filesLabel");
  return (
    <ModalDialog
      label={heading}
      heading={heading}
      onClose={props.onClose}
      footer={<CancelChoice onClose={props.onClose} />}
    >
      <FilesOrderingChoice
        series={props.series}
        onChosen={() => {
          props.onClose();
          props.onChosen?.();
        }}
      />
    </ModalDialog>
  );
}

/** The foot of a dialog whose choice acts as it is made: Cancel, to leave it as it is. */
function CancelChoice(props: { readonly onClose: () => void }): JSX.Element {
  const { t } = useTranslation();
  return (
    <button type="button" onClick={props.onClose}>
      {t("dialog.cancel")}
    </button>
  );
}

/**
 * That the ordering chosen is being fetched, on the page, until it is (episode-orderings.md §4):
 * its choice is in a dialog gone once chosen.
 */
export function OrderingFetching(props: { readonly series: SeriesDetail }): JSX.Element | null {
  const { t } = useTranslation();
  const { series } = props;
  const chosen = (series.orderings ?? []).find((ordering) => ordering.name === series.ordering);
  return chosen?.isAwaitingFetch === true ? (
    <p className="series__absent">{t("ordering.fetching")}</p>
  ) : null;
}

/**
 * The orderings a series has, to choose the one the person watches it in (D159 §1): offered only when
 * it has more than one, aired first, each by its label with its kind and how many of the series'
 * episodes it holds (or its provider lists, before it holds any). A choice keeps it and shows the
 * series at its default season, a season of the order left meaning nothing (episode-orderings.md §7
 * Q10); one chosen before its contents were fetched is said to be fetched on the page
 * (OrderingFetching, §4).
 */
export function OrderingChoice(props: {
  readonly series: SeriesDetail;
  readonly onChosen?: (() => void) | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { api, navigation } = useServices();
  const { series } = props;
  return (
    <OrderingChooser
      orderings={series.orderings ?? []}
      current={series.ordering ?? orderingAired}
      fieldLabel={t("ordering.label")}
      failedKey="ordering.failed"
      onChoose={async (chosen) => {
        throwIfRefused(
          await api.PUT("/items/{id}/preferences", {
            params: { path: { id: series.id } },
            body: { episodeOrdering: chosen },
          }),
        );
        // Another ordering is a view of the same series: no step in the history (D367 §9).
        navigation.replace({ screen: "series-page", collectionId: series.id });
        props.onChosen?.();
      }}
    />
  );
}

/**
 * The orderings a series has, to choose the one its files are numbered in, an admin's choice for
 * the household (D210 §1, b2.md Phase B): aired first, the default, each named as the viewing order's
 * choice names it. A choice is kept and the series' match queued, so the library is filed again by
 * it; the page is then read again.
 */
export function FilesOrderingChoice(props: {
  readonly series: SeriesDetail;
  readonly onChosen?: (() => void) | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { api } = useServices();
  const { series } = props;
  return (
    <OrderingChooser
      orderings={series.orderings ?? []}
      current={series.filesOrdering ?? orderingAired}
      fieldLabel={t("ordering.filesLabel")}
      failedKey="ordering.filesFailed"
      onChoose={async (chosen) => {
        throwIfRefused(
          await api.PUT("/items/{id}/files-ordering", {
            params: { path: { id: series.id } },
            body: { filesOrdering: chosen },
          }),
        );
        props.onChosen?.();
      }}
    />
  );
}

/**
 * A choice among a series' orderings in the chooser a remote works, not a native select, which a
 * television's browser cannot (plans/toolkit.md step 6): offered only when there are more than one,
 * the current one marked, and why a choice failed said beside it.
 */
function OrderingChooser(props: {
  readonly orderings: readonly Ordering[];
  /** The name of the ordering chosen now. */
  readonly current: string;
  readonly fieldLabel: string;
  /** The catalogue's key of the words a failed choice says, given its why. */
  readonly failedKey: "ordering.failed" | "ordering.filesFailed";
  readonly onChoose: (chosen: string) => Promise<void>;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  const { orderings, onChoose } = props;
  if (orderings.length < 2) {
    return null;
  }
  const choose = (chosen: string): void => {
    if (isBusy) {
      return;
    }
    runRequest(async () => {
      await onChoose(chosen);
    });
  };
  const options = orderings.map((ordering): ActionChoice => ({
    key: ordering.name,
    label: orderingName(t, ordering),
    isCurrent: ordering.name === props.current,
    onChoose: () => {
      choose(ordering.name);
    },
  }));
  return (
    <div className="series__ordering">
      <ActionChooser
        fieldLabel={props.fieldLabel}
        current={currentLabelOf(options)}
        options={options}
      />
      {problem === undefined ? null : (
        <span className="series__absent">{t(props.failedKey, { why: problem })}</span>
      )}
    </div>
  );
}

/**
 * An ordering as the choices name it: "Aired", or "DVD Order (DVD) · episodes here: 23", followed by
 * "(no longer listed)" where its provider no longer lists it (episode-orderings.md §6 step 5).
 */
export function orderingName(t: TFunction, ordering: Ordering): string {
  const name = orderingOwnName(t, ordering);
  return ordering.isUnlisted === true ? t("ordering.unlisted", { name }) : name;
}

/** An ordering's name as orderingName says it, short of whether its provider still lists it. */
function orderingOwnName(t: TFunction, ordering: Ordering): string {
  if (ordering.name === orderingAired) {
    return t("ordering.aired");
  }
  const kind =
    ordering.kind === undefined
      ? t("orderingKind.authored")
      : t(`orderingKind.${camelKey(ordering.kind)}`, { defaultValue: ordering.kind });
  const label = ordering.label ?? ordering.name;
  if (ordering.episodeCount === 0 && ordering.listedCount !== undefined) {
    return t("ordering.optionListed", { label, kind, count: ordering.listedCount });
  }
  return t("ordering.option", { label, kind, count: ordering.episodeCount });
}
