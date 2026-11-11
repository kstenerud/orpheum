// SPDX-License-Identifier: MPL-2.0

import "./system-words.ts";

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { ConfirmButton } from "./confirm-button.tsx";
import { idempotent } from "./idempotent.ts";
import { ApiProblem, dataOrThrow, isRefusedAt, problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";

type NetworkState = components["schemas"]["NetworkStateBody"];
type Network = components["schemas"]["NetworkBody"];
type Setting = keyof Network;

/**
 * The house's network settings, which are the app's, not the compose file's (D462 §2): the ports
 * the server runs on, a form keeping new ones, which the server's next start takes, and what takes
 * them: this page's restart, or on the install script's way its next run. A port kept that would
 * not bind at the start is said with the default the server took instead, so no setting locks
 * anyone out (D462 §10.11). Nothing at all on a server that keeps no network settings.
 */
export function SystemNetwork(): JSX.Element | null {
  const { api } = useServices();
  const [state, reload] = useResource("system-network", async (signal) => {
    try {
      return dataOrThrow(await api.GET("/system/network", { signal: signal ?? null }));
    } catch (cause) {
      // A server that keeps no network settings says so with a conflict: show it no section.
      if (cause instanceof ApiProblem && cause.code === "conflict") {
        return undefined;
      }
      throw cause;
    }
  });
  if (state.state !== "loaded" || state.value === undefined) {
    return null;
  }
  return <SystemNetworkView state={state.value} onChanged={reload} />;
}

/** The section for state, read: what runs, what fell back, the form, and what takes it. */
export function SystemNetworkView(props: {
  readonly state: NetworkState;
  readonly onChanged: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { state } = props;
  return (
    <section aria-labelledby="system-network">
      <h2 id="system-network">{t("system.network.heading")}</h2>
      <p>{t("system.network.what")}</p>
      <p>
        {t("system.network.running", state.running)}{" "}
        {state.running.router === ""
          ? t("system.network.noRouter")
          : t("system.network.routerRunning", { router: state.running.router })}
      </p>
      {state.fellBack.map((setting) => (
        <p key={setting}>
          <output>
            {t(
              setting === "httpsPort"
                ? "system.network.fellBackHttpsPort"
                : "system.network.fellBackWebrtcPort",
              { kept: state.kept[setting], running: state.running[setting] },
            )}
          </output>
        </p>
      ))}
      <NetworkForm kept={state.kept} onChanged={props.onChanged} />
      {state.pending || state.fellBack.length > 0 ? <Takes appliedBy={state.appliedBy} /> : null}
    </section>
  );
}

/** What takes the settings kept: this page's restart, or the install script's next run. */
function Takes(props: { readonly appliedBy: NetworkState["appliedBy"] }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [isRestarting, setIsRestarting] = useState(false);
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  if (props.appliedBy === "script") {
    return <p>{t("system.network.pendingScript")}</p>;
  }
  return (
    <>
      <p>{t("system.network.pendingRestart")}</p>
      <ConfirmButton
        label={t("system.network.restart")}
        isDisabled={isBusy || isRestarting}
        consequence={t("system.network.restartConsequence")}
        confirmLabel={t("system.network.restartConfirm")}
        onConfirm={() => {
          runRequest(async (idempotencyKey) => {
            throwIfRefused(await api.POST("/system/restart", idempotent(idempotencyKey)));
            setIsRestarting(true);
          });
        }}
      />
      {isRestarting ? <output>{t("system.network.restarting")}</output> : null}
      {problem === undefined ? null : <p role="alert">{problem}</p>}
    </>
  );
}

/** The form keeping new settings, saying which one the server refused and why, in plain words. */
function NetworkForm(props: {
  readonly kept: Network;
  readonly onChanged: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [entered, setEntered] = useState({
    httpsPort: String(props.kept.httpsPort),
    webrtcPort: String(props.kept.webrtcPort),
    router: props.kept.router,
  });
  const [isSaved, setIsSaved] = useState(false);
  const { isBusy, problem, runRequest } = useRequest((cause) => {
    const setting = refusedSetting(cause);
    return setting === undefined ? problemText(t, cause) : t(refusalKey[setting]);
  });
  const field = (setting: Setting): JSX.Element => (
    <NetworkField
      label={t(`system.network.${setting}`)}
      isPort={setting !== "router"}
      value={entered[setting]}
      onChange={(value) => {
        setIsSaved(false);
        setEntered({ ...entered, [setting]: value });
      }}
    />
  );
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        setIsSaved(false);
        runRequest(async () => {
          throwIfRefused(
            await api.PUT("/system/network", {
              body: {
                httpsPort: Number(entered.httpsPort.trim()),
                webrtcPort: Number(entered.webrtcPort.trim()),
                router: entered.router.trim(),
              },
            }),
          );
          setIsSaved(true);
          props.onChanged();
        });
      }}
    >
      {field("httpsPort")}
      {field("webrtcPort")}
      {field("router")}
      <p>
        <button type="submit" disabled={isBusy}>
          {t("system.network.save")}
        </button>{" "}
        {isSaved ? <output>{t("system.network.saved")}</output> : null}
      </p>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
    </form>
  );
}

/** One setting's field: a port, or the router's address. */
function NetworkField(props: {
  readonly label: string;
  readonly isPort: boolean;
  readonly value: string;
  readonly onChange: (value: string) => void;
}): JSX.Element {
  return (
    <p>
      <label>
        {props.label}{" "}
        <input
          type="text"
          inputMode={props.isPort ? "numeric" : "decimal"}
          value={props.value}
          onChange={(event) => {
            props.onChange(event.target.value);
          }}
        />
      </label>
    </p>
  );
}

const refusalKey = {
  httpsPort: "system.network.refusedHttpsPort",
  webrtcPort: "system.network.refusedWebrtcPort",
  router: "system.network.refusedRouter",
} as const;

/** The setting the server refused, by its invalidRequest's location ("body.httpsPort"). */
function refusedSetting(cause: unknown): Setting | undefined {
  const settings: readonly Setting[] = ["httpsPort", "webrtcPort", "router"];
  return settings.find((setting) => isRefusedAt(cause, `body.${setting}`));
}
