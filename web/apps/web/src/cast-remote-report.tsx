// SPDX-License-Identifier: MPL-2.0

import { expectedGroupPositionMs, type PlaybackReport, type TogetherSnapshot } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { ListOption } from "./list-box.tsx";
import { ProblemReportDialog, qualityOf, type ReportedPlayback } from "./problem-report.tsx";
import { monotonicMs } from "./use-together.ts";

/** What the remote's report takes besides the receiver's playback: the film and the session. */
export interface RemoteReport {
  /** The film the cast plays. */
  readonly fileId: number;
  /** The session the remote follows the receiver in, its group saying where the film stands. */
  readonly snapshot: TogetherSnapshot | undefined;
}

/**
 * The receiver's state for a report from the remote's ⁝ (D524 §2), the player's own report taken
 * from the receiver as the cast's session says it: the film's file, where the group stands now,
 * how the receiver plays (its playback) and whether the group plays or is paused.
 */
export function receiverReportedNow(
  report: RemoteReport,
  playback: PlaybackReport | undefined,
  nowMs: number,
): ReportedPlayback {
  const { snapshot } = report;
  const position = snapshot === undefined ? undefined : expectedGroupPositionMs(snapshot, nowMs);
  const state = snapshot?.group?.state;
  return {
    fileId: report.fileId,
    positionMs: Math.max(0, Math.round(position ?? 0)),
    quality: qualityOf(playback),
    state: state === "playing" || state === "paused" ? state : "",
  };
}

/**
 * ⁝'s Report a problem on the cast's remote, as the player's (D524 §2): its entry for the menu, and
 * the dialog, drawn outside the menu, which closes as the entry is chosen. The report carries the
 * receiver's playback in the player's place, taken as the entry is chosen.
 */
export function useReportFromTheRemote(
  report: RemoteReport | undefined,
  playback: PlaybackReport | undefined,
): { readonly entry: JSX.Element | null; readonly dialog: JSX.Element | null } {
  const { t } = useTranslation();
  const [reported, setReported] = useState<ReportedPlayback | undefined>(undefined);
  if (report === undefined) {
    return { entry: null, dialog: null };
  }
  return {
    entry: (
      <ListOption
        onChoose={() => {
          setReported(receiverReportedNow(report, playback, monotonicMs()));
        }}
      >
        {t("problemReport.open")}
      </ListOption>
    ),
    dialog:
      reported === undefined ? null : (
        <ProblemReportDialog
          playback={reported}
          onClose={() => {
            setReported(undefined);
          }}
        />
      ),
  };
}
