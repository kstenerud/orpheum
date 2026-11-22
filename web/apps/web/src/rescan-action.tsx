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
  itemId: number,
): MoreAction {
  return {
    key: "rescan",
    label: t("libraries.rescan"),
    dialog: (close) => <ItemRescanQuestion itemId={itemId} onClose={close} />,
  };
}

/**
 * The question every Rescan asks first (D535 §6): that it is expensive, a library's that it can take
 * hours, with Yes, No and an unticked "Discard cached data", which has the rescan throw out
 * everything made or fetched for its scope, whatever differs. No, Escape or Back send nothing.
 */
export function RescanQuestion(props: {
  readonly isLong: boolean;
  readonly onRescan: (isDiscarding: boolean) => void;
  readonly onClose: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <ConfirmDialog
      heading={t("libraries.rescan")}
      consequence={props.isLong ? t("libraries.rescanAskLibrary") : t("libraries.rescanAskItem")}
      confirmLabel={t("libraries.rescanYes")}
      keepLabel={t("libraries.rescanNo")}
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
 * The question a video page's Rescan opens, alone: Yes closes it at once, and the rescan's start, or
 * its failure to start, is told in the shell's notice (notice.ts), outside any dialog (the owner,
 * 2026-11-22: "It should have dismissed the box when I rescanned").
 */
function ItemRescanQuestion(props: {
  readonly itemId: number;
  readonly onClose: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  return (
    <RescanQuestion
      isLong={false}
      onClose={props.onClose}
      onRescan={(isDiscarding) => {
        void rescanItem(api, t, { itemId: props.itemId, isDiscarding });
      }}
    />
  );
}

/** Asks the item's rescan, then says in the notice whether it started, or why it could not. */
async function rescanItem(
  api: Api,
  t: ReturnType<typeof useTranslation>["t"],
  asked: { readonly itemId: number; readonly isDiscarding: boolean },
): Promise<void> {
  const { itemId, isDiscarding } = asked;
  try {
    const answer = dataOrThrow(
      await api.POST("/items/{id}/rescan", {
        params: { path: { id: itemId }, query: discardQuery(isDiscarding) },
        ...idempotent(newIdempotencyKey()),
      }),
    );
    showNotice(
      answer.isStarted ? t("libraries.rescanThisStarted") : t("libraries.rescanThisRunning"),
    );
  } catch (cause: unknown) {
    showNotice(identifyProblemText(t, cause));
  }
}
