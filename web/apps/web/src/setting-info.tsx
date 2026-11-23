// SPDX-License-Identifier: MPL-2.0

import "./setting-info.css";
import "./settings-words.ts";

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { camelKey } from "./catalogue-key.ts";
import { IconButton } from "./icon-button.tsx";
import { AboutIcon } from "./icons.tsx";
import { DialogClose, ModalDialog } from "./modal-dialog.tsx";

/** What a setting's or a key's What it is for says. */
export interface SettingInfoProps {
  /** The setting's or key's name in the person's words: the dialog's title. */
  readonly label: string;
  /** Its name in the configuration (backupEveryHours), which an admin's config file uses. */
  readonly configName: string;
  /** What it is for, or, for a question not yet answered, the question (D184). */
  readonly description: string;
  /** Its default as a person reads it; undefined for a key or password, which has none. */
  readonly defaultText?: string | undefined;
}

/**
 * A setting's info button, ⓘ, and the What it is for it opens (D538 §1, pages-spec-3): a dialog, not
 * a help pop-up, because it holds a button (help-button.tsx holds nothing to press). Its title is the
 * label; its body the description, the longer words where the catalogue has them, its name in the
 * configuration and its default. Its foot is Done alone: what puts a setting back to its default is
 * the row's own Use the default (server-settings.tsx; the owner, 2026-11-23, revamp question 16).
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

/** The dialog an info button opens: what the setting is for, and its default. */
function SettingInfoDialog(
  props: SettingInfoProps & { readonly onClose: () => void },
): JSX.Element {
  const { t, i18n } = useTranslation();
  const detailsKey = `settings.details.${camelKey(props.configName)}`;
  const details = i18n.exists(detailsKey) ? t(detailsKey) : "";
  return (
    <ModalDialog
      label={props.label}
      heading={props.label}
      isWide
      onClose={props.onClose}
      footer={<DialogClose onClose={props.onClose} />}
    >
      <div className="setting-info">
        <p>{props.description}</p>
        {details === "" ? null : <p>{details}</p>}
        <p>
          {t("settings.configName")} <code>{props.configName}</code>
        </p>
        {props.defaultText === undefined ? null : (
          <p>{t("settings.defaultIs", { start: props.defaultText })}</p>
        )}
      </div>
    </ModalDialog>
  );
}
