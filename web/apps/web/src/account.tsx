// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX, type SyntheticEvent } from "react";
import { useTranslation } from "react-i18next";

import {
  changePassword,
  issueDeviceCode,
  keepShownCode,
  shownCode,
  type Invitation,
} from "./accounts.ts";
import { problemText } from "./problem.ts";
import { YourDevices } from "./your-devices.tsx";
import { PasswordField } from "./register.tsx";
import { useServices } from "./services.tsx";
import { Share } from "./share.tsx";

/** A person's own account: their password, and adding a device such as a television (D218). */
export function Account(): JSX.Element {
  const { t } = useTranslation();
  return (
    <section className="account" aria-labelledby="account-heading">
      <h1 id="account-heading">{t("account.heading")}</h1>
      <YourDevices />
      <AddDevice />
      <ChangePassword />
    </section>
  );
}

/** This tab's own storage, or undefined where the browser refuses it. */
function tabStorage(): Storage | undefined {
  try {
    return window.sessionStorage;
  } catch {
    return undefined;
  }
}

/**
 * Asks for a code that signs a new device in as this person (D218 §3). The code stays on screen
 * across a reload, from this tab's own storage, until it expires: the server keeps only its hash.
 */
function AddDevice(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [name, setName] = useState("");
  const [code, setCode] = useState<Invitation | undefined>(() => {
    const storage = tabStorage();
    return storage === undefined ? undefined : shownCode(storage, Date.now());
  });
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const submit = (event: SyntheticEvent): void => {
    event.preventDefault();
    setProblem(undefined);
    issueDeviceCode(api, name).then(
      (made) => {
        setCode(made);
        const storage = tabStorage();
        if (storage !== undefined) {
          keepShownCode(storage, { ...made, deviceName: name });
        }
      },
      (cause: unknown) => {
        setProblem(problemText(t, cause));
      },
    );
  };
  return (
    <form className="account__part" onSubmit={submit}>
      <h2>{t("account.addDevice")}</h2>
      <p>{t("account.addDeviceExplain", { origin: window.location.origin })}</p>
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
      {code === undefined ? null : <Share what="code" text={code.secret} invitation={code} />}
      {problem === undefined ? null : <p role="alert">{problem}</p>}
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
        setOutcome(problemText(t, cause));
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
