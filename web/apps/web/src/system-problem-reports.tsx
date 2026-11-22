// SPDX-License-Identifier: MPL-2.0

import "./system-problem-reports.css";
import "./system-words.ts";

import type { components } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { ConfirmButton } from "./confirm-button.tsx";
import { CopyButton } from "./copy-button.tsx";
import { dateTimeText } from "./date-time-text.ts";
import { setEveryonesSoundOffset } from "./file-sound-offset.tsx";
import { i18n } from "./i18n.ts";
import { Loaded } from "./loaded.tsx";
import { SystemSectionsContext, useSectionGiven } from "./page-sections.ts";
import { dataOrThrow, problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { StatusNote } from "./status-note.tsx";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";

type Summary = components["schemas"]["ProblemReportSummaryBody"];
type SuggestedOffset = components["schemas"]["ProblemReportSoundOffsetBody"];

/** A form a report downloads in: shareable hides every name, full holds them (D524 §4). */
type Form = "shareable" | "full";

/** A report downloaded: the form chosen, the whole of it and its summary for an issue's body. */
interface Downloaded {
  readonly form: Form;
  readonly whole: string;
  readonly summary: string;
}

/**
 * The System page's problem reports (D524 §2, §4): what the household's people reported from the
 * account menu or the player's ⁝, newest first, each with who, when, from which version and their
 * words, kept 60 days. Each downloads shareable or full, the admin choosing which every time, as
 * no form is the default, and is deleted at a confirmed press.
 */
export function SystemProblemReports(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [reports, reload] = useResource(
    "problem-reports",
    async () => dataOrThrow(await api.GET("/problem-reports")),
    {
      given: useSectionGiven(SystemSectionsContext, (sections) => ({
        value: sections.problemReports,
      })),
    },
  );
  return (
    <section aria-labelledby="system-problem-reports">
      <h2 id="system-problem-reports">{t("system.problemReports.heading")}</h2>
      <p>{t("system.problemReports.what")}</p>
      <Loaded
        resource={reports}
        render={(value) => <ReportList reports={value.reports ?? []} onDeleted={reload} />}
      />
    </section>
  );
}

/** The reports, newest first, or that there is none. */
function ReportList(props: {
  readonly reports: readonly Summary[];
  readonly onDeleted: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  return props.reports.length === 0 ? (
    <p>{t("system.problemReports.none")}</p>
  ) : (
    <ul className="system-problem-reports">
      {props.reports.map((report) => (
        <ReportEntry key={report.id} report={report} onDeleted={props.onDeleted} />
      ))}
    </ul>
  );
}

/** One report: who made it when, from which version, their words, its downloads and Delete. */
function ReportEntry(props: {
  readonly report: Summary;
  readonly onDeleted: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { report } = props;
  const deleting = useRequest((cause) => problemText(t, cause));
  const made = t("system.problemReports.made", {
    who: report.userName ?? t("system.problemReports.someoneGone"),
    at: dateTimeText(report.madeAt, i18n.language),
    version: report.version,
  });
  return (
    <li>
      <details>
        <summary>{made}</summary>
        <blockquote className="system-problem-reports__words">{report.words}</blockquote>
        {report.soundOffset === undefined ? null : (
          <SuggestedSoundOffset suggested={report.soundOffset} />
        )}
        <ReportDownload reportId={report.id} />
        {deleting.problem === undefined ? null : (
          <StatusNote told={{ kind: "problem", text: deleting.problem }} />
        )}
        <ConfirmButton
          label={t("system.problemReports.delete")}
          question={t("system.problemReports.deleteQuestion")}
          consequence={t("system.problemReports.deleteConsequence")}
          confirmLabel={t("system.problemReports.deleteConfirm")}
          isDisabled={deleting.isBusy}
          onConfirm={() => {
            deleting.runRequest(async () => {
              throwIfRefused(
                await api.DELETE("/problem-reports/{id}", {
                  params: { path: { id: report.id } },
                }),
              );
              props.onDeleted();
            });
          }}
        />
      </details>
    </li>
  );
}

/**
 * The file's sound offset a report suggests, from the Sound menu's Tell the admin (D537 §5): what it
 * suggests, and Apply to everyone, which makes it the file's offset for every person (D537 §2)
 * through the one call the Sound menu's Make it everyone's makes (setEveryonesSoundOffset). It
 * replaces the file's offset for everyone, with nothing to undo it by, so it asks first, a plain
 * confirm as for what is disruptive but harmless, its focus on Cancel (ui.md rule 8); once applied
 * it says so in its place.
 */
function SuggestedSoundOffset(props: { readonly suggested: SuggestedOffset }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { fileId, soundOffsetMs } = props.suggested;
  const [isApplied, setIsApplied] = useState(false);
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  return (
    <div className="system-problem-reports__sound-offset">
      <p>{suggestedOffsetText(t, props.suggested)}</p>
      {problem === undefined ? null : <StatusNote told={{ kind: "problem", text: problem }} />}
      {isApplied ? (
        <StatusNote
          told={{ kind: "success", text: t("system.problemReports.soundOffset.applied") }}
        />
      ) : (
        <ConfirmButton
          label={t("system.problemReports.soundOffset.apply")}
          question={t("system.problemReports.soundOffset.applyQuestion")}
          consequence={t("system.problemReports.soundOffset.applyConsequence", {
            id: fileId,
          })}
          confirmLabel={t("system.problemReports.soundOffset.applyConfirm")}
          isHarmless
          isDisabled={isBusy}
          onConfirm={() => {
            runRequest(async () => {
              await setEveryonesSoundOffset(api, fileId, soundOffsetMs);
              setIsApplied(true);
            });
          }}
        />
      )}
    </div>
  );
}

/** What a report's suggested offset says: the file, and how far its sound moves, or in step. */
function suggestedOffsetText(t: TFunction, suggested: SuggestedOffset): string {
  const { fileId: id, soundOffsetMs: ms } = suggested;
  if (ms === 0) {
    return t("system.problemReports.soundOffset.inStep", { id });
  }
  return ms > 0
    ? t("system.problemReports.soundOffset.later", { id, ms })
    : t("system.problemReports.soundOffset.earlier", { id, ms: -ms });
}

/**
 * A report's downloads: Shareable and Full, side by side, neither first in weight, so each download
 * is a choice (D524 §4), and whether the full form names the other library's videos too (D349 §10).
 * Downloaded, it is saved as a Markdown file, and its summary, which fits an issue's body, copied.
 */
function ReportDownload(props: { readonly reportId: number }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [shouldIncludeOther, setShouldIncludeOther] = useState(false);
  const [downloaded, setDownloaded] = useState<Downloaded | undefined>(undefined);
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  const download = (form: Form): void => {
    runRequest(async () => {
      const got = dataOrThrow(
        await api.GET("/problem-reports/{id}", {
          params: {
            path: { id: props.reportId },
            query: {
              form,
              ...(form === "full" && shouldIncludeOther ? { includeOther: true } : {}),
            },
          },
        }),
      );
      setDownloaded({ form, whole: got.whole, summary: got.summary });
    });
  };
  return (
    <div className="system-problem-reports__download">
      <p>{t("system.problemReports.choose")}</p>
      <FormButtons isBusy={isBusy} onChoose={download} />
      <label>
        <input
          type="checkbox"
          checked={shouldIncludeOther}
          onChange={(event) => {
            setShouldIncludeOther(event.target.checked);
          }}
        />{" "}
        {t("system.problemReports.includeOther")}
      </label>
      {problem === undefined ? null : <StatusNote told={{ kind: "problem", text: problem }} />}
      {downloaded === undefined ? null : (
        <DownloadedReport reportId={props.reportId} downloaded={downloaded} />
      )}
    </div>
  );
}

/** Shareable and Full, side by side and alike, so neither is the one to press. */
function FormButtons(props: {
  readonly isBusy: boolean;
  readonly onChoose: (form: Form) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <div className="system-problem-reports__forms">
      {(["shareable", "full"] as const).map((form) => (
        <button
          key={form}
          type="button"
          disabled={props.isBusy}
          onClick={() => {
            props.onChoose(form);
          }}
        >
          {form === "full" ? t("system.problemReports.full") : t("system.problemReports.shareable")}
        </button>
      ))}
    </div>
  );
}

/** A report downloaded: which form it is, a button that saves it whole, and its summary to copy. */
function DownloadedReport(props: {
  readonly reportId: number;
  readonly downloaded: Downloaded;
}): JSX.Element {
  const { t } = useTranslation();
  const { form, whole, summary } = props.downloaded;
  const summaryRef = useRef<HTMLTextAreaElement>(null);
  const fileName = `problem-report-${String(props.reportId)}-${form}.md`;
  return (
    <div className="system-problem-reports__downloaded">
      <p>
        {form === "full"
          ? t("system.problemReports.fullGot")
          : t("system.problemReports.shareableGot")}
      </p>
      <button
        type="button"
        onClick={() => {
          saveAsFile(whole, fileName);
        }}
      >
        {t("system.problemReports.save", { name: fileName })}
      </button>
      <textarea
        ref={summaryRef}
        readOnly
        rows={10}
        value={summary}
        aria-label={t("system.problemReports.summary")}
      />
      <CopyButton
        text={summary}
        label={t("system.problemReports.copySummary")}
        copiedNote={t("system.problemReports.copied")}
        // Where the clipboard is not, as at a plain-HTTP LAN address, the summary is selected to copy.
        onRefused={() => summaryRef.current?.select()}
      />
    </div>
  );
}

/**
 * Has the browser save text as a Markdown file named name: an address made for it, a link to it
 * pressed, and the address let go once the browser has taken it.
 */
function saveAsFile(text: string, name: string): void {
  const href = URL.createObjectURL(new Blob([text], { type: "text/markdown" }));
  const link = document.createElement("a");
  link.href = href;
  link.download = name;
  link.click();
  setTimeout(() => {
    URL.revokeObjectURL(href);
  }, 0);
}
