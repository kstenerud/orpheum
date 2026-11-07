// SPDX-License-Identifier: MPL-2.0

import { leaderOf, type TogetherSnapshot } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { DebugTool } from "./debug-tools.tsx";

/**
 * A Watch Together session's devices as its roster says them, a debug tool (the owner, 2026-10-29):
 * each member's devices by id and role, the one leading the session marked "▶ leading" and the one
 * this page is on "this device", the marks moving as the roster moves the lead (D371 §1); and a line
 * under them: the group's state, the leader by id and member, why it leads as the server says it,
 * and this device's id.
 */
export function TogetherDebug(props: {
  readonly snapshot: TogetherSnapshot | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  // Folded, at the dialog's foot: a tester's, not the host's (the owner, staging, 2026-11-07).
  return (
    <DebugTool>
      {props.snapshot === undefined ? null : (
        <details className="together-debug-fold">
          <summary>{t("together.debug.heading")}</summary>
          <SessionDevices snapshot={props.snapshot} />
        </details>
      )}
    </DebugTool>
  );
}

/** The roster's devices, marked, and the line under them. */
function SessionDevices(props: { readonly snapshot: TogetherSnapshot }): JSX.Element {
  const { t } = useTranslation();
  const { snapshot } = props;
  const leader = leaderOf(snapshot);
  const leaderMember = snapshot.roster.find((member) =>
    member.devices.some((device) => device.id === leader?.id),
  );
  const none = t("together.debug.none");
  return (
    <section className="together-debug" aria-label={t("together.debug.heading")}>
      <ul>
        {snapshot.roster.flatMap((member) =>
          member.devices.map((device) => {
            const marks = [
              device.isLeader === true ? t("together.debug.leading") : undefined,
              device.id === snapshot.deviceId ? t("together.debug.thisDevice") : undefined,
            ].filter((mark) => mark !== undefined);
            return (
              <li key={device.id}>
                {[
                  t("together.debug.device", {
                    name: member.name,
                    id: device.id,
                    role: device.role,
                  }),
                  ...marks,
                ].join(" · ")}
              </li>
            );
          }),
        )}
      </ul>
      <p>
        {t("together.debug.line", {
          state: snapshot.group?.state ?? none,
          leader:
            leader === undefined
              ? none
              : t("together.debug.leader", { id: leader.id, name: leaderMember?.name ?? none }),
          why: leader?.leadsBecause ?? none,
          device: snapshot.deviceId ?? none,
        })}
      </p>
    </section>
  );
}
