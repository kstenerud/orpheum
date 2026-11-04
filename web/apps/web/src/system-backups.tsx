// SPDX-License-Identifier: MPL-2.0

import "./system.css";

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
import { Loaded } from "./loaded.tsx";
import { ModalDialog } from "./modal-dialog.tsx";
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
  const [isAsking, setIsAsking] = useState(false);
  return (
    <section aria-labelledby="system-backups">
      <div className="system__heading-row">
        <h2 id="system-backups">{t("system.backups.heading")}</h2>
        <button
          type="button"
          onClick={() => {
            setIsAsking(true);
          }}
        >
          {t("system.backups.make")}
        </button>
      </div>
      <p>{t("system.backups.what")}</p>
      {isAsking ? (
        <AskForBackup
          onAsked={() => {
            setIsAsking(false);
            setIsAsked(true);
            reload();
          }}
          onClose={() => {
            setIsAsking(false);
          }}
        />
      ) : null}
      {isAsked ? <output>{t("system.backups.asked")}</output> : null}
      <Loaded resource={backups} render={(value) => <BackupList backups={value} />} />
    </section>
  );
}

/**
 * The dialog "Back up now" opens (the owner, 2026-11-04): whether to put the server's keys in the
 * backup, and the request. Making a backup is not dangerous, so focus arrives on its first field
 * (D421). It closes once the backup is asked for; a refusal is said in it, and it stays.
 */
function AskForBackup(props: {
  readonly onAsked: () => void;
  readonly onClose: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
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
      props.onAsked();
    });
  };
  return (
    <ModalDialog label={t("system.backups.make")} isWide onClose={props.onClose}>
      <h2>{t("system.backups.make")}</h2>
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
      {shouldIncludeKey ? <p>{t("system.backups.includeKeyWarning")}</p> : null}
      {problem === undefined ? null : <p role="alert">{problem}</p>}
      <button type="button" disabled={isBusy} onClick={makeBackup}>
        {t("system.backups.create")}
      </button>
    </ModalDialog>
  );
}

/** The backups directory, newest first, folded under their count, and where it is. */
function BackupList(props: { readonly backups: Backups }): JSX.Element {
  const { t } = useTranslation();
  const listed = props.backups.backups ?? [];
  return (
    <>
      <p>{t("system.backups.where", { directory: props.backups.directory })}</p>
      {listed.length === 0 ? (
        <p>{t("system.backups.none")}</p>
      ) : (
        <details className="system__fold">
          <summary>{t("system.backups.list", { count: listed.length })}</summary>
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
        </details>
      )}
    </>
  );
}
