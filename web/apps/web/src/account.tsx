// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX, type SyntheticEvent } from "react";
import { useTranslation } from "react-i18next";

import { changePassword, listMyDevices } from "./accounts.ts";
import { ShownDeviceCode, useDeviceCode, type DeviceCode } from "./device-code.tsx";
import { problemText } from "./problem.ts";
import { PasswordField } from "./register.tsx";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";
import { YourDevices } from "./your-devices.tsx";

/**
 * A person's own account: their devices, a code to add one or sign one back in, and their
 * password (D218, D221). A code on screen goes once it is used, and the list shows the device.
 */
export function Account(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [devices, reload] = useResource("my-devices", () => listMyDevices(api));
  const codes = useDeviceCode(reload);
  return (
    <section className="account" aria-labelledby="account-heading">
      <h1 id="account-heading">{t("account.heading")}</h1>
      {codes.code === undefined ? null : <ShownDeviceCode code={codes.code} />}
      <YourDevices
        devices={devices}
        reload={reload}
        onCodeFor={(device) => {
          codes.make({ deviceId: device.id }, device.name);
        }}
      />
      <AddDevice codes={codes} />
      <ChangePassword />
    </section>
  );
}

/** Makes a code that signs a new device in as this person (D218 §3). */
function AddDevice(props: { readonly codes: DeviceCode }): JSX.Element {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const submit = (event: SyntheticEvent): void => {
    event.preventDefault();
    props.codes.make({ deviceName: name }, name);
  };
  return (
    <form className="account__part" onSubmit={submit}>
      <h2>{t("account.addDevice")}</h2>
      <p>{t("account.addDeviceExplain")}</p>
      <label>
        {t("account.deviceName")}{" "}
        <input
          required
          maxLength={100}
          placeholder={t("account.deviceNameExample")}
          value={name}
          onChange={(event) => {
            setName(event.target.value);
          }}
        />
      </label>
      <button type="submit">{t("account.makeCode")}</button>
      {props.codes.problem === undefined ? null : <p role="alert">{props.codes.problem}</p>}
    </form>
  );
}

/** Changes this person's password; nobody is signed out (D218 §6). */
function ChangePassword(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [current, setCurrent] = useState("");
  const [replacement, setReplacement] = useState("");
  const [outcome, setOutcome] = useState<string | undefined>(undefined);
  const submit = (event: SyntheticEvent): void => {
    event.preventDefault();
    changePassword(api, current, replacement).then(
      () => {
        setCurrent("");
        setReplacement("");
        setOutcome(t("account.passwordChanged"));
      },
      (cause: unknown) => {
        setOutcome(problemText(t, cause, { forbidden: t("account.wrongPassword") }));
      },
    );
  };
  return (
    <form className="account__part" onSubmit={submit}>
      <h2>{t("account.changePassword")}</h2>
      <PasswordField
        label={t("account.currentPassword")}
        value={current}
        onChange={setCurrent}
        autoComplete="current-password"
      />
      <PasswordField
        label={t("account.newPassword")}
        value={replacement}
        onChange={setReplacement}
      />
      <button type="submit">{t("account.changePassword")}</button>
      {outcome === undefined ? null : <output>{outcome}</output>}
    </form>
  );
}
