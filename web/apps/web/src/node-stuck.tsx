// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { dateTimeText } from "./date-time-text.ts";
import { i18n } from "./i18n.ts";

/** A piece of work a machine gave up on that waits on a disk not answering. */
export type Stuck = components["schemas"]["StuckBody"];

/**
 * The work a machine gave up on that still waits on a disk not answering (D233 §10): a program the
 * kernel will not stop until the disk answers. It holds no slot, so the machine's other work goes
 * on, and the admin is told which disk to look at.
 */
export function NodeStuckView(props: { readonly stuck: readonly Stuck[] }): JSX.Element | null {
  const { t } = useTranslation();
  if (props.stuck.length === 0) {
    return null;
  }
  return (
    <div role="alert">
      <p>{t("system.stuck")}</p>
      <ul className="system__stuck">
        {props.stuck.map((stuck) => (
          <li key={`${stuck.program}:${String(stuck.since)}:${stuck.paths?.join(":") ?? ""}`}>
            {t("system.stuckWork", {
              program: t(`system.stuckProgram.${stuck.program}`),
              path: stuck.paths?.[0] ?? "",
              since: dateTimeText(stuck.since, i18n.language),
            })}
          </li>
        ))}
      </ul>
    </div>
  );
}
