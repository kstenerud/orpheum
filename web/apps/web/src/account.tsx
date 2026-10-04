// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { changePassword, listMyDevices } from "./accounts.ts";
import {
  NewDeviceCodeForm,
  ShownDeviceCode,
  useDeviceCode,
  type DeviceCodeState,
} from "./device-code.tsx";
import { FormRow } from "./form-row.tsx";
import { fieldProblemText } from "./problem.ts";
import { PasswordField } from "./credentials-form.tsx";
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
  const deviceCode = useDeviceCode(reload);
  return (
    <section className="account" aria-labelledby="account-heading">
      <h1 id="account-heading">{t("account.heading")}</h1>
      {deviceCode.code === undefined ? null : <ShownDeviceCode code={deviceCode.code} />}
      <YourDevices
        devices={devices}
        onChanged={reload}
        onMakeCode={(device) => {
          deviceCode.makeCode({ deviceId: device.id }, device.name);
        }}
      />
      <AddDevice deviceCode={deviceCode} />
      <ChangePassword />
    </section>
  );
}

/** Makes a code that signs a new device in as this person (D218 §3). */
function AddDevice(props: { readonly deviceCode: DeviceCodeState }): JSX.Element {
  const { t } = useTranslation();
  return (
    <NewDeviceCodeForm
      onMakeCode={(name) => {
        props.deviceCode.makeCode({ deviceName: name }, name);
      }}
      problem={props.deviceCode.problem}
      isMaking={props.deviceCode.isMaking}
    >
      <h2>{t("account.addDevice")}</h2>
      <p>{t("account.addDeviceExplain")}</p>
    </NewDeviceCodeForm>
  );
}

/** Changes this person's password; nobody is signed out (D218 §6). */
function ChangePassword(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [outcome, setOutcome] = useState<string | undefined>(undefined);
  const submit = (): void => {
    changePassword(api, currentPassword, newPassword).then(
      () => {
        setCurrentPassword("");
        setNewPassword("");
        setOutcome(t("account.passwordChanged"));
      },
      (cause: unknown) => {
        setOutcome(
          fieldProblemText(t, cause, {
            fields: {
              "body.currentPassword": t("account.currentPassword"),
              "body.newPassword": t("account.newPassword"),
            },
            overrides: { forbidden: t("account.wrongPassword") },
          }),
        );
      },
    );
  };
  return (
    <FormRow onSubmit={submit}>
      <h2>{t("account.changePassword")}</h2>
      <PasswordField
        label={t("account.currentPassword")}
        value={currentPassword}
        onChange={setCurrentPassword}
        autoComplete="current-password"
      />
      <PasswordField
        label={t("account.newPassword")}
        value={newPassword}
        onChange={setNewPassword}
      />
      <button type="submit">{t("account.changePassword")}</button>
      {outcome === undefined ? null : <output>{outcome}</output>}
    </FormRow>
  );
}
