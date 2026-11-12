// SPDX-License-Identifier: MPL-2.0

import "./more-actions.css";

import { useState, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { DropDown } from "./drop-down.tsx";
import { MoreIcon } from "./icons.tsx";
import { ListOption } from "./list-box.tsx";

/**
 * One of a page's rarely used actions: its name in the menu, and either the dialog it opens, given
 * how to close it, or what it does at once, chosen, where it can be undone (a card's Hide).
 */
export type MoreAction = {
  readonly key: string;
  readonly label: string;
} & ({ readonly dialog: (close: () => void) => ReactNode } | { readonly onChoose: () => void });

/**
 * A page's rarely used actions behind a vertical "…" (the owner, 2026-10-29: "these
 * unlikely-to-be-used UI items should be under a vertical '…' selector so that they don't clutter
 * the UI"): the one drop-down (drop-down.tsx) with no chevron, as the banner's menu is, each place
 * opening its own dialog, which takes the list's place rather than stacking over it, so one press
 * outside it is back on the page (the owner, 2026-10-29), or doing what it does at once. Nothing
 * where the page has none for this person. label names it where a page has more than one: a card's,
 * by the card's name.
 */
export function MoreActions(props: {
  readonly actions: readonly MoreAction[];
  readonly label?: string;
  readonly className?: string;
}): JSX.Element | null {
  const { t } = useTranslation();
  const [openKey, setOpenKey] = useState<string | undefined>(undefined);
  if (props.actions.length === 0) {
    return null;
  }
  const label = props.label ?? t("moreActions.label");
  const opened = props.actions.find((action) => action.key === openKey);
  return (
    <>
      <DropDown
        label={label}
        opener={<MoreIcon />}
        openerClassName={
          props.className === undefined
            ? "more-actions__button"
            : `more-actions__button ${props.className}`
        }
        openerNaming={{ "aria-label": label, title: label }}
        isFromItsTop
      >
        {props.actions.map((action) => (
          <ListOption
            key={action.key}
            onChoose={() => {
              if ("onChoose" in action) {
                action.onChoose();
              } else {
                setOpenKey(action.key);
              }
            }}
          >
            {action.label}
          </ListOption>
        ))}
      </DropDown>
      {opened === undefined || !("dialog" in opened)
        ? null
        : opened.dialog(() => {
            setOpenKey(undefined);
          })}
    </>
  );
}
