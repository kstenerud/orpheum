// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { ConfirmButton } from "./confirm-button.tsx";
import { dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";

type Unused = components["schemas"]["UnusedBody"];

/**
 * The settings this release does not read, each with why, and deleting one: listed and deleted only
 * by a person, never on their own (D202 §2). Nothing shows while there are none.
 */
export function SystemUnusedSettings(): JSX.Element | null {
  const { t } = useTranslation();
  const { api } = useServices();
  const [unused, reload] = useResource("unused-settings", async () =>
    dataOrThrow(await api.GET("/config/unused")),
  );
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  const remove = (key: string): void => {
    runRequest(async () => {
      throwIfRefused(await api.DELETE("/config/unused/{key}", { params: { path: { key } } }));
      reload();
    });
  };
  const listed = unused.state === "loaded" ? (unused.value.unusedSettings ?? []) : [];
  if (listed.length === 0 && problem === undefined) {
    return null;
  }
  return (
    <section aria-labelledby="system-unused-settings">
      <h2 id="system-unused-settings">{t("system.unusedSettings.heading")}</h2>
      <p>{t("system.unusedSettings.what")}</p>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
      <ul>
        {listed.map((setting) => (
          <li key={setting.key}>
            {unusedText(t, setting)}{" "}
            <ConfirmButton
              label={t("system.unusedSettings.delete")}
              labelForReader={t("system.unusedSettings.deleteFor", { key: setting.key })}
              isDisabled={isBusy}
              consequence={t("system.unusedSettings.consequence", { key: setting.key })}
              confirmLabel={t("system.unusedSettings.confirm")}
              onConfirm={() => {
                remove(setting.key);
              }}
            />
          </li>
        ))}
      </ul>
    </section>
  );
}

/** What an unused setting holds, and why nothing reads it. */
function unusedText(t: TFunction, setting: Unused): string {
  const value = JSON.stringify(setting.value);
  return setting.retiredIn === undefined || setting.retiredIn === ""
    ? t("system.unusedSettings.typo", { key: setting.key, value })
    : t("system.unusedSettings.retired", {
        key: setting.key,
        value,
        release: setting.retiredIn,
        reason: setting.retiredReason ?? "",
      });
}
