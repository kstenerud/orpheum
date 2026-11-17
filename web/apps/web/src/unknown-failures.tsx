// SPDX-License-Identifier: MPL-2.0

import "./unknown-failures.css";

import type { Api, components } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { camelKey } from "./catalogue-key.ts";
import { dateTimeText } from "./date-time-text.ts";
import { i18n } from "./i18n.ts";
import { WarningIcon } from "./icons.tsx";
import { Loaded } from "./loaded.tsx";
import { dataOrThrow } from "./problem.ts";
import type { Resource } from "./use-resource.ts";

/** Every failure of unknown cause, by kind, in full, as GET /unknown-failures answers (D511 §6). */
export type UnknownFailuresReport = components["schemas"]["UnknownFailuresBody"];

type Kind = components["schemas"]["UnknownFailureKindBody"];
type Failure = components["schemas"]["UnknownFailureBody"];

/** Reads every failure of unknown cause. */
export async function readUnknownFailures(api: Api): Promise<UnknownFailuresReport> {
  return dataOrThrow(await api.GET("/unknown-failures"));
}

/** The kinds the report lists; none while it is not read. */
export function reportedKinds(
  report: Resource<UnknownFailuresReport | undefined>,
): readonly Kind[] {
  return report.state === "loaded" ? (report.value?.kinds ?? []) : [];
}

/**
 * The Needs attention page's failures of unknown cause (D509 §4, D511 §6), for an admin: one entry
 * per kind, with its count in the last hour and since the server started, when one last failed and
 * whether its alert is raised, each opening on its failures with the error and the output in full.
 * Nothing while there are none; a read that failed says so in their place.
 */
export function UnknownFailures(props: {
  readonly report: Resource<UnknownFailuresReport | undefined>;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { report } = props;
  if (report.state !== "failed" && reportedKinds(report).length === 0) {
    return null;
  }
  return (
    <section aria-labelledby="unknown-failures-heading" className="unknown-failures">
      <h2 id="unknown-failures-heading">{t("unknownFailures.heading")}</h2>
      <Loaded
        resource={report}
        render={(value) => (
          <>
            <p>{t("unknownFailures.about")}</p>
            <ul>
              {(value?.kinds ?? []).map((kind) => (
                <KindEntry key={kind.kind} kind={kind} />
              ))}
            </ul>
          </>
        )}
      />
    </section>
  );
}

/** One kind: its counts and whether its alert is raised, opening on its failures. */
function KindEntry(props: { readonly kind: Kind }): JSX.Element {
  const { t } = useTranslation();
  const { kind } = props;
  return (
    <li>
      <details>
        <summary>
          {kind.isRaised ? (
            <>
              <WarningIcon />{" "}
            </>
          ) : null}
          <strong>{t(`jobs.kind.${camelKey(kind.kind)}`, { defaultValue: kind.kind })}</strong>{" "}
          {t("unknownFailures.counts", {
            lastHour: kind.lastHour,
            sinceStart: kind.sinceStart,
            at: dateTimeText(kind.lastAt, i18n.language),
          })}
          {kind.isRaised ? ` ${t("unknownFailures.raised")}` : null}
        </summary>
        <ul>
          {(kind.failures ?? []).map((failure) => (
            <FailureEntry key={failure.subject} failure={failure} />
          ))}
        </ul>
      </details>
    </li>
  );
}

/** One failure, with everything it said: the error and the output whole, wrapped, never cut off. */
function FailureEntry(props: { readonly failure: Failure }): JSX.Element {
  const { t } = useTranslation();
  const { failure } = props;
  return (
    <li className="unknown-failures__failure">
      <p>
        <strong>{failure.subject}</strong>{" "}
        {t("unknownFailures.times", {
          count: failure.count,
          first: dateTimeText(failure.firstAt, i18n.language),
          last: dateTimeText(failure.lastAt, i18n.language),
          version: failure.version,
        })}
        {failure.isThisStart ? null : ` ${t("unknownFailures.earlierStart")}`}
      </p>
      <p>{t("unknownFailures.error")}</p>
      <pre>{failure.error}</pre>
      <p>{t("unknownFailures.output")}</p>
      {failure.output === "" ? <p>{t("unknownFailures.noOutput")}</p> : <pre>{failure.output}</pre>}
    </li>
  );
}
