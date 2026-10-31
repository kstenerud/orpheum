// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { Link } from "./link.tsx";
import { Loaded } from "./loaded.tsx";
import { useUpdate, type Update } from "./update.ts";

/**
 * How to update, for an admin (security-updates.md Part B): the version the server runs, the newer
 * one and the security fix it lacks when there are, and the steps, the command among them: the
 * release's own install script, run again over the deployment, which keeps everything (D314).
 */
export function HowToUpdate(): JSX.Element {
  const { t } = useTranslation();
  const update = useUpdate(true);
  return (
    <section className="update" aria-labelledby="update-heading">
      <h1 id="update-heading">{t("update.heading")}</h1>
      <Loaded resource={update} render={(value) => <HowToUpdateView update={value} />} />
    </section>
  );
}

/** The How to update page's content for what the server knows. */
export function HowToUpdateView(props: {
  readonly update: Update | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { update } = props;
  if (update === undefined) {
    return null;
  }
  return (
    <>
      <UpdateStanding update={update} />
      {update.security === undefined ? null : (
        <p role="alert">
          {t("update.securityFix", {
            version: update.security.fixedIn,
            summary: update.security.summary.replace(/\.$/u, ""),
          })}
        </p>
      )}
      {update.command === undefined ? null : (
        <ol>
          <li>{t("update.step1")}</li>
          <li>
            {t("update.step2")}
            <pre>
              <code>{update.command}</code>
            </pre>
          </li>
          <li>{t("update.step3")}</li>
        </ol>
      )}
      {update.digest === undefined ? null : <p>{t("update.digest", { digest: update.digest })}</p>}
    </>
  );
}

/** The version the server runs, and the newer one when there is, or why none is known. */
function UpdateStanding(props: { readonly update: Update }): JSX.Element {
  const { t } = useTranslation();
  const { update } = props;
  let known = t("update.nothingKnown");
  if (update.version === "dev") {
    known = t("update.developmentBuild");
  } else if (update.latest !== undefined) {
    known = t("update.available", { latest: update.latest });
  } else if (update.checkedAt > 0) {
    known = t("update.latestKnown");
  }
  return (
    <p>
      {t("update.runs", { version: update.version })} {known}
    </p>
  );
}

/**
 * The settings page's Updates row: the version the server runs and a newer one, with the way to the
 * How to update page; the one place a newer release with no security fix shows (security-updates.md
 * Part B). Shown once read, and not at all while it is not: the settings beside it do not hang on it.
 */
export function UpdateSettings(): JSX.Element | null {
  const { t } = useTranslation();
  const update = useUpdate(true);
  if (update.state !== "loaded" || update.value === undefined) {
    return null;
  }
  return (
    <section aria-labelledby="update-settings-heading">
      <h2 className="settings__section" id="update-settings-heading">
        {t("update.settingsHeading")}
      </h2>
      <UpdateStanding update={update.value} />
      {update.value.command === undefined ? null : (
        <p>
          <Link to={{ screen: "update" }}>{t("update.howTo")}</Link>
        </p>
      )}
    </section>
  );
}
