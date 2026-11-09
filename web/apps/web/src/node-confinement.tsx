// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { camelKey } from "./catalogue-key.ts";

/** What a machine said about the sandbox its FFmpeg runs in. */
export type NodeConfinement = Pick<components["schemas"]["SystemNodeBody"], "name" | "confinement">;

/** One mechanism of the sandbox, as a machine reported it. */
type Mechanism = NonNullable<NodeConfinement["confinement"]>[number];

/**
 * The causes that are the kernel's lack, not a fault: too old, built without it, or not switched on
 * at boot. The machine runs without the mechanism, unasked, and the page notes it calmly (D454 §1, §3),
 * as the node's confine.State.IsMissingFromKernel decides.
 */
const missingFromKernel: ReadonlySet<string> = new Set(["kernel_without", "disabled_at_boot"]);

function isMissingFromKernel(mechanism: Mechanism): boolean {
  return !mechanism.isInForce && missingFromKernel.has(mechanism.causeCode ?? "");
}

/**
 * Whether a machine runs FFmpeg in its sandbox, said out loud (D112): a hostile file is what the
 * sandbox is for, so the admin is told when part of it is missing, why, and how to put it back. A part
 * the machine's kernel lacks is a note naming the kernel that brings it, not an alarm (D454 §1).
 */
export function NodeConfinementView(props: { readonly node: NodeConfinement }): JSX.Element {
  const { t } = useTranslation();
  const { node } = props;
  const confinement = node.confinement ?? [];
  const missing = confinement.filter((mechanism) => !mechanism.isInForce);
  if (missing.length === 0 && confinement.length > 0) {
    return <p>{t("nodes.confined", { node: node.name })}</p>;
  }
  const causes = missing.map((mechanism) => (
    <p key={mechanism.mechanism}>
      {mechanism.causeCode === "kernel_without"
        ? t(`nodes.needsKernel.${mechanism.mechanism}`)
        : t(`nodes.cause.${camelKey(mechanism.causeCode ?? "unexplained")}`, {
            mechanism: t(`nodes.mechanism.${mechanism.mechanism}`),
          })}
    </p>
  ));
  if (missing.length > 0 && missing.every(isMissingFromKernel)) {
    return (
      <div>
        <p>{t("nodes.olderKernel", { node: node.name })}</p>
        {causes}
      </div>
    );
  }
  return (
    <div role="alert">
      <p>{t("nodes.unconfined", { node: node.name })}</p>
      {causes}
    </div>
  );
}
