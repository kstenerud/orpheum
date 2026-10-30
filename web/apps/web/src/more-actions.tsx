// SPDX-License-Identifier: MPL-2.0

import "./more-actions.css";

import { useState, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { DropDown } from "./drop-down.tsx";
import { Icon } from "./icon.tsx";

/**
 * One of a page's rarely used actions: its name in the menu, and the dialog it opens, given how to
 * close it.
 */
export interface MoreAction {
  readonly key: string;
  readonly label: string;
  readonly dialog: (close: () => void) => ReactNode;
}

/** Three dots one above the other: the mark of a page's menu of rarely used actions. */
function MoreIcon(): JSX.Element {
  return (
    <Icon className="more-actions__icon">
      <circle cx="12" cy="5" r="2" />
      <circle cx="12" cy="12" r="2" />
      <circle cx="12" cy="19" r="2" />
    </Icon>
  );
}

/**
 * A page's rarely used actions behind a vertical "…" (the owner, 2026-10-29: "these
 * unlikely-to-be-used UI items should be under a vertical '…' selector so that they don't clutter
 * the UI"): the one drop-down (drop-down.tsx) with no chevron, as the banner's menu is, each place
 * opening its own dialog, which takes the list's place rather than stacking over it, so one press
 * outside it is back on the page (the owner, 2026-10-29). Nothing where the page has none for this
 * person.
 */
export function MoreActions(props: {
  readonly actions: readonly MoreAction[];
}): JSX.Element | null {
  const { t } = useTranslation();
  const [openKey, setOpenKey] = useState<string | undefined>(undefined);
  if (props.actions.length === 0) {
    return null;
  }
  const label = t("moreActions.label");
  const opened = props.actions.find((action) => action.key === openKey);
  return (
    <>
      <DropDown
        label={label}
        opener={<MoreIcon />}
        openerClassName="more-actions__button"
        openerNaming={{ "aria-label": label, title: label }}
        isFromItsTop
      >
        {props.actions.map((action) => (
          <li key={action.key}>
            <button
              type="button"
              onClick={() => {
                setOpenKey(action.key);
              }}
            >
              {action.label}
            </button>
          </li>
        ))}
      </DropDown>
      {opened === undefined
        ? null
        : opened.dialog(() => {
            setOpenKey(undefined);
          })}
    </>
  );
}
