// SPDX-License-Identifier: MPL-2.0

import "./what-happened.css";

import { stepsSoFar } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

/**
 * What happened when a device could not reach its server, for a tester to send whoever runs it
 * (discovery-core.md K16, the protocol's §3.6): the server, the addresses tried, what failed and
 * the browser. It names no person and holds no secret: a server's id and its published names are
 * what any link of it carries. The steps the page took come last (stepsText).
 */
export function whatHappened(
  reached: {
    readonly coreId: string | undefined;
    readonly addressesTried: readonly string[];
    readonly problem: unknown;
  },
  userAgent: string,
  steps: readonly string[] = stepsSoFar(),
): string {
  const failed = failedText(reached.problem);
  return [
    "Orpheum could not reach its server.",
    `Server: ${reached.coreId ?? "not found"}`,
    `Addresses tried: ${reached.addressesTried.length === 0 ? "none" : reached.addressesTried.join(", ")}`,
    `What failed: ${failed}`,
    `Browser: ${userAgent}`,
    ...stepsText(steps),
  ].join("\n");
}

/**
 * The lines telling the steps a page took to reach its core (the SDK's noteStep), each indented
 * under a heading, or none where it took none: which step it reached is what says where a
 * connection the core held was lost (plan item 15, 2026-11-02).
 */
export function stepsText(steps: readonly string[]): readonly string[] {
  return steps.length === 0 ? [] : ["Steps:", ...steps.map((step) => `  ${step}`)];
}

/**
 * What failed, as text: the problem's message, each alternative an AggregateError holds and each
 * cause behind it, a few deep. An AggregateError's own message is often empty, and said alone it
 * hid a core's refusal of the device's key behind "What failed:" and nothing (the owner's Fire TV,
 * 2026-10-28).
 */
function failedText(problem: unknown, depth = 0): string {
  if (!(problem instanceof Error)) {
    return String(problem);
  }
  const parts = problem.message === "" ? [] : [problem.message];
  if (depth < 4 && problem instanceof AggregateError) {
    const alternatives: readonly unknown[] = Array.from<unknown>(problem.errors);
    parts.push(...alternatives.map((each) => failedText(each, depth + 1)));
  }
  if (depth < 4 && problem.cause !== undefined) {
    parts.push(`because ${failedText(problem.cause, depth + 1)}`);
  }
  return parts.join("; ");
}

/**
 * The text of what happened, to copy and send, and shown to read: a copy the browser refuses, as
 * one that may not use the clipboard does, still leaves it there to select.
 */
export function CopyWhatHappened(props: {
  readonly text: string | undefined;
  readonly copy: (text: string) => Promise<void>;
}): JSX.Element | null {
  const { t } = useTranslation();
  const [copied, setCopied] = useState<"copied" | "refused" | undefined>(undefined);
  const { text } = props;
  if (text === undefined) {
    return null;
  }
  return (
    <div className="what-happened">
      <button
        type="button"
        className="quiet"
        onClick={() => {
          props.copy(text).then(
            () => {
              setCopied("copied");
            },
            () => {
              setCopied("refused");
            },
          );
        }}
      >
        {t("whatHappened.copy")}
      </button>
      {copied === undefined ? null : <output>{t(`whatHappened.${copied}`)}</output>}
      <details>
        <summary>{t("whatHappened.show")}</summary>
        <pre>{text}</pre>
      </details>
    </div>
  );
}
