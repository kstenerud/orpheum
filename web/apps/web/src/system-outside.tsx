// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { dateTimeText } from "./date-time-text.ts";
import { i18n } from "./i18n.ts";
import { waitingText } from "./jobs.tsx";
import { tenthsText } from "./number-text.ts";

type Destination = components["schemas"]["DestinationBody"];
type Provider = components["schemas"]["ProviderBody"];

/**
 * What work needs that is down now, each with why, since when and when it is looked at next
 * (D231 §12): an outside host, a machine, a folder, the server's own storage. Nothing while all is up.
 */
export function DownDestinations(props: {
  readonly destinations: readonly Destination[];
}): JSX.Element | null {
  const { t } = useTranslation();
  if (props.destinations.length === 0) {
    return null;
  }
  return (
    <section aria-labelledby="system-down">
      <h2 id="system-down">{t("system.down")}</h2>
      <ul>
        {props.destinations.map((destination) => (
          <li key={destination.destination}>{waitingText(t, destination, i18n.language)}</li>
        ))}
      </ul>
    </section>
  );
}

/**
 * Each outside provider (D231 §5): the rate we keep to it and background work's share, whether its
 * key or login works, what is left of its day's count and when it starts again, a wait it asked
 * for, and whether it is answering.
 */
export function OutsideServices(props: { readonly providers: readonly Provider[] }): JSX.Element {
  const { t } = useTranslation();
  return (
    <section aria-labelledby="system-outside">
      <h2 id="system-outside">{t("system.outside")}</h2>
      <ul>
        {props.providers.map((provider) => (
          <li key={provider.destination}>
            <strong>{provider.name}</strong>{" "}
            {t("system.providerRate", {
              rate: tenthsText(provider.ratePerSecond, i18n.language),
              background: tenthsText(provider.backgroundRatePerSecond, i18n.language),
            })}{" "}
            {t(`system.credentials.${provider.credentials}`)}
            <ProviderStatus provider={provider} />
          </li>
        ))}
      </ul>
    </section>
  );
}

/** A provider's day's count, a wait it asked for, and whether it is down. */
function ProviderStatus(props: { readonly provider: Provider }): JSX.Element {
  const { t } = useTranslation();
  const { quota, waitUntil, down } = props.provider;
  return (
    <>
      {quota === undefined ? null : (
        <>
          {" "}
          {t("system.quota", { remaining: quota.remaining, allowed: quota.allowed })}
          {quota.resetAt === undefined
            ? null
            : ` ${t("system.quotaResets", { at: dateTimeText(quota.resetAt, i18n.language) })}`}
        </>
      )}
      {waitUntil === undefined
        ? null
        : ` ${t("system.waitUntil", { at: dateTimeText(waitUntil, i18n.language) })}`}
      {down === undefined ? null : <p role="alert">{waitingText(t, down, i18n.language)}</p>}
    </>
  );
}
