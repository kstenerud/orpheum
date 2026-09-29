// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { register, secretFromHash } from "./accounts.ts";
import { CredentialsForm, CredentialsProblem, PasswordField } from "./credentials-form.tsx";
import { problemText } from "./problem.ts";
import { thisDevice } from "./device.ts";
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
  const [passwordAgain, setPasswordAgain] = useState("");
  const [problem, setProblem] = useState<RegisterProblem | undefined>(
    secret === undefined ? "noLink" : undefined,
  );
  const [refusalText, setRefusalText] = useState("");
  const submit = (): void => {
    if (secret === undefined) {
      return;
    }
    if (password !== passwordAgain) {
      setProblem("mismatch");
      return;
    }
    register(api, { secret, password, device: thisDevice(session) }).then(
      ({ token, user }) => {
        // The secret is spent; it should not stay in the address or the history.
        window.history.replaceState(null, "", "/");
        session.signIn(token, user.device?.id);
        navigation.go({ screen: "films" });
      },
      (cause: unknown) => {
        setRefusalText(problemText(t, cause, { authRequired: t("register.badLink") }));
        setProblem("refused");
      },
    );
  };
  return (
    <CredentialsForm heading={t("register.heading")} onSubmit={submit}>
      <p>{t("register.explain")}</p>
      <PasswordField label={t("register.password")} value={password} onChange={setPassword} />
      <PasswordField
        label={t("register.again")}
        value={passwordAgain}
        onChange={setPasswordAgain}
      />
      {problem === undefined ? null : (
        <CredentialsProblem isAlert>
          {t(`register.${problem}`, { message: refusalText })}
        </CredentialsProblem>
      )}
      <button type="submit" disabled={secret === undefined}>
        {t("register.submit")}
      </button>
    </CredentialsForm>
  );
}
