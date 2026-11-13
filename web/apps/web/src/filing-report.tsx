// SPDX-License-Identifier: MPL-2.0

import { allPages, type components } from "@orpheum/sdk";
import { use, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { dateTimeText } from "./date-time-text.ts";
import { i18n } from "./i18n.ts";
import { idempotent } from "./idempotent.ts";
import { useBackgroundChangesPoller } from "./polling.ts";
import { dataOrThrow, problemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { copyToClipboard } from "./clipboard.ts";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";
import { IsAdminContext } from "./warning-sign.tsx";

type Report = components["schemas"]["FilingReportBody"];
type ReportEntry = components["schemas"]["FilingReportEntryBody"];

/**
 * An admin's "Report this", in "Identified incorrectly" on a film's, episode's or series' page
 * (match-correction.tsx; D306 §3): the server
 * writes a Markdown report of how its files were read and filed to its own disk, and shows it here,
 * to copy, edit and post as an issue. Nothing leaves the server by itself. Nothing for anyone else.
 */
export function FilingReportButton(props: { readonly itemId: number }): JSX.Element | null {
  const { t } = useTranslation();
  const { api } = useServices();
  const isAdmin = use(IsAdminContext);
  const [report, setReport] = useState<Report | undefined>(undefined);
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  if (!isAdmin) {
    return null;
  }
  const writeReport = (): void => {
    runRequest(async (idempotencyKey) => {
      setReport(
        dataOrThrow(
          await api.POST("/items/{id}/filing-report", {
            params: { path: { id: props.itemId } },
            ...idempotent(idempotencyKey),
          }),
        ),
      );
    });
  };
  return (
    <div className="filing-report">
      <button type="button" disabled={isBusy} onClick={writeReport}>
        {t("filingReport.write")}
      </button>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
      {report === undefined ? null : <ReportText report={report} />}
    </div>
  );
}

/** A report's text, where it is on the server's disk, and a way to copy it. */
export function ReportText(props: { readonly report: Report }): JSX.Element {
  const { t } = useTranslation();
  const textRef = useRef<HTMLTextAreaElement>(null);
  const [isCopied, setIsCopied] = useState(false);
  const copy = (): void => {
    const area = textRef.current;
    // Where the clipboard is not, as at the owner's plain-HTTP LAN address, the text is selected to copy.
    copyToClipboard(props.report.markdown).then(
      () => {
        setIsCopied(true);
      },
      () => area?.select(),
    );
  };
  return (
    <div className="filing-report__text">
      {props.report.path === undefined ? null : (
        <p>{t("filingReport.savedAt", { path: props.report.path })}</p>
      )}
      <textarea
        ref={textRef}
        readOnly
        rows={12}
        value={props.report.markdown}
        aria-label={t("filingReport.text")}
      />
      <button type="button" onClick={copy}>
        {t("filingReport.copy")}
      </button>
      {isCopied ? <output>{t("filingReport.copied")}</output> : null}
    </div>
  );
}

/** The System page's reports of works filed wrongly, newest first, each read when opened. */
export function SystemFilingReports(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [reports] = useResource(
    "filing-reports",
    async () =>
      await allPages(async (query) => {
        const page = dataOrThrow(await api.GET("/filing-reports", { params: { query } }));
        return { entries: page.reports ?? [], nextCursor: page.nextCursor };
      }),
    { poller: useBackgroundChangesPoller() },
  );
  return (
    <section aria-labelledby="system-filing-reports">
      <h2 id="system-filing-reports">{t("filingReport.heading")}</h2>
      <p>{t("filingReport.what")}</p>
      {reports.state === "loaded" ? <ReportList reports={reports.value} /> : null}
      {reports.state === "failed" ? <p role="alert">{problemText(t, reports.cause)}</p> : null}
    </section>
  );
}

/** The reports, newest first, or that there is none. */
function ReportList(props: { readonly reports: readonly ReportEntry[] }): JSX.Element {
  const { t } = useTranslation();
  return props.reports.length === 0 ? (
    <p>{t("filingReport.none")}</p>
  ) : (
    <ul>
      {props.reports.map((report) => (
        <ReportEntryView key={report.name} entry={report} />
      ))}
    </ul>
  );
}

/** One report in the System page's list, read from the server when opened. */
function ReportEntryView(props: { readonly entry: ReportEntry }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [report, setReport] = useState<Report | undefined>(undefined);
  const { problem, runRequest } = useRequest((cause) => problemText(t, cause));
  const open = (): void => {
    if (report !== undefined) {
      return;
    }
    runRequest(async () => {
      setReport(
        dataOrThrow(
          await api.GET("/filing-reports/{name}", { params: { path: { name: props.entry.name } } }),
        ),
      );
    });
  };
  return (
    <li>
      <details onToggle={open}>
        <summary>
          {props.entry.name} · {dateTimeText(props.entry.writtenAt, i18n.language)}
        </summary>
        {problem === undefined ? null : <p role="alert">{problem}</p>}
        {report === undefined ? null : <ReportText report={report} />}
      </details>
    </li>
  );
}
