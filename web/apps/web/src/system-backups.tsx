// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { idempotent } from "./idempotent.ts";
import { dateTimeText } from "./date-time-text.ts";
import { i18n } from "./i18n.ts";
import { byteSizeText } from "./number-text.ts";
import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow, problemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";

type Backups = components["schemas"]["BackupsOutputBody"];

/**
 * Backing the server up from the System page (D242 §5): the backups there are, newest first, and
 * asking for one now, a job, with secret.key in it only when the admin asks, for moving to another
 * machine. The server also makes one on its own each day, never with the key (D243 §11).
 */
export function SystemBackups(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [backups, reload] = useResource(
    "backups",
    async () => dataOrThrow(await api.GET("/backups")),
    useBackgroundChangesPoller(),
  );
  const [isAsked, setIsAsked] = useState(false);
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  const [shouldIncludeKey, setShouldIncludeKey] = useState(false);
  const makeBackup = (): void => {
    runRequest(async (idempotencyKey) => {
      dataOrThrow(
        await api.POST("/backups", {
          body: { includesKey: shouldIncludeKey },
          ...idempotent(idempotencyKey),
        }),
      );
      setIsAsked(true);
      reload();
    });
  };
  return (
    <section aria-labelledby="system-backups">
      <h2 id="system-backups">{t("system.backups.heading")}</h2>
      <p>{t("system.backups.what")}</p>
      <label>
        <input
          type="checkbox"
          checked={shouldIncludeKey}
          onChange={(event) => {
            setShouldIncludeKey(event.currentTarget.checked);
          }}
        />{" "}
        {t("system.backups.includeKey")}
      </label>
      {shouldIncludeKey ? <p>{t("system.backups.includeKeyWarning")}</p> : null}{" "}
      <button type="button" disabled={isBusy} onClick={makeBackup}>
        {t("system.backups.make")}
      </button>
      {isAsked ? <output>{t("system.backups.asked")}</output> : null}
      {problem === undefined ? null : <p role="alert">{problem}</p>}
      <Loaded resource={backups} render={(value) => <BackupList backups={value} />} />
    </section>
  );
}

/** The backups directory, newest first, and where it is. */
function BackupList(props: { readonly backups: Backups }): JSX.Element {
  const { t } = useTranslation();
  const listed = props.backups.backups ?? [];
  return (
    <>
      <p>{t("system.backups.where", { directory: props.backups.directory })}</p>
      {listed.length === 0 ? (
        <p>{t("system.backups.none")}</p>
      ) : (
        <ul>
          {listed.map((backup) => (
            <li key={backup.name}>
              {t("system.backups.backup", {
                name: backup.name,
                size: byteSizeText(backup.sizeBytes, i18n.language),
                at: dateTimeText(backup.madeAt, i18n.language),
              })}
            </li>
          ))}
        </ul>
      )}
    </>
  );
}
