// SPDX-License-Identifier: MPL-2.0

import "./libraries-words.ts";

import type { Api, paths } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { identifyProblemText } from "./candidate-chooser.tsx";
import { ConfirmDialog } from "./confirm-button.tsx";
import { idempotent, newIdempotencyKey } from "./idempotent.ts";
import type { MoreAction } from "./more-actions.tsx";
import { showNotice } from "./notice.ts";
import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";

/**
 * An admin's "Rescan" in a video page's "…" menu, a film's, a series', an episode's or an other
 * video's (D535 §3, §5): the page's files read again entirely, a series' every episode's, what their
 * differences touch thrown out (D392), and a film or a series then matched again unless a person
 * chose its match (D357). It is the one place a video's page offers it; the "Identified incorrectly"
 * dialog does not. The page gives it to an admin alone, and never on an extra's page.
 */
export function rescanAction(
  t: ReturnType<typeof useTranslation>["t"],
  asked: { readonly itemId: number; readonly onStarted: () => void },
): MoreAction {
  return {
    key: "rescan",
    label: t("libraries.rescan"),
    dialog: (close) => (
      <ItemRescanQuestion itemId={asked.itemId} onStarted={asked.onStarted} onClose={close} />
    ),
  };
}

/**
 * The question every Rescan asks first (D535 §6), the confirm (ui.md rule 8): that it is expensive, a
 * library's that it can take hours, with an unticked "Discard cached data", which has the rescan
 * throw out everything made or fetched for its scope, whatever differs, then Cancel, focused, and
 * "Start the rescan" (D535 §6's Yes and No, named as every confirm's are). Cancel, Escape or Back send
 * nothing.
 */
export function RescanQuestion(props: {
  readonly isLong: boolean;
  readonly onRescan: (isDiscarding: boolean) => void;
  readonly onClose: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <ConfirmDialog
      question={t("libraries.rescanQuestion")}
      consequence={props.isLong ? t("libraries.rescanAskLibrary") : t("libraries.rescanAskItem")}
      confirmLabel={t("libraries.rescanConfirm")}
      isHarmless
      option={t("libraries.rescanDiscard")}
      onConfirm={props.onRescan}
      onClose={props.onClose}
    />
  );
}

/** A rescan's query, the same on each of the three rescans (D535 §6). */
type RescanQuery = NonNullable<paths["/items/{id}/rescan"]["post"]["parameters"]["query"]>;

/** A rescan request's query: discard, when the question's box was ticked (D535 §6). */
export function discardQuery(isDiscarding: boolean): RescanQuery {
  return isDiscarding ? { discard: true } : {};
}

/**
 * The question a video page's Rescan opens, alone: Yes closes it at once (the owner, 2026-11-22: "It
 * should have dismissed the box when I rescanned"), and the page, read again, says "Rescanning…" in
 * its facts while the rescan runs (D535 §7): a notice seconds after the press could land on another
 * page. Only a failure to start is told in the shell's notice (notice.ts).
 */
function ItemRescanQuestion(props: {
  readonly itemId: number;
  readonly onStarted: () => void;
  readonly onClose: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  return (
    <RescanQuestion
      isLong={false}
      onClose={props.onClose}
      onRescan={(isDiscarding) => {
        void rescanItem(api, t, { itemId: props.itemId, isDiscarding, onStarted: props.onStarted });
      }}
    />
  );
}

/**
 * Asks the item's rescan, then has the page read again, which then says it is being rescanned, the
 * rescan started now or one already running; says in the notice why it could not start.
 */
async function rescanItem(
  api: Api,
  t: ReturnType<typeof useTranslation>["t"],
  asked: {
    readonly itemId: number;
    readonly isDiscarding: boolean;
    readonly onStarted: () => void;
  },
): Promise<void> {
  const { itemId, isDiscarding } = asked;
  try {
    dataOrThrow(
      await api.POST("/items/{id}/rescan", {
        params: { path: { id: itemId }, query: discardQuery(isDiscarding) },
        ...idempotent(newIdempotencyKey()),
      }),
    );
    asked.onStarted();
  } catch (cause: unknown) {
    showNotice(identifyProblemText(t, cause));
  }
}
