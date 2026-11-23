// SPDX-License-Identifier: MPL-2.0

import "./settings-words.ts";

import type { components } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import type { JSX, MouseEvent } from "react";
import { useTranslation } from "react-i18next";

import { ConfirmButton } from "./confirm-button.tsx";
import { dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { StatusNote } from "./status-note.tsx";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";

/** A setting this release does not read, as GET /config/unused lists it. */
export type UnreadSetting = components["schemas"]["UnusedBody"];

/** The fold's id on Settings' index, which its Needs you line leads to. */
export const unreadFoldId = "settings-unread";

/** The settings this release does not read, and deleting one, which reads them again. */
export interface UnreadSettings {
  readonly listed: readonly UnreadSetting[];
  readonly isBusy: boolean;
  readonly problem: string | undefined;
  readonly remove: (key: string) => void;
}

/** Reads the settings this release does not read, deleting one only as a person asks (D202 §2). */
export function useUnreadSettings(): UnreadSettings {
  const { t } = useTranslation();
  const { api } = useServices();
  const [unread, reload] = useResource("unused-settings", async () =>
    dataOrThrow(await api.GET("/config/unused")),
  );
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  const remove = (key: string): void => {
    runRequest(async () => {
      throwIfRefused(await api.DELETE("/config/unused/{key}", { params: { path: { key } } }));
      reload();
    });
  };
  const listed = unread.state === "loaded" ? (unread.value.unusedSettings ?? []) : [];
  return { listed, isBusy, problem, remove };
}

/** Opens the fold in place of following the link, and puts focus on its summary. */
function openFold(event: MouseEvent<HTMLAnchorElement>): void {
  const fold = document.getElementById(unreadFoldId);
  if (!(fold instanceof HTMLDetailsElement)) {
    return;
  }
  event.preventDefault();
  fold.open = true;
  fold.querySelector("summary")?.focus();
}

/**
 * Needs you's line for the settings this release does not read (pages-spec-3), leading to their
 * fold on the index, which it opens: nothing while there are none.
 */
export function UnreadLine(props: { readonly count: number }): JSX.Element | null {
  const { t } = useTranslation();
  if (props.count === 0) {
    return null;
  }
  return (
    <li>
      <a href={`#${unreadFoldId}`} onClick={openFold}>
        {props.count === 1
          ? t("settings.unread.lineOne")
          : t("settings.unread.line", { count: props.count })}
      </a>
    </li>
  );
}

/**
 * The settings this release does not read, folded on Settings' index, each with its value and why,
 * and deleting one only once confirmed (D202 §2). Nothing shows while there are none.
 */
export function UnreadSettingsFold(props: { readonly unread: UnreadSettings }): JSX.Element | null {
  const { t } = useTranslation();
  const { listed, isBusy, problem, remove } = props.unread;
  if (listed.length === 0 && problem === undefined) {
    return null;
  }
  return (
    <details id={unreadFoldId} className="settings__fold">
      <summary>{t("settings.unread.heading")}</summary>
      <p>{t("settings.unread.what")}</p>
      {problem === undefined ? null : <StatusNote told={{ kind: "problem", text: problem }} />}
      <ul>
        {listed.map((setting) => (
          <li key={setting.key}>
            {unreadText(t, setting)}{" "}
            <ConfirmButton
              label={t("settings.unread.delete")}
              labelForReader={t("settings.unread.deleteFor", { key: setting.key })}
              isDisabled={isBusy}
              question={t("settings.unread.question", { key: setting.key })}
              consequence={t("settings.unread.consequence", { key: setting.key })}
              confirmLabel={t("settings.unread.confirm")}
              onConfirm={() => {
                remove(setting.key);
              }}
            />
          </li>
        ))}
      </ul>
    </details>
  );
}

/** What an unread setting holds, and why nothing reads it. */
function unreadText(t: TFunction, setting: UnreadSetting): string {
  const value = JSON.stringify(setting.value);
  return setting.retiredIn === undefined || setting.retiredIn === ""
    ? t("settings.unread.typo", { key: setting.key, value })
    : t("settings.unread.retired", {
        key: setting.key,
        value,
        release: setting.retiredIn,
        reason: setting.retiredReason ?? "",
      });
}
