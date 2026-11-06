// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import type { Api, components } from "@orpheum/sdk";

import { arrivalMark, dangerousMark } from "./focus-navigation.ts";
import { FolderBrowser, type Place } from "./folder-picker.tsx";
import { ModalDialog } from "./modal-dialog.tsx";
import { dataOrThrow, problemText } from "./problem.ts";
import { useServices } from "./services.tsx";

export type { Place } from "./folder-picker.tsx";

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

/** Asks the server what saying from moved to to would do, changing nothing (D429 §2). */
async function previewRepoint(api: Api, from: Place, to: Place): Promise<RepointReport> {
  return dataOrThrow(
    await api.POST("/repoints/preview", {
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
 * A library): a dialog where the admin chooses where it is now, among the connected nodes' roots, or
 * a folder browsed to inside one in the folder browser's dialog (D359 §3, D361 §3), then checks what
 * the move would do, a dry run that changes nothing, and only then, when nothing refuses it, confirms;
 * everything known under the folder takes the new path with its history, and the next scan confirms
 * each file. The folder's own place is not offered. "Keep it where it is", the least harmful choice,
 * comes before the check and the move, which is marked a dangerous action, and is focused as it opens
 * (D421): marked the arrival by hand, the place to choose being the dialog's first control.
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
    <ModalDialog label={heading} onClose={props.onClose}>
      <h2>{heading}</h2>
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
 * Where the folder is now, chosen among the places offered or a folder browsed to inside one, then
 * checked, and confirmed or kept where it is. Choosing another place forgets the last check.
 */
function RepointForm(props: {
  readonly from: Place;
  readonly places: readonly Place[];
  readonly onRepoint: (to: Place) => void;
  readonly onKeep: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [browsed, setBrowsed] = useState<Place | undefined>(undefined);
  const offered = props.places.filter((place) => !isSamePlace(place, props.from));
  const choices = browsed === undefined ? offered : [...offered, browsed];
  const [chosenIndex, setChosenIndex] = useState(0);
  const [checked, setChecked] = useState<Checked | undefined>(undefined);
  const chosen = choices[chosenIndex];
  const report =
    checked !== undefined && chosen !== undefined && isSamePlace(checked.to, chosen)
      ? checked
      : undefined;
  const choose = (index: number): void => {
    setChosenIndex(index);
    setChecked(undefined);
  };
  return (
    <>
      <RepointChoice
        choices={choices}
        chosenIndex={chosenIndex}
        canMove={report?.report !== undefined && (report.report.refusals ?? []).length === 0}
        onChoose={choose}
        onCheck={() => {
          if (chosen === undefined) {
            return;
          }
          previewRepoint(api, props.from, chosen).then(
            (answer) => {
              setChecked({ to: chosen, report: answer });
            },
            (cause: unknown) => {
              setChecked({ to: chosen, failure: problemText(t, cause) });
            },
          );
        }}
        onConfirm={() => {
          if (chosen !== undefined) {
            props.onRepoint(chosen);
          }
        }}
        onKeep={props.onKeep}
      />{" "}
      <FolderBrowser
        opener={t("libraries.browseInside")}
        heading={t("libraries.repointHeading")}
        starts={props.places}
        onChoose={(place) => {
          const known = offered.findIndex((each) => isSamePlace(each, place));
          if (!isSamePlace(place, props.from)) {
            setBrowsed(known === -1 ? place : undefined);
            choose(known === -1 ? offered.length : known);
          }
        }}
      />
      {report?.failure === undefined ? null : <p role="alert">{report.failure}</p>}
      {report?.report === undefined ? null : <RepointReportView report={report.report} />}
    </>
  );
}

/**
 * Where the folder is now, chosen among the places, and keeping it where it is, checking the move, or,
 * once checked and not refused, confirming it.
 */
function RepointChoice(props: {
  readonly choices: readonly Place[];
  readonly chosenIndex: number;
  readonly canMove: boolean;
  readonly onChoose: (index: number) => void;
  readonly onCheck: () => void;
  readonly onConfirm: () => void;
  readonly onKeep: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <span>
      <label>
        {t("libraries.repointTo")}{" "}
        <select
          value={props.chosenIndex}
          onChange={(event) => {
            props.onChoose(Number(event.currentTarget.value));
          }}
        >
          {props.choices.map((place, index) => (
            <option key={`${place.nodeName}:${place.path}`} value={index}>
              {t("libraries.root", { node: place.nodeName, path: place.path })}
            </option>
          ))}
        </select>
      </label>{" "}
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
    <div role="status">
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
    </div>
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
