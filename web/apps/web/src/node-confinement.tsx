// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { camelKey } from "./catalogue-key.ts";

/** What a machine said about the sandbox its FFmpeg runs in. */
export type NodeConfinement = Pick<components["schemas"]["SystemNodeBody"], "name" | "confinement">;

/**
 * Whether a machine runs FFmpeg in its sandbox, said out loud (D112): a hostile file is what the
 * sandbox is for, so the admin is told when part of it is missing, why, and how to put it back.
 */
export function NodeConfinementView(props: { readonly node: NodeConfinement }): JSX.Element {
  const { t } = useTranslation();
  const { node } = props;
  const confinement = node.confinement ?? [];
  const missing = confinement.filter((mechanism) => !mechanism.isInForce);
  if (missing.length === 0 && confinement.length > 0) {
    return <p>{t("nodes.confined", { node: node.name })}</p>;
  }
  return (
    <div role="alert">
      <p>{t("nodes.unconfined", { node: node.name })}</p>
      {missing.map((mechanism) => (
        <p key={mechanism.mechanism}>
          {t(`nodes.cause.${camelKey(mechanism.causeCode ?? "unexplained")}`, {
            mechanism: t(`nodes.mechanism.${mechanism.mechanism}`),
          })}
        </p>
      ))}
    </div>
  );
}
