// SPDX-License-Identifier: MPL-2.0

import { ApiProblem } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { copyToClipboard } from "./clipboard.ts";
import { dateTimeText } from "./date-time-text.ts";
import { Link } from "./link.tsx";
import { Loaded } from "./loaded.tsx";
import { dataOrThrow, problemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { composeUpdateCommand, useReadUpdateAgain, useUpdate, type Update } from "./update.tsx";
import { useRequest } from "./use-request.ts";

/** The way the server was installed, as the server says it (D450 §2, D462 §9). */
type InstallRoute = Update["installRoute"];

/**
 * How to update, for an admin (security-updates.md Part B), the one place for updating, which the
 * menu's Update opens (D462 §8): the version the server runs, the newer one and the security fix it
 * lacks when there are, Check now, and the steps of the way the server was installed, with the
 * filled-in compose file when it changed (D462 §4).
 */
export function HowToUpdate(): JSX.Element {
  const { t } = useTranslation();
  const update = useUpdate(true);
  // What Check now answered, over the update read it was pressed on: shown until the next read.
  const [checked, setChecked] = useState<{ readonly over: Update; readonly answer: Update }>();
  return (
    <section className="update" aria-labelledby="update-heading">
      <h1 id="update-heading">{t("update.heading")}</h1>
      <Loaded
        resource={update}
        render={(value) => {
          const shown = checked !== undefined && checked.over === value ? checked.answer : value;
          return (
            <>
              <HowToUpdateView update={shown} />
              <CheckNow
                update={shown}
                onChecked={(answer) => {
                  if (value !== undefined) {
                    setChecked({ over: value, answer });
                  }
                }}
              />
            </>
          );
        }}
      />
    </section>
  );
}

/**
 * Check now (D462 §8, §10.9): the server reads the release record at once, at most once a minute,
 * and the page shows what it found; one pressed sooner is told so, and the shell's own read of the
 * update is asked again, so the menu's mark follows.
 */
function CheckNow(props: {
  readonly update: Update | undefined;
  readonly onChecked: (answer: Update) => void;
}): JSX.Element | null {
  const { t, i18n } = useTranslation();
  const { api } = useServices();
  const readAgain = useReadUpdateAgain();
  const [isChecked, setIsChecked] = useState(false);
  const { isBusy, problem, runRequest } = useRequest((cause) => {
    if (cause instanceof ApiProblem && cause.withCode("rateLimited") !== undefined) {
      return t("update.checkedJustNow");
    }
    if (cause instanceof ApiProblem && cause.code === "conflict") {
      return t("update.nothingToCheck");
    }
    return problemText(t, cause);
  });
  const { update, onChecked } = props;
  if (update === undefined || update.version === "dev") {
    return null;
  }
  return (
    <>
      <p>
        <button
          type="button"
          disabled={isBusy}
          onClick={() => {
            setIsChecked(false);
            runRequest(async () => {
              onChecked(dataOrThrow(await api.POST("/system/update/check")));
              setIsChecked(true);
              readAgain();
            });
          }}
        >
          {t("update.checkNow")}
        </button>{" "}
        {isChecked && problem === undefined && update.problem === undefined ? (
          <output>
            {t("update.checkedAt", { at: dateTimeText(update.checkedAt, i18n.language) })}
          </output>
        ) : null}
      </p>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
    </>
  );
}

/**
 * The How to update page's content for what the server knows: the steps of the way the server was
 * installed, and every way's when it does not know (D450 §2, D462 §9).
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
      <CheckProblem update={update} />
      {update.security === undefined ? null : (
        <p role="alert">
          {t("update.securityFix", {
            version: update.security.fixedIn,
            summary: update.security.summary.replace(/\.$/u, ""),
          })}
        </p>
      )}
      {update.pinnedTag === undefined ? null : (
        <p role="alert">
          {t(update.installRoute === "synology" ? "update.pinnedTagSynology" : "update.pinnedTag", {
            tag: update.pinnedTag,
          })}
        </p>
      )}
      {update.command === undefined ? null : (
        <RouteSteps
          command={update.command}
          installRoute={update.installRoute}
          version={update.version}
        />
      )}
      {update.digest === undefined ? null : <p>{t("update.digest", { digest: update.digest })}</p>}
      {update.composeFile === undefined ? null : (
        <NewerComposeFile file={update.composeFile} installRoute={update.installRoute} />
      )}
    </>
  );
}

/**
 * What the last reading of the release record found wrong, and when one last succeeded (D462
 * §10.9), so an admin knows how old what the page says is; nothing while the last one held.
 */
function CheckProblem(props: { readonly update: Update }): JSX.Element | null {
  const { t, i18n } = useTranslation();
  const { update } = props;
  if (update.problem === undefined) {
    return null;
  }
  return (
    <p role="alert">
      {t("update.checkFailed", { problem: update.problem.replace(/\.$/u, "") })}{" "}
      {update.checkedAt > 0
        ? t("update.lastSucceeded", { at: dateTimeText(update.checkedAt, i18n.language) })
        : t("update.noneSucceeded")}
    </p>
  );
}

/**
 * A newer compose file than the one that started the server, and what it brings (D461 §3): shown
 * whether or not a newer release is, since pulling the images leaves the file as it was. The server
 * hands it over filled in with this household's values, to copy and paste over the old one, a copy
 * of which is kept (D462 §4, §10.5).
 */
function NewerComposeFile(props: {
  readonly file: NonNullable<Update["composeFile"]>;
  readonly installRoute: InstallRoute;
}): JSX.Element {
  const { t } = useTranslation();
  const { file, installRoute } = props;
  return (
    <>
      <p>{t("update.composeFileNewer", { version: file.version, newest: file.newest })}</p>
      <ul>
        {file.brings.map((brings) => (
          <li key={brings}>{brings}</li>
        ))}
      </ul>
      {file.file === "" ? (
        <p>{t("update.composeFileTake")}</p>
      ) : (
        <>
          <p>{t("update.composeFileKeepCopy")}</p>
          <p>{t(pasteStepOf(installRoute))}</p>
          <CopyText text={file.file} />
          <pre>
            <code>{file.file}</code>
          </pre>
        </>
      )}
    </>
  );
}

/** The step that takes a pasted compose file, in the way's own words (D462 §4). */
function pasteStepOf(
  installRoute: InstallRoute,
): "update.composeFilePasteOMV" | "update.composeFilePasteSynology" | "update.composeFilePaste" {
  if (installRoute === "omv") {
    return "update.composeFilePasteOMV";
  }
  return installRoute === "synology"
    ? "update.composeFilePasteSynology"
    : "update.composeFilePaste";
}

/** Copy, for text to paste, saying when the browser refused it, the text still there to select. */
function CopyText(props: { readonly text: string }): JSX.Element {
  const { t } = useTranslation();
  const [copied, setCopied] = useState<"copied" | "copyRefused" | undefined>(undefined);
  return (
    <p>
      <button
        type="button"
        onClick={() => {
          copyToClipboard(props.text).then(
            () => {
              setCopied("copied");
            },
            () => {
              setCopied("copyRefused");
            },
          );
        }}
      >
        {t("update.copy")}
      </button>{" "}
      {copied === undefined ? null : <output>{t(`update.${copied}`)}</output>}
    </p>
  );
}

/**
 * The steps of the way the server was installed; every way's, each saying which it is for, while the
 * server does not know (D450 §2).
 */
function RouteSteps(props: {
  readonly command: string;
  readonly installRoute: InstallRoute;
  readonly version: string;
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
      {installRoute === "script" ? null : (
        <ComposeSteps installRoute={installRoute} version={props.version} />
      )}
    </>
  );
}

/**
 * How a server run from a compose file updates (D450 §2, D462 §3): its images follow the latest
 * release, so pulling them and starting again takes the new one, by the NAS's own buttons or at a
 * prompt. Synology's Build can keep the image it has, so the page says what to do while the old
 * version still runs (D462 §10.1). The update may bring a newer compose file, offered here after it
 * (D462 §10.4).
 */
function ComposeSteps(props: {
  readonly installRoute: Exclude<InstallRoute, "script">;
  readonly version: string;
}): JSX.Element {
  const { t } = useTranslation();
  const { installRoute } = props;
  const omv = <li key="omv">{t("update.composeOMV")}</li>;
  const synology = <li key="synology">{t("update.composeSynology")}</li>;
  const docker = (
    <li key="docker">
      {t("update.composeDocker")}
      <pre>
        <code>{composeUpdateCommand}</code>
      </pre>
    </li>
  );
  const ways = {
    omv: [omv, docker],
    synology: [synology],
    portainer: [docker],
    casaos: [docker],
    docker: [docker],
  };
  return (
    <>
      <p>{t("update.composeIntro")}</p>
      <ul>{installRoute === undefined ? [omv, synology, docker] : ways[installRoute]}</ul>
      {installRoute === "synology" || installRoute === undefined ? (
        <p>{t("update.composeSynologyStill", { version: props.version })}</p>
      ) : null}
      <p>{t("update.composeMayBringFile")}</p>
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
