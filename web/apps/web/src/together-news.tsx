// SPDX-License-Identifier: MPL-2.0

import type { TogetherSnapshot } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

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
    </div>
  );
}
