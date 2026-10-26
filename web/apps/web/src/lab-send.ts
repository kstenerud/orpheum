// SPDX-License-Identifier: MPL-2.0

import type { Api, components } from "@orpheum/sdk";
import { useCallback, useState } from "react";

import { idempotent, newIdempotencyKey } from "./idempotent.ts";
import { dataOrThrow } from "./problem.ts";

/** Which lab page measured a result, as the lab endpoint names them. */
export type LabProbe = components["schemas"]["LabResultInputBody"]["probe"];

/** Sends a lab page's result to the server's log; answers the name the log keeps it by. */
export async function sendLabResult(
  api: Api,
  probe: LabProbe,
  result: Readonly<Record<string, unknown>>,
): Promise<string> {
  const sent = await api.POST("/lab-results", {
    body: { probe, result: { ...result } },
    ...idempotent(newIdempotencyKey()),
  });
  return dataOrThrow(sent).id;
}

/** Where a page's sending stands: not sent, on its way, sent under a name, or failed. */
export type Sending =
  | { readonly state: "unsent" }
  | { readonly state: "sending" }
  | { readonly state: "sent"; readonly id: string }
  | { readonly state: "failed" };

/** A page's sending of its results, and its state. */
export function useLabSending(
  api: Api,
  probe: LabProbe,
): {
  readonly sending: Sending;
  readonly send: (result: Readonly<Record<string, unknown>>) => void;
} {
  const [sending, setSending] = useState<Sending>({ state: "unsent" });
  const send = useCallback(
    (result: Readonly<Record<string, unknown>>) => {
      setSending({ state: "sending" });
      sendLabResult(api, probe, result).then(
        (id) => {
          setSending({ state: "sent", id });
        },
        () => {
          setSending({ state: "failed" });
        },
      );
    },
    [api, probe],
  );
  return { sending, send };
}
