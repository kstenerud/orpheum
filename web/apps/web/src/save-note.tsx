// SPDX-License-Identifier: MPL-2.0

import "./save-note.css";

import { useCallback, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { problemText } from "./problem.ts";

/**
 * How a setting's last change went, where every setting saves at once (the owner, 2026-11-04, Q5):
 * saved, refused with why, or nothing yet.
 */
export type SaveState =
  { readonly kind: "saved" } | { readonly kind: "refused"; readonly problem: string } | undefined;

/**
 * The state of a field whose page keeps its refusals and saves by name: refused where it has a
 * problem, saved where its last change was, else nothing.
 */
export function shownSaveState(problem: string, isSaved: boolean): SaveState {
  if (problem !== "") {
    return { kind: "refused", problem };
  }
  return isSaved ? { kind: "saved" } : undefined;
}

/**
 * What a setting says beside its field once a change is sent, the one way the admin's pages say it
 * (rule 11): "Saved", or the refusal in its place, in the warning colour, as an alert.
 */
export function SaveNote(props: { readonly state: SaveState }): JSX.Element | null {
  const { t } = useTranslation();
  const { state } = props;
  if (state === undefined) {
    return null;
  }
  if (state.kind === "refused") {
    return (
      <p className="save-note save-note--refused" role="alert">
        {state.problem}
      </p>
    );
  }
  return <output className="save-note">{t("common.saved")}</output>;
}

/**
 * One field's saving at once: save sends a change and the state says how it went, for SaveNote; the
 * refusal in the viewer's words (problemText), or a form's own words where it passes them.
 */
export function useSaveState(): {
  readonly state: SaveState;
  readonly save: (send: () => Promise<void>) => Promise<boolean>;
} {
  const { t } = useTranslation();
  const [state, setState] = useState<SaveState>(undefined);
  const save = useCallback(
    async (send: () => Promise<void>): Promise<boolean> => {
      try {
        await send();
        setState({ kind: "saved" });
        return true;
      } catch (cause: unknown) {
        setState({ kind: "refused", problem: problemText(t, cause) });
        return false;
      }
    },
    [t],
  );
  return { state, save };
}
