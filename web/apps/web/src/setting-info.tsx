// SPDX-License-Identifier: MPL-2.0

import "./setting-info.css";
import "./settings-words.ts";

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { camelKey } from "./catalogue-key.ts";
import { arrivalMark } from "./focus-navigation.ts";
import { IconButton } from "./icon-button.tsx";
import { AboutIcon } from "./icons.tsx";
import { DialogClose, ModalDialog } from "./modal-dialog.tsx";

/** What a setting's or a key's What it is for says, and what its Use the default does. */
export interface SettingInfoProps {
  /** The setting's or key's name in the person's words: the dialog's title. */
  readonly label: string;
  /** Its name in the configuration (backupEveryHours), which an admin's config file uses. */
  readonly configName: string;
  /** What it is for, or, for a question not yet answered, the question (D184). */
  readonly description: string;
  /**
   * Its default as a person reads it, and whether the value differs from it, with what puts it back;
   * undefined for a key or password, which has none.
   */
  readonly default?:
    | {
        readonly text: string;
        readonly isModified: boolean;
        readonly onUse: () => void;
      }
    | undefined;
}

/**
 * A setting's info button, ⓘ, and the What it is for it opens (D538 §1, pages-spec-3): a dialog, not
 * a help pop-up, because it holds a button (help-button.tsx holds nothing to press). Its title is the
 * label; its body the description, the longer words where the catalogue has them, its name in the
 * configuration and its default. While the value differs from the default its foot has Cancel and,
 * its main action, Use the default, which saves the default and closes it, with no confirm: setting
 * it again undoes it. At the default, or for a key, Done alone.
 */
export function SettingInfo(props: SettingInfoProps): JSX.Element {
  const { t } = useTranslation();
  const [isOpen, setIsOpen] = useState(false);
  const label = t("settings.about", { key: props.label });
  const close = (): void => {
    setIsOpen(false);
  };
  return (
    <>
      <IconButton
        label={label}
        icon={<AboutIcon />}
        isExpanded={isOpen}
        onClick={() => {
          setIsOpen(true);
        }}
      />
      {isOpen ? <SettingInfoDialog {...props} onClose={close} /> : null}
    </>
  );
}

/** The dialog an info button opens: what the setting is for, and its way back to the default. */
function SettingInfoDialog(
  props: SettingInfoProps & { readonly onClose: () => void },
): JSX.Element {
  const { t, i18n } = useTranslation();
  const detailsKey = `settings.details.${camelKey(props.configName)}`;
  const details = i18n.exists(detailsKey) ? t(detailsKey) : "";
  const shown = props.default;
  return (
    <ModalDialog
      label={props.label}
      heading={props.label}
      isWide
      onClose={props.onClose}
      footer={
        shown?.isModified === true ? (
          <>
            <button type="button" onClick={props.onClose}>
              {t("dialog.cancel")}
            </button>
            <button
              type="button"
              {...arrivalMark}
              onClick={() => {
                props.onClose();
                shown.onUse();
              }}
            >
              {t("settings.useDefault")}
            </button>
          </>
        ) : (
          <DialogClose onClose={props.onClose} />
        )
      }
    >
      <div className="setting-info">
        <p>{props.description}</p>
        {details === "" ? null : <p>{details}</p>}
        <p>
          {t("settings.configName")} <code>{props.configName}</code>
        </p>
        {shown === undefined ? null : <p>{t("settings.defaultIs", { start: shown.text })}</p>}
      </div>
    </ModalDialog>
  );
}
