// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { WarningSign } from "./warning-sign.tsx";

type Root = components["schemas"]["RootBody"];

/**
 * The warning sign beside a library folder with a problem (D278): its scan could not read it, its
 * removals wait for the admin and why (D277), or a disk inside it is missing. Nothing for a folder
 * that is well.
 */
export function RootWarning(props: { readonly root: Root }): JSX.Element | null {
  const { t } = useTranslation();
  const { root } = props;
  const missing = root.missingMountPaths ?? [];
  if (root.lastScanOutcome === "withheld") {
    return (
      <WarningSign
        summary={t("warning.folderWaiting")}
        forAdmin={`${root.withheldReason ?? ""} ${t("libraries.approveRemovals")}.`}
      />
    );
  }
  if (root.lastScanOutcome === "unreadable") {
    return <WarningSign summary={t("warning.folderOffline")} forAdmin={t("warning.goToSystem")} />;
  }
  if (missing.length > 0) {
    return (
      <WarningSign
        summary={t("warning.diskMissing")}
        forAdmin={`${missing.join(", ")}. ${t("warning.goToSystem")}`}
      />
    );
  }
  return null;
}
