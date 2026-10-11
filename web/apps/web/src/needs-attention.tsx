// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { Link } from "./link.tsx";
import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { useResource } from "./use-resource.ts";
import { WarningIcon } from "./warning-sign.tsx";

/**
 * Everything that needs the admin, in one place, which the status tray's warning sign leads to
 * (D280). A placeholder until it is designed: it counts what the reminder counts and leads to the
 * pages that have each, the System page and the Libraries.
 */
export function NeedsAttention(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [counts] = useResource("needs-attention", async () =>
    dataOrThrow(await api.GET("/offline-reminder")),
  );
  return (
    <section aria-labelledby="needs-attention-heading">
      <h1 id="needs-attention-heading">{t("needsAttention.heading")}</h1>
      <p>{t("needsAttention.placeholder")}</p>
      <Loaded
        resource={counts}
        render={(value) => {
          const lines = [
            { count: value.rootCount, key: "needsAttention.offlineFolders" },
            { count: value.diskCount, key: "needsAttention.missingDisks" },
            { count: value.waitingRootCount, key: "needsAttention.waitingFolders" },
            { count: value.unreadCount, key: "needsAttention.unreadPaths" },
          ].filter((line) => line.count > 0);
          return lines.length === 0 ? (
            <p>{t("needsAttention.nothing")}</p>
          ) : (
            <ul>
              {lines.map((line) => (
                <li key={line.key}>
                  <WarningIcon /> {t(line.key, { count: line.count })}
                </li>
              ))}
            </ul>
          );
        }}
      />
      <p>
        <Link to={{ screen: "system" }}>{t("needsAttention.toSystem")}</Link> ·{" "}
        <Link to={{ screen: "libraries" }}>{t("needsAttention.toLibraries")}</Link>
      </p>
    </section>
  );
}
