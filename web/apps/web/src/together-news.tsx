// SPDX-License-Identifier: MPL-2.0

import type { RosterMember, TogetherSnapshot } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { rateText } from "./rate-text.ts";
import { groupEventText } from "./together-events.ts";

/**
 * What is happening in the session, for everyone in it: a drop as a state ("reconnecting"), the
 * group's last change as a named event, and who is here, by the names they chose (D188,
 * watch-together.md: What each person sees). Never the host's notes.
 */
export function TogetherNews(props: {
  readonly snapshot: TogetherSnapshot | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { snapshot } = props;
  if (snapshot === undefined) {
    return null;
  }
  const event =
    snapshot.group === undefined ? undefined : groupEventText(t, snapshot.group, snapshot.roster);
  return (
    <div className="together-news">
      {snapshot.status === "connecting" || snapshot.status === "reconnecting" ? (
        <output>{t(`together.${snapshot.status}`)}</output>
      ) : null}
      {event === undefined ? null : <output>{event}</output>}
      {snapshot.roster.length === 0 ? null : (
        <p>
          {t("together.here", {
            names: snapshot.roster.map((member) =>
              member.devices.length === 0 ? t("together.away", { name: member.name }) : member.name,
            ),
          })}
        </p>
      )}
      <StatusLines roster={snapshot.roster} />
      {snapshot.roster.flatMap((member) => {
        const slow = member.devices.find(
          (device) => device.cannotKeepUp !== undefined,
        )?.cannotKeepUp;
        if (slow === undefined) {
          return [];
        }
        const lowest = rateText(slow.lowestKbps);
        return [
          <output key={`slow-${String(member.id)}`}>
            {slow.linkKbps === undefined
              ? t("together.cannotKeepUpUnmeasured", { name: member.name, lowest })
              : t("together.cannotKeepUp", {
                  name: member.name,
                  link: rateText(slow.linkKbps),
                  lowest,
                })}
          </output>,
        ];
      })}
    </div>
  );
}

/**
 * Where each person's player stands, a line each, as the server reads it: what their page waits on,
 * their speakers or a tap, loading, catching up, or watching; nothing of a voice alone (the owner's
 * Watch Together session, 2026-10-18: a guest still choosing her speakers showed as nothing at
 * all, and the host could not tell why her film did not play).
 */
function StatusLines(props: { readonly roster: readonly RosterMember[] }): JSX.Element | null {
  const { t } = useTranslation();
  const lines = props.roster.flatMap((member) => statusText(t, member));
  return lines.length === 0 ? null : (
    <ul>
      {lines.map((line) => (
        <li key={line}>{line}</li>
      ))}
    </ul>
  );
}

/** A member's status in words, by their first device with one; none for a member without. */
function statusText(t: TFunction, member: RosterMember): string[] {
  const status = member.devices.find((device) => device.status !== undefined)?.status;
  return status === undefined ? [] : [t(`together.status.${status}`, { name: member.name })];
}
