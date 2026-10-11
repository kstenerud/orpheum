// SPDX-License-Identifier: MPL-2.0

import { useRequest } from "./use-request.ts";
import { idempotent } from "./idempotent.ts";
import "./offline-reminder.css";

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";

import { dateText } from "./date-time-text.ts";
import { Link } from "./link.tsx";
import { dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";

/** Whether to remind the admin of what is offline, and how much since when. */
export type OfflineReminderReport = components["schemas"]["OfflineReminderOutputBody"];

/**
 * The reminder an admin sees above every page while library folders or disks are offline, a scan
 * waits for them before removing files, or a scan could not read something (D277), which leads to
 * the System page. Dismissing it hides it from them for offline_reminder_interval_days;
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
  const dismissing = useRequest((cause) => problemText(t, cause));
  if (reminder.state !== "loaded" || !reminder.value.isShown || isDismissed) {
    return null;
  }
  const lines = reminderLines(t, reminder.value, i18n.language);
  const dismiss = (): void => {
    dismissing.runRequest(async (idempotencyKey) => {
      throwIfRefused(await api.POST("/offline-reminder/dismissal", idempotent(idempotencyKey)));
      setIsDismissed(true);
    });
  };
  return (
    <aside className="offline-reminder" aria-label={t("offlineReminder.label")}>
      <p className="offline-reminder__text">
        {lines.join(" ")} <Link to={{ screen: "system" }}>{t("offlineReminder.see")}</Link>
      </p>
      <button type="button" disabled={dismissing.isBusy} onClick={dismiss}>
        {t("offlineReminder.dismiss")}
      </button>
      {dismissing.problem === undefined ? null : <p role="alert">{dismissing.problem}</p>}
    </aside>
  );
}

/**
 * What the reminder says: what is offline and since when, how many folders' scans wait for the
 * admin, and how many files or folders could not be read (D231 §7, D277).
 */
function reminderLines(t: TFunction, report: OfflineReminderReport, language: string): string[] {
  const { rootCount, diskCount, waitingRootCount, unreadCount, offlineAt: since } = report;
  const lines: string[] = [];
  if (rootCount > 0 || diskCount > 0) {
    const what = offlineWhat(t, rootCount, diskCount);
    lines.push(t("offlineReminder.offlineSince", { what, since: dateText(since ?? 0, language) }));
  }
  if (waitingRootCount > 0) {
    lines.push(
      waitingRootCount === 1
        ? t("offlineReminder.oneWaiting")
        : t("offlineReminder.waitingCount", { count: waitingRootCount }),
    );
  }
  if (unreadCount > 0) {
    lines.push(
      unreadCount === 1
        ? t("offlineReminder.oneUnread")
        : t("offlineReminder.unreadCount", { count: unreadCount }),
    );
  }
  return lines;
}

/** What is offline, in words: so many folders, so many disks, or both. */
function offlineWhat(t: TFunction, rootCount: number, diskCount: number): string {
  const folders =
    rootCount === 1
      ? t("offlineReminder.oneFolder")
      : t("offlineReminder.folders", { count: rootCount });
  const disks =
    diskCount === 1
      ? t("offlineReminder.oneDisk")
      : t("offlineReminder.disks", { count: diskCount });
  if (rootCount === 0) {
    return disks;
  }
  return diskCount > 0 ? t("offlineReminder.foldersAndDisks", { folders, disks }) : folders;
}
