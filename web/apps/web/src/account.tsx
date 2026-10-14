// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { changePassword, getOwnLink, listMyDevices, registrationUrl } from "./accounts.ts";

import { ActivateDevice } from "./device-activation.tsx";
import { FormRow } from "./form-row.tsx";
import { dataOrThrow, fieldProblemText } from "./problem.ts";
import { PasswordField } from "./credentials-form.tsx";
import { QrCode } from "./qr-code.tsx";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";
import { YourDevices } from "./your-devices.tsx";

/**
 * A person's own account: at the top, activating a device by the code it shows (D288); their
 * devices, the link they registered by, and their password (D218, D221). A device activated joins the list. A session that may not
 * change the account's security (a device a code activated, D292 §4) sees its devices and is told
 * where to do the rest.
 */
export function Account(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [devices, reload] = useResource("my-devices", () => listMyDevices(api));
  const [me] = useResource("me", async () => dataOrThrow(await api.GET("/me")));
  const canManage = me.state === "loaded" && me.value.canManageAccount;
  return (
    <section className="account" aria-labelledby="account-heading">
      <h1 id="account-heading">{t("account.heading")}</h1>
      {canManage ? <ActivateDevice onActivated={reload} /> : null}
      <YourDevices devices={devices} onChanged={reload} canManage={canManage} />
      <YourLink />
      {canManage ? <ChangePassword /> : null}
      {me.state === "loaded" && !canManage ? <p>{t("account.notOnThisDevice")}</p> : null}
    </section>
  );
}

/**
 * The link this person registered by, which leads any device of theirs back to the server and signs
 * no one in: to open on another device or scan with its camera (D292 §4, D290 §10). Nothing while
 * they have none the server kept.
 */
function YourLink(): JSX.Element | null {
  const { t } = useTranslation();
  const { api, navigation } = useServices();
  const [own] = useResource("own-link", () => getOwnLink(api));
  if (own.state !== "loaded" || own.value?.secret === undefined) {
    return null;
  }
  const text = registrationUrl(navigation.origin(), {
    secret: own.value.secret,
    link: own.value.link,
  });
  return (
    <section aria-labelledby="your-link-heading">
      <h2 id="your-link-heading">{t("account.yourLink")}</h2>
      <p>{t("account.yourLinkText")}</p>
      <QrCode text={text} label={t("account.yourLinkQr")} />
      <output className="share__link">{text}</output>
    </section>
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
