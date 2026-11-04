// SPDX-License-Identifier: MPL-2.0

import "./system.css";

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { dateTimeText } from "./date-time-text.ts";
import { i18n } from "./i18n.ts";
import { idempotent } from "./idempotent.ts";
import { dataOrThrow, problemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";

type Report = components["schemas"]["ReachabilityOutputBody"];
type Result = components["schemas"]["ReachabilityResult"];

/**
 * Whether the server is reached from outside (D243 §7), checked when the admin asks: its public
 * name, HTTPS through it to this server, and the voice relay's port, each hop that failed named
 * with what to look at (remote-access.md, "Am I reachable?").
 */
export function SystemReachability(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [report, setReport] = useState<Report | undefined>(undefined);
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  const check = (): void => {
    runRequest(async (idempotencyKey) => {
      setReport(dataOrThrow(await api.POST("/system/reachability", idempotent(idempotencyKey))));
    });
  };
  return (
    <section aria-labelledby="system-reachability">
      <div className="system__heading-row">
        <h2 id="system-reachability">{t("system.reachability.heading")}</h2>
        <button type="button" disabled={isBusy} onClick={check}>
          {t("system.reachability.check")}
        </button>
      </div>
      <p>{t("system.reachability.what")}</p>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
      {report === undefined ? null : <ReachabilityReport report={report} />}
    </section>
  );
}

/** One check: where the name points, and each hop's outcome. */
function ReachabilityReport(props: { readonly report: Report }): JSX.Element {
  const { t } = useTranslation();
  const { report } = props;
  const addresses = report.addresses ?? [];
  return (
    <output>
      <p>
        {t("system.reachability.checkedAt", { at: dateTimeText(report.checkedAt, i18n.language) })}
      </p>
      {addresses.length === 0 ? null : (
        <p>
          {t("system.reachability.points", { name: report.name, addresses: addresses.join(", ") })}
        </p>
      )}
      <ul>
        <HopLine
          hop={t("system.reachability.hop.https", { name: report.name })}
          result={report.https}
        />
        <HopLine hop={t("system.reachability.hop.relayUdp")} result={report.relayUdp} />
        <HopLine hop={t("system.reachability.hop.relayTcp")} result={report.relayTcp} />
      </ul>
    </output>
  );
}

/** One hop's outcome in words, and the failing layer's own, when it failed. */
function HopLine(props: { readonly hop: string; readonly result: Result }): JSX.Element {
  const { t } = useTranslation();
  const { hop, result } = props;
  return (
    <li>
      {t(`system.reachability.outcome.${result.outcome}`, { hop })}
      {result.detail === undefined ? null : (
        <> {t("system.reachability.detail", { detail: result.detail })}</>
      )}
    </li>
  );
}
