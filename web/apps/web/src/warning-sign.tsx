// SPDX-License-Identifier: MPL-2.0

import "./warning-sign.css";

import { createContext, use, useId, useState, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { WarningIcon } from "./icons.tsx";
/**
 * Whether the person signed in runs the server, so a warning sign says what is wrong and how to fix
 * it, or only that something is and to tell whoever does (D278 §3). Anyone not known to be one is
 * not.
 */
export const IsAdminContext = createContext(false);

/**
 * The sign beside something with a problem that affects it (D278 §1): the warning triangle, which
 * opens what is wrong (§2). An administrator is told what is wrong and how to fix it; anyone else
 * only that something is, and to tell whoever runs the server (§3).
 */
export function WarningSign(props: {
  /** What is wrong, in a few words: the sign's name and the detail's heading. */
  readonly summary: string;
  /** What an administrator is told: what is wrong, and how to fix it when that is known. */
  readonly forAdmin: ReactNode;
  /** What anyone else is told, before being asked to tell the administrator; the summary when absent. */
  readonly forViewer?: ReactNode;
  /** The sign's icon: the warning triangle unless another cue's (an uncertain match's, D301 §4). */
  readonly icon?: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  const isAdmin = use(IsAdminContext);
  const [isOpen, setIsOpen] = useState(false);
  const detailId = useId();
  return (
    <span className="warning-sign">
      <button
        type="button"
        className="warning-sign__button"
        aria-label={t("warning.show", { summary: props.summary })}
        aria-expanded={isOpen}
        aria-controls={detailId}
        onClick={(event) => {
          event.preventDefault();
          event.stopPropagation();
          setIsOpen(!isOpen);
        }}
      >
        {props.icon ?? <WarningIcon />}
      </button>
      {isOpen ? (
        <span className="warning-sign__detail" id={detailId} role="note">
          <strong className="warning-sign__summary">{props.summary}</strong>
          {isAdmin ? (
            <span className="warning-sign__body">{props.forAdmin}</span>
          ) : (
            <>
              {props.forViewer === undefined ? null : (
                <span className="warning-sign__body">{props.forViewer}</span>
              )}
              <span className="warning-sign__body">{t("warning.tellAdmin")}</span>
            </>
          )}
        </span>
      ) : null}
    </span>
  );
}
