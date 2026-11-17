// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { camelKey } from "./catalogue-key.ts";
import { Link } from "./link.tsx";
import { shownAndEveryPoller } from "./polling.ts";
import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";

/** The kinds whose failures of unknown cause raise the alert, as GET /unknown-failures/alerts answers. */
type Alert = components["schemas"]["UnknownFailureAlertBody"];

/** How often a shown page reads the alerts again: five minutes, the alert being an hour's count. */
export const alertsReadEveryMs = 5 * 60 * 1000;

/** The alerts' poller: when the page is shown again, and every five minutes while it shows. */
const alertsPoller = shownAndEveryPoller(alertsReadEveryMs);

/**
 * The banner above every page while a kind of work's failures of unknown cause passed the alert's
 * count in the last hour, for its admins alone (D509 §3): the kinds, each with its count in the last
 * hour, and the way to the Needs attention page, which lists them in full (D511 §6). It is not
 * dismissed: it goes once the server clears the alert, after an hour under its count. Everyone else
 * sees nothing, and nothing is asked of the server for them (D511 §7).
 */
export function UnknownFailuresNote(props: { readonly isAdmin: boolean }): JSX.Element | null {
  return props.isAdmin ? <AdminsNote /> : null;
}

/** The banner for an admin, read when the page shows and again on the poller's schedule. */
function AdminsNote(): JSX.Element | null {
  const { t } = useTranslation();
  const { api } = useServices();
  const [alerts] = useResource(
    "unknown-failure-alerts",
    async (signal) =>
      dataOrThrow(await api.GET("/unknown-failures/alerts", { signal: signal ?? null })).alerts ??
      [],
    { poller: alertsPoller },
  );
  if (alerts.state !== "loaded" || alerts.value.length === 0) {
    return null;
  }
  const kinds = alerts.value.map((alert: Alert) =>
    t("unknownFailures.kindInLastHour", {
      kind: t(`jobs.kind.${camelKey(alert.kind)}`, { defaultValue: alert.kind }),
      count: alert.lastHour,
    }),
  );
  return (
    <output className="app-shell__note app-shell__note--unknown-failures">
      {t("unknownFailures.banner", { kinds: kinds.join("; ") })}{" "}
      <Link to={{ screen: "needs-attention" }}>{t("unknownFailures.seeThem")}</Link>
    </output>
  );
}
