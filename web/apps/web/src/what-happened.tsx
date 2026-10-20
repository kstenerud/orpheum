// SPDX-License-Identifier: MPL-2.0

import "./what-happened.css";

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

/**
 * What happened when a device could not reach its server, for a tester to send whoever runs it
 * (discovery-core.md K16, the protocol's §3.6): the server, the addresses tried, what failed and
 * the browser. It names no person and holds no secret: a server's id and its published names are
 * what any link of it carries.
 */
export function whatHappened(
  reached: {
    readonly coreId: string | undefined;
    readonly addressesTried: readonly string[];
    readonly problem: unknown;
  },
  userAgent: string,
): string {
  const failed =
    reached.problem instanceof Error ? reached.problem.message : String(reached.problem);
  return [
    "Orpheum could not reach its server.",
    `Server: ${reached.coreId ?? "not found"}`,
    `Addresses tried: ${reached.addressesTried.length === 0 ? "none" : reached.addressesTried.join(", ")}`,
    `What failed: ${failed}`,
    `Browser: ${userAgent}`,
  ].join("\n");
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
