// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import {
  changePassword,
  getOwnLink,
  listMyDevices,
  registrationUrl,
  setAdminAway,
} from "./accounts.ts";

import { AutoplaySetting } from "./autoplay-setting.tsx";
import { DebugToolsSetting } from "./debug-tools.tsx";
import { ActivateDevice } from "./device-activation.tsx";
import { FormRow } from "./form-row.tsx";
import { HomeAddress } from "./home-address.tsx";
import { dataOrThrow, fieldProblemText } from "./problem.ts";
import { PasswordField } from "./credentials-form.tsx";
import { QrCode } from "./qr-code.tsx";
import { useServices } from "./services.tsx";
import { useResource, type Resource } from "./use-resource.ts";
import { YourDevices } from "./your-devices.tsx";

/**
 * A person's own account, in the order of how often each is used (ui-audit-2026-11-04.md §3, Your
 * account): under its heading, who is signed in (the owner, 2026-10-30: "nothing anywhere to tell me
 * who I'm logged in as"); then activating a device by the code it shows (D288), its code, name and
 * Activate in one row; how their episodes play on (plans/up-next.md); their password (D218, D221) and,
 * for an admin, administering from away (D302); the server's home address (D292 §6) and the link they
 * registered by; their devices, the most recent five, with signing out everywhere else; and, last,
 * whether their pages show debug tools. A device activated joins the list. A session that may not
 * change the account's security (a device a code activated, D292 §4) is told so at the top, where
 * what it may not do would stand, and sees its devices.
 */
export function Account(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [devices, reload] = useResource("my-devices", () => listMyDevices(api));
  const [me, reloadMe] = useResource("me", async () => dataOrThrow(await api.GET("/me")));
  const canManage = me.state === "loaded" && me.value.canManageAccount;
  return (
    <section className="account" aria-labelledby="account-heading">
      <h1 id="account-heading">{t("account.heading")}</h1>
      <SignedInAs me={me} />
      <NotOnThisDevice isShown={me.state === "loaded" && !canManage} />
      {canManage ? <ActivateHere onActivated={reload} /> : null}
      <AutoplaySetting />
      {canManage ? <ChangePassword /> : null}
      {me.state === "loaded" && me.value.adminAway !== undefined ? (
        <AdminAway adminAway={me.value.adminAway} onChanged={reloadMe} />
      ) : null}
      <YourServerAtHome
        addresses={me.state === "loaded" ? (me.value.homeAddresses ?? undefined) : undefined}
      />
      {canManage ? <YourLink /> : null}
      <YourDevices devices={devices} onChanged={reload} canManage={canManage} />
      <DebugToolsSetting />
    </section>
  );
}

/**
 * Activate a device on one's account, with the device's QR code the address carries, if any (D321),
 * which leaves the address once answered.
 */
function ActivateHere(props: { readonly onActivated: () => void }): JSX.Element {
  const { navigation } = useServices();
  const route = navigation.route();
  const scanned = route.screen === "account" ? route.scanned : undefined;
  if (scanned === undefined) {
    return <ActivateDevice onActivated={props.onActivated} />;
  }
  return (
    <ActivateDevice
      onActivated={props.onActivated}
      scanned={scanned}
      onScannedAnswered={() => {
        navigation.replace({ screen: "account" });
      }}
    />
  );
}

/** Who is signed in, by me, the page's GET /me, said once it is read. */
function SignedInAs(props: {
  readonly me: Resource<{ readonly name: string }>;
}): JSX.Element | null {
  const { t } = useTranslation();
  return props.me.state === "loaded" ? (
    <p className="account__who">{t("account.signedInAs", { name: props.me.value.name })}</p>
  ) : null;
}

/** What a device a code activated cannot do here, said once the page knows it is one. */
function NotOnThisDevice(props: { readonly isShown: boolean }): JSX.Element | null {
  const { t } = useTranslation();
  return props.isShown ? <p>{t("account.notOnThisDevice")}</p> : null;
}

/**
 * An admin's own setting: whether they administer from away from home too (D302), which is off
 * unless they turn it on, from home, and said with what it risks.
 */
function AdminAway(props: {
  readonly adminAway: "never" | "always";
  readonly onChanged: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const isAlways = props.adminAway === "always";
  return (
    <section aria-labelledby="admin-away-heading">
      <h2 id="admin-away-heading">{t("account.adminAway")}</h2>
      <p>{t("account.adminAwayText")}</p>
      <label>
        <input
          type="checkbox"
          checked={isAlways}
          onChange={(event) => {
            setProblem(undefined);
            setAdminAway(api, event.target.checked ? "always" : "never").then(
              props.onChanged,
              (cause: unknown) => {
                setProblem(
                  fieldProblemText(t, cause, {
                    fields: {},
                    overrides: { forbidden: t("account.adminAwayAtHome") },
                  }),
                );
              },
            );
          }}
        />{" "}
        {t("account.adminAwayAlways")}
      </label>
      {problem === undefined ? null : <output>{problem}</output>}
    </section>
  );
}

/** The server's home address, told every household (D292 §6); nothing until it has one. */
function YourServerAtHome(props: {
  readonly addresses: readonly string[] | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  if (props.addresses === undefined || props.addresses.length === 0) {
    return null;
  }
  return (
    <section aria-labelledby="home-address-heading">
      <h2 id="home-address-heading">{t("homeAddress.heading")}</h2>
      <HomeAddress addresses={props.addresses} />
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
