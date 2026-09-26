// SPDX-License-Identifier: MPL-2.0

import "./sign-in.css";

import { useState, type JSX, type SyntheticEvent } from "react";
import { useTranslation } from "react-i18next";

import { register, secretFromHash } from "./accounts.ts";
import { deviceFrom } from "./device.ts";
import { useServices } from "./services.tsx";

/** Why a registration did not go through. */
export type RegisterProblem = "noLink" | "mismatch" | "refused";

/**
 * Registration from a one-time link (D218 §2): the person chooses a password, and this device is
 * signed in as theirs. The link's secret is after #, so no request carried it here.
 */
export function Register(): JSX.Element {
  const { t } = useTranslation();
  const { api, session, navigation } = useServices();
  const [secret] = useState(() => secretFromHash(window.location.hash));
  const [password, setPassword] = useState("");
  const [again, setAgain] = useState("");
  const [problem, setProblem] = useState<RegisterProblem | undefined>(
    secret === undefined ? "noLink" : undefined,
  );
  const [message, setMessage] = useState("");
  const submit = (event: SyntheticEvent): void => {
    event.preventDefault();
    if (secret === undefined) {
      return;
    }
    if (password !== again) {
      setProblem("mismatch");
      return;
    }
    register(api, { secret, password, device: deviceFrom(navigator.userAgent) }).then(
      ({ token }) => {
        // The secret is spent; it should not stay in the address or the history.
        window.history.replaceState(null, "", "/");
        session.signIn(token);
        navigation.go({ screen: "films" });
      },
      (cause: unknown) => {
        setMessage(cause instanceof Error ? cause.message : String(cause));
        setProblem("refused");
      },
    );
  };
  return (
    <main className="sign-in">
      <form className="sign-in__form" onSubmit={submit}>
        <h1>{t("register.heading")}</h1>
        <p>{t("register.explain")}</p>
        <PasswordField label={t("register.password")} value={password} onChange={setPassword} />
        <PasswordField label={t("register.again")} value={again} onChange={setAgain} />
        {problem === undefined ? null : (
          <p className="sign-in__problem" role="alert">
            {t(`register.${problem}`, { message })}
          </p>
        )}
        <button type="submit" disabled={secret === undefined}>
          {t("register.submit")}
        </button>
      </form>
    </main>
  );
}

/** A new password, typed. */
export function PasswordField(props: {
  readonly label: string;
  readonly value: string;
  readonly onChange: (value: string) => void;
  readonly autoComplete?: "new-password" | "current-password";
}): JSX.Element {
  return (
    <label className="sign-in__field">
      {props.label}
      <input
        type="password"
        autoComplete={props.autoComplete ?? "new-password"}
        required
        value={props.value}
        onChange={(event) => {
          props.onChange(event.target.value);
        }}
      />
    </label>
  );
}
