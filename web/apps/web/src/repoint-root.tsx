// SPDX-License-Identifier: MPL-2.0

import "./libraries-words.ts";

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import type { Api, components } from "@orpheum/sdk";

import { arrivalMark, dangerousMark } from "./focus-navigation.ts";
import { ChooseOpener } from "./file-browser-choose.tsx";
import type { Place } from "./file-browser.tsx";
import { ModalDialog } from "./modal-dialog.tsx";
import { dataOrThrow, problemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { idempotent, newIdempotencyKey } from "./idempotent.ts";
import { StatusNote } from "./status-note.tsx";

export type { Place } from "./file-browser.tsx";

/** A repoint's dry run, as the server answers it: what would move, and why it would be refused. */
export type RepointReport = components["schemas"]["RepointPreviewOutputBody"];

/**
 * The words for each reason the dry run gives that a repoint is refused (repoint-library.md, "The
 * spec"); a reason this client does not know gets general words.
 */
const refusalKeys: ReadonlyMap<string, string> = new Map([
  ["nothingToMove", "libraries.repointRefusedNothingToMove"],
  ["sameFolder", "libraries.repointRefusedSameFolder"],
  ["targetNotReadable", "libraries.repointRefusedTargetNotReadable"],
  ["targetHoldsKnown", "libraries.repointRefusedTargetHoldsKnown"],
  ["libraryWouldNest", "libraries.repointRefusedLibraryWouldNest"],
  ["scanInProgress", "libraries.repointRefusedScanInProgress"],
]);

/** Whether two places are one: the same node and path. */
function isSamePlace(a: Place, b: Place): boolean {
  return a.nodeName === b.nodeName && a.path === b.path;
}

/**
 * Asks the server what saying from moved to to would do, changing nothing (D429 §2); a press of
 * Check, keyed as every click's request is (D244 §4).
 */
async function previewRepoint(api: Api, from: Place, to: Place): Promise<RepointReport> {
  return dataOrThrow(
    await api.POST("/repoints/preview", {
      ...idempotent(newIdempotencyKey()),
      body: {
        nodeName: from.nodeName,
        path: from.path,
        newNodeName: to.nodeName,
        newPath: to.path,
      },
    }),
  );
}

/**
 * Saying a folder moved (D429 §2), its "This folder moved…" under its "…" (ui-audit-2026-11-04.md §3,
 * A library): a dialog where the admin chooses where it is now in the file browser's Choose (D539 §2),
 * which opens at the folder's own place, or the nearest folder above it that is there, then checks what
 * the move would do, a dry run that changes nothing, and only then, when nothing refuses it, confirms;
 * everything known under the folder takes the new path with its history, and the next scan confirms
 * each file. The folder's own place is not offered. "Keep it where it is", the least harmful choice,
 * comes before the check and the move, which is marked a dangerous action, and is focused as it opens
 * (D421): marked the arrival by hand. It starts at the first other place offered.
 */
export function RepointDialog(props: {
  readonly from: Place;
  readonly places: readonly Place[];
  readonly onRepoint: (to: Place) => void;
  readonly onClose: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const heading = t("libraries.repointHeading");
  return (
    // Its check, preview and apply become a page, "A library folder moved", in the pages batch
    // (dialogs-spec, ui.md rule 6); until then it keeps today's behaviour.
    <ModalDialog label={heading} heading={heading} onClose={props.onClose}>
      <RepointForm
        from={props.from}
        places={props.places}
        onRepoint={(to) => {
          props.onClose();
          props.onRepoint(to);
        }}
        onKeep={props.onClose}
      />
    </ModalDialog>
  );
}

/** A dry run of a move to one place: its report, or why it could not be checked. */
interface Checked {
  readonly to: Place;
  readonly report?: RepointReport;
  readonly failure?: string;
}

/**
 * Where the folder is now, the first other place offered until another is chosen in Choose, then
 * checked, and confirmed or kept where it is. Choosing another place forgets the last check.
 */
function RepointForm(props: {
  readonly from: Place;
  readonly places: readonly Place[];
  readonly onRepoint: (to: Place) => void;
  readonly onKeep: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [chosen, setChosen] = useState<Place | undefined>(() =>
    props.places.find((place) => !isSamePlace(place, props.from)),
  );
  const { report, check, forget } = useRepointCheck(props.from, chosen);
  return (
    <>
      <RepointChoice
        chosen={chosen}
        canMove={report?.report !== undefined && (report.report.refusals ?? []).length === 0}
        onCheck={check}
        onConfirm={() => {
          if (chosen !== undefined) {
            props.onRepoint(chosen);
          }
        }}
        onKeep={props.onKeep}
      />{" "}
      <ChooseOpener
        opener={t("libraries.browseInside")}
        heading={t("libraries.repointHeading")}
        places={props.places}
        startAt={props.from}
        onChoose={(place) => {
          if (!isSamePlace(place, props.from)) {
            setChosen(place);
            forget();
          }
        }}
      />
      {report?.failure === undefined ? null : (
        <StatusNote told={{ kind: "problem", text: report.failure }} />
      )}
      {report?.report === undefined ? null : <RepointReportView report={report.report} />}
    </>
  );
}

/**
 * The dry run of moving from to chosen: check asks the server, its answer or failure kept for that
 * place alone, so choosing another shows none until it is checked; forget drops it.
 */
function useRepointCheck(
  from: Place,
  chosen: Place | undefined,
): {
  readonly report: Checked | undefined;
  readonly check: () => void;
  readonly forget: () => void;
} {
  const { t } = useTranslation();
  const { api } = useServices();
  const [checked, setChecked] = useState<Checked | undefined>(undefined);
  const report =
    checked !== undefined && chosen !== undefined && isSamePlace(checked.to, chosen)
      ? checked
      : undefined;
  const check = (): void => {
    if (chosen === undefined) {
      return;
    }
    previewRepoint(api, from, chosen).then(
      (answer) => {
        setChecked({ to: chosen, report: answer });
      },
      (cause: unknown) => {
        setChecked({ to: chosen, failure: problemText(t, cause) });
      },
    );
  };
  return {
    report,
    check,
    forget: () => {
      setChecked(undefined);
    },
  };
}

/**
 * Where the folder is now, and keeping it where it is, checking the move, or, once checked and not
 * refused, confirming it.
 */
function RepointChoice(props: {
  readonly chosen: Place | undefined;
  readonly canMove: boolean;
  readonly onCheck: () => void;
  readonly onConfirm: () => void;
  readonly onKeep: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <span>
      {t("libraries.repointTo")}{" "}
      <output className="repoint-root__chosen">
        {props.chosen === undefined
          ? null
          : t("libraries.root", { node: props.chosen.nodeName, path: props.chosen.path })}
      </output>{" "}
      {t("libraries.repointConsequence")}{" "}
      <button type="button" {...arrivalMark} onClick={props.onKeep}>
        {t("libraries.repointKeep")}
      </button>{" "}
      <button type="button" onClick={props.onCheck}>
        {t("libraries.repointCheck")}
      </button>
      {props.canMove ? (
        <>
          {" "}
          <button type="button" {...dangerousMark} onClick={props.onConfirm}>
            {t("libraries.repointConfirm")}
          </button>
        </>
      ) : null}
    </span>
  );
}

/**
 * What the move would do, from its dry run: how many places move, a few of them, the library folders
 * that go with it, what each library gains and loses, and each reason it is refused, with what is in
 * the way at the new folder.
 */
function RepointReportView(props: { readonly report: RepointReport }): JSX.Element {
  const { t } = useTranslation();
  const { report } = props;
  return (
    <output>
      <p>
        {t("libraries.repointMoves", {
          live: report.livePlaceCount,
          removed: report.removedPlaceCount,
        })}
      </p>
      <ul>
        {(report.examples ?? []).map((example) => (
          <li key={example.path}>
            {t("libraries.repointExample", { path: example.path, newPath: example.newPath })}
          </li>
        ))}
        {(report.roots ?? []).map((root) => (
          <li key={`${String(root.libraryId)}:${root.path}`}>
            {t("libraries.repointRootMoves", {
              library: root.libraryName,
              path: root.path,
              newPath: root.newPath,
            })}
          </li>
        ))}
        {(report.libraries ?? []).map((change) => (
          <li key={change.libraryId}>
            {t("libraries.repointLibraryChange", {
              library: change.libraryName,
              gains: change.gainsCount,
              loses: change.losesCount,
            })}
          </li>
        ))}
      </ul>
      {report.isCrossNode ? <p>{t("libraries.repointCrossNode")}</p> : null}
      {report.lookAlikeCount === 0 ? null : (
        <p>{t("libraries.repointLookAlikes", { count: report.lookAlikeCount })}</p>
      )}
      <RepointRefusals report={report} />
    </output>
  );
}

/** Each reason the move is refused, and what is already known at the new folder. */
function RepointRefusals(props: { readonly report: RepointReport }): JSX.Element | null {
  const { t } = useTranslation();
  const { report } = props;
  const refusals = report.refusals ?? [];
  if (refusals.length === 0) {
    return null;
  }
  return (
    <div role="alert">
      <p>{t("libraries.repointRefused")}</p>
      <ul>
        {refusals.map((refusal) => (
          <li key={refusal}>{t(refusalKeys.get(refusal) ?? "libraries.repointRefusedOther")}</li>
        ))}
      </ul>
      {report.conflictCount === 0 ? null : (
        <>
          <p>{t("libraries.repointConflicts", { count: report.conflictCount })}</p>
          <ul>
            {(report.conflictExamples ?? []).map((path) => (
              <li key={path}>
                <code>{path}</code>
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  );
}
