// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { use, type JSX } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

import { orderingAired } from "./episode-place.ts";
import { camelKey } from "./catalogue-key.ts";
import type { MoreAction, MoreSubmenu } from "./more-actions.tsx";
import { problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { StatusNote, type SaveTold } from "./status-note.tsx";
import { useRequest } from "./use-request.ts";
import { IsAdminContext } from "./warning-sign.tsx";

/**
 * A series' orderings on its page (episode-orderings.md §4): the choice of the one a person watches it
 * in, an admin's of the one its files are numbered in (D210 §1, b2.md Phase B), each a list in the
 * page's "…"'s place at its opener (D538 §3), and how an ordering is named in both; kept apart from
 * series.tsx so it fits in one read.
 */

type SeriesDetail = components["schemas"]["SeriesBody"];
type Ordering = components["schemas"]["OrderingBody"];

/** A series' choices of ordering as its "…" offers them, and how the last one chosen failed. */
export interface OrderingActions {
  readonly actions: readonly MoreAction[];
  /** Why the last choice was not kept, said on the page, the "…" closed once chosen; else none. */
  readonly told: SaveTold;
}

/**
 * The orderings a series has, as its "…"'s lists (D538 §3): Order, the one the person watches it in
 * (D159 §1), and, an admin's alone, "Files are numbered in", the one its files' numbers are in, for the
 * household (D210 §1); each offered only where it has more than one, aired first, the current one
 * marked. A viewing order chosen is kept and the series shown at its top, a season of the order left
 * meaning nothing (episode-orderings.md §7 Q10), one chosen before its contents were fetched said to
 * be fetched on the page (OrderingFetching, §4); a files' order chosen is kept and the series' match
 * queued, so the library is filed again by it. Either then reads the page again (onChosen).
 */
export function useOrderingActions(
  series: SeriesDetail,
  onChosen: (() => void) | undefined,
): OrderingActions {
  const { t } = useTranslation();
  const { api, navigation } = useServices();
  const isAdmin = use(IsAdminContext);
  const watching = useRequest((cause) => t("ordering.failed", { why: problemText(t, cause) }));
  const filing = useRequest((cause) => t("ordering.filesFailed", { why: problemText(t, cause) }));
  const orderings = series.orderings ?? [];
  if (orderings.length < 2) {
    return { actions: [], told: undefined };
  }
  const watched = orderingSubmenu(
    t,
    { orderings, current: series.ordering ?? orderingAired },
    (chosen) => {
      watching.runRequest(async () => {
        throwIfRefused(
          await api.PUT("/items/{id}/preferences", {
            params: { path: { id: series.id } },
            body: { episodeOrdering: chosen },
          }),
        );
        // Another ordering is a view of the same series: no step in the history (D367 §9).
        navigation.replace({ screen: "series-page", collectionId: series.id });
        onChosen?.();
      });
    },
  );
  const filed = orderingSubmenu(
    t,
    { orderings, current: series.filesOrdering ?? orderingAired },
    (chosen) => {
      filing.runRequest(async () => {
        throwIfRefused(
          await api.PUT("/items/{id}/files-ordering", {
            params: { path: { id: series.id } },
            body: { filesOrdering: chosen },
          }),
        );
        onChosen?.();
      });
    },
  );
  const actions: MoreAction[] = [{ key: "ordering", label: t("ordering.label"), submenu: watched }];
  if (isAdmin) {
    actions.push({ key: "files-ordering", label: t("ordering.filesLabel"), submenu: filed });
  }
  const problem = watching.problem ?? filing.problem;
  return { actions, told: problem === undefined ? undefined : { kind: "problem", text: problem } };
}

/** A choice among a series' orderings, as a list of the "…": each by its name, the current marked. */
function orderingSubmenu(
  t: TFunction,
  of: { readonly orderings: readonly Ordering[]; readonly current: string },
  onChoose: (chosen: string) => void,
): MoreSubmenu {
  const { orderings, current } = of;
  const choices = orderings.map((ordering) => ({
    key: ordering.name,
    label: orderingName(t, ordering),
    isCurrent: ordering.name === current,
    onChoose: () => {
      onChoose(ordering.name);
    },
  }));
  const shown = orderings.find((ordering) => ordering.name === current);
  return { value: shown === undefined ? current : orderingName(t, shown), choices };
}

/**
 * How the ordering chosen went, on the page, its choice being in the "…", gone once chosen: why it
 * was not kept (useOrderingActions), and that it is being fetched, until it is
 * (episode-orderings.md §4).
 */
export function OrderingFetching(props: {
  readonly series: SeriesDetail;
  readonly told: SaveTold;
}): JSX.Element {
  const { t } = useTranslation();
  const { series } = props;
  const chosen = (series.orderings ?? []).find((ordering) => ordering.name === series.ordering);
  return (
    <>
      <StatusNote told={props.told} />
      {chosen?.isAwaitingFetch === true ? (
        <p className="series__absent">{t("ordering.fetching")}</p>
      ) : null}
    </>
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
