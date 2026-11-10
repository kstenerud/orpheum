// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { Link } from "./link.tsx";
import { Loaded } from "./loaded.tsx";
import { composeUpdateCommand, useUpdate, type Update } from "./update.ts";

/**
 * How to update, for an admin (security-updates.md Part B): the version the server runs, the newer
 * one and the security fix it lacks when there are, and the steps, the command among them: the
 * release's own install script, run again over the deployment, which keeps everything (D314); and,
 * for a server run from the compose file, which follows the latest release, pulling it (D450 §2).
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

/**
 * The How to update page's content for what the server knows: the steps of the way the server was
 * installed, and both ways' when it does not know (D450 §2).
 */
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
        <RouteSteps command={update.command} installRoute={update.installRoute} />
      )}
      {update.digest === undefined ? null : <p>{t("update.digest", { digest: update.digest })}</p>}
      {update.composeFile === undefined ? null : <NewerComposeFile file={update.composeFile} />}
    </>
  );
}

/**
 * A newer compose file than the one that started the server, and what it brings (D461 §3): shown
 * whether or not a newer release is, since pulling the images leaves the file as it was.
 */
function NewerComposeFile(props: {
  readonly file: NonNullable<Update["composeFile"]>;
}): JSX.Element {
  const { t } = useTranslation();
  const { file } = props;
  return (
    <>
      <p>{t("update.composeFileNewer", { version: file.version, newest: file.newest })}</p>
      <ul>
        {file.brings.map((brings) => (
          <li key={brings}>{brings}</li>
        ))}
      </ul>
      <p>{t("update.composeFileTake")}</p>
    </>
  );
}

/**
 * The steps of the way the server was installed; both ways', each saying which it is for, while the
 * server does not know (D450 §2).
 */
function RouteSteps(props: {
  readonly command: string;
  readonly installRoute: Update["installRoute"];
}): JSX.Element {
  const { t } = useTranslation();
  const { command, installRoute } = props;
  const isRouteUnknown = installRoute === undefined;
  return (
    <>
      {isRouteUnknown ? <p>{t("update.scriptIf")}</p> : null}
      {installRoute !== undefined && installRoute !== "script" ? null : (
        <ol>
          <li>{t("update.step1")}</li>
          <li>
            {t("update.step2")}
            <pre>
              <code>{command}</code>
            </pre>
          </li>
          <li>{t("update.step3")}</li>
        </ol>
      )}
      {isRouteUnknown ? <p>{t("update.composeIf")}</p> : null}
      {installRoute === "script" ? null : <ComposeSteps />}
    </>
  );
}

/**
 * How a server run from the compose file updates (D450 §2): its images follow the latest release, so
 * pulling them and starting again takes the new one, in OpenMediaVault's Compose page or at a prompt.
 */
function ComposeSteps(): JSX.Element {
  const { t } = useTranslation();
  return (
    <>
      <p>{t("update.composeIntro")}</p>
      <ul>
        <li>{t("update.composeOMV")}</li>
        <li>
          {t("update.composeDocker")}
          <pre>
            <code>{composeUpdateCommand}</code>
          </pre>
        </li>
      </ul>
    </>
  );
}

/**
 * The System page's Updates section (D450 §2): the version the server runs and a newer one, with how
 * to update it. Shown once read, and not at all while it is not.
 */
export function UpdateSystem(): JSX.Element | null {
  const { t } = useTranslation();
  const update = useUpdate(true);
  if (update.state !== "loaded" || update.value === undefined) {
    return null;
  }
  return (
    <section aria-labelledby="system-update">
      <h2 id="system-update">{t("update.settingsHeading")}</h2>
      <HowToUpdateView update={update.value} />
    </section>
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
