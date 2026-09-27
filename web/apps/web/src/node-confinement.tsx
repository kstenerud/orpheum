// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { camelKey } from "./catalogue-key.ts";
import { dataOrThrow } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { useResource } from "./use-resource.ts";

/** One machine that runs FFmpeg, as the server knows it. */
export type NodeReport = components["schemas"]["NodeBody"];

/**
 * Whether each connected machine runs FFmpeg in its sandbox, said out loud (D112): a hostile
 * file is what the sandbox is for, so the admin is told when part of it is missing, why, and
 * how to put it back.
 */
export function NodeConfinement(): JSX.Element {
  const { api } = useServices();
  const [nodes] = useResource(
    "nodes",
    async () => dataOrThrow(await api.GET("/nodes")).nodes ?? [],
  );
  return <Loaded resource={nodes} render={(value) => <NodeConfinementView nodes={value} />} />;
}

/** The machines' sandboxes, for those connected. */
export function NodeConfinementView(props: { readonly nodes: readonly NodeReport[] }): JSX.Element {
  const { t } = useTranslation();
  const connected = props.nodes.filter((node) => node.isConnected);
  return (
    <section aria-labelledby="machines-heading">
      <h2 id="machines-heading">{t("nodes.heading")}</h2>
      {connected.length === 0 ? <p>{t("nodes.noneConnected")}</p> : null}
      {connected.map((node) => {
        const missing = (node.confinement ?? []).filter((mechanism) => !mechanism.isInForce);
        if (missing.length === 0 && (node.confinement ?? []).length > 0) {
          return <p key={node.name}>{t("nodes.confined", { node: node.name })}</p>;
        }
        return (
          <div key={node.name} role="alert">
            <p>{t("nodes.unconfined", { node: node.name })}</p>
            {missing.map((mechanism) => (
              <p key={mechanism.mechanism}>
                {t(`nodes.cause.${camelKey(mechanism.cause ?? "unexplained")}`, {
                  mechanism: t(`nodes.mechanism.${mechanism.mechanism}`),
                })}
              </p>
            ))}
          </div>
        );
      })}
    </section>
  );
}
