// SPDX-License-Identifier: MPL-2.0

import "./offline-reminder.css";

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { dateText } from "./date-time-text.ts";
import { Link } from "./link.tsx";
import { dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";

/** Whether to remind the admin of what is offline, and how much since when. */
export type OfflineReminderReport = components["schemas"]["OfflineReminderOutputBody"];

/**
 * The reminder an admin sees above every page while library folders or disks are offline, which
 * leads to the System page. Dismissing it hides it from them for offline_reminder_interval_days;
 * it comes back after that while anything is still offline (D231 §7). A reminder that cannot be
 * loaded shows nothing: the System page still says what is offline.
 */
export function OfflineReminder(): JSX.Element | null {
  const { t, i18n } = useTranslation();
  const { api } = useServices();
  const [reminder] = useResource("offline-reminder", async () =>
    dataOrThrow(await api.GET("/offline-reminder")),
  );
  const [isDismissed, setIsDismissed] = useState(false);
  const [problem, setProblem] = useState<string | undefined>(undefined);
  if (reminder.state !== "loaded" || !reminder.value.isShown || isDismissed) {
    return null;
  }
  const { rootCount, diskCount, since } = reminder.value;
  const folders =
    rootCount === 1
      ? t("offlineReminder.oneFolder")
      : t("offlineReminder.folders", { count: rootCount });
  const disks =
    diskCount === 1
      ? t("offlineReminder.oneDisk")
      : t("offlineReminder.disks", { count: diskCount });
  let what = folders;
  if (rootCount === 0) {
    what = disks;
  } else if (diskCount > 0) {
    what = t("offlineReminder.foldersAndDisks", { folders, disks });
  }
  const dismiss = (): void => {
    api
      .POST("/offline-reminder/dismissal")
      .then((answer) => {
        throwIfRefused(answer);
        setIsDismissed(true);
      })
      .catch((cause: unknown) => {
        setProblem(problemText(t, cause));
      });
  };
  return (
    <aside className="offline-reminder" aria-label={t("offlineReminder.label")}>
      <p className="offline-reminder__text">
        {t("offlineReminder.offlineSince", {
          what,
          since: dateText(since ?? 0, i18n.language),
        })}{" "}
        <Link to={{ screen: "system" }}>{t("offlineReminder.see")}</Link>
      </p>
      <button type="button" onClick={dismiss}>
        {t("offlineReminder.dismiss")}
      </button>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
    </aside>
  );
}
