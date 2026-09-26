// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX, type SyntheticEvent } from "react";
import { useTranslation } from "react-i18next";

import {
  createAccount,
  issueDeviceCode,
  issueRegistrationLink,
  listAccounts,
  registrationUrl,
  type Account,
  type Invitation,
} from "./accounts.ts";
import { Loaded } from "./title.tsx";
import { useServices } from "./services.tsx";
import { Share } from "./share.tsx";
import { useResource } from "./use-resource.ts";

/** What the admin last made, to pass on: a person's link or a device's code. */
type Made =
  | { readonly what: "link"; readonly name: string; readonly invitation: Invitation }
  | { readonly what: "code"; readonly name: string; readonly invitation: Invitation };

/**
 * The household, for the admin (D218): everyone, whether they have registered, a way to add a
 * person, and for each a new registration link or a code for a device.
 */
export function People(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [accounts, reload] = useResource("accounts", () => listAccounts(api));
  const [made, setMade] = useState<Made | undefined>(undefined);
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const failed = (cause: unknown): void => {
    setProblem(cause instanceof Error ? cause.message : String(cause));
  };
  const act = {
    linkFor: (account: Account): void => {
      issueRegistrationLink(api, account.id).then((invitation) => {
        setMade({ what: "link", name: account.name, invitation });
      }, failed);
    },
    codeFor: (account: Account, deviceName: string): void => {
      issueDeviceCode(api, deviceName, account.id).then((invitation) => {
        setMade({ what: "code", name: account.name, invitation });
      }, failed);
    },
  };
  return (
    <section className="people" aria-labelledby="people-heading">
      <h1 id="people-heading">{t("people.heading")}</h1>
      <AddPerson
        onMade={(name, invitation) => {
          setMade({ what: "link", name, invitation });
          reload();
        }}
        onFailed={failed}
      />
      {made === undefined ? null : <MadeView made={made} />}
      {problem === undefined ? null : <p role="alert">{problem}</p>}
      <Loaded resource={accounts} render={(list) => <PeopleList accounts={list} act={act} />} />
    </section>
  );
}

/** What was just made, to pass on. */
function MadeView(props: { readonly made: Made }): JSX.Element {
  const { t } = useTranslation();
  const { made } = props;
  const text =
    made.what === "link"
      ? registrationUrl(window.location.origin, made.invitation.secret)
      : made.invitation.secret;
  return (
    <div className="people__made">
      <h2>{t(`people.made.${made.what}`, { name: made.name })}</h2>
      <Share what={made.what} text={text} invitation={made.invitation} />
    </div>
  );
}

/** Adds a person: their name and role, and their registration link to pass on. */
function AddPerson(props: {
  readonly onMade: (name: string, invitation: Invitation) => void;
  readonly onFailed: (cause: unknown) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [name, setName] = useState("");
  const [role, setRole] = useState<"admin" | "user">("user");
  const submit = (event: SyntheticEvent): void => {
    event.preventDefault();
    createAccount(api, name, role).then(({ registration }) => {
      props.onMade(name, registration);
      setName("");
    }, props.onFailed);
  };
  return (
    <form className="people__add" onSubmit={submit}>
      <h2>{t("people.add")}</h2>
      <label>
        {t("people.name")}{" "}
        <input
          required
          maxLength={100}
          value={name}
          onChange={(event) => {
            setName(event.target.value);
          }}
        />
      </label>
      <label>
        {t("people.role")}{" "}
        <select
          value={role}
          onChange={(event) => {
            setRole(event.target.value === "admin" ? "admin" : "user");
          }}
        >
          <option value="user">{t("people.roleUser")}</option>
          <option value="admin">{t("people.roleAdmin")}</option>
        </select>
      </label>
      <button type="submit">{t("people.create")}</button>
    </form>
  );
}

/** Everyone, each with what the admin can make for them. */
export function PeopleList(props: {
  readonly accounts: readonly Account[];
  readonly act: {
    readonly linkFor: (account: Account) => void;
    readonly codeFor: (account: Account, deviceName: string) => void;
  };
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <ul className="people__list">
      {props.accounts.map((account) => (
        <li key={account.id} className="people__person">
          <span className="people__name">{account.name}</span>{" "}
          <span className="people__status">
            {t(account.hasPassword ? "people.registered" : "people.waiting")}
          </span>{" "}
          <button
            type="button"
            onClick={() => {
              props.act.linkFor(account);
            }}
          >
            {t("people.newLink")}
          </button>{" "}
          <DeviceFor
            onCode={(deviceName) => {
              props.act.codeFor(account, deviceName);
            }}
          />
        </li>
      ))}
    </ul>
  );
}

/** A code for a device of one person: its name, then the code (D218 §3). */
function DeviceFor(props: { readonly onCode: (deviceName: string) => void }): JSX.Element {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  return (
    <form
      className="people__device"
      onSubmit={(event) => {
        event.preventDefault();
        props.onCode(name);
      }}
    >
      <input
        required
        maxLength={100}
        aria-label={t("account.deviceName")}
        placeholder={t("account.deviceNameExample")}
        value={name}
        onChange={(event) => {
          setName(event.target.value);
        }}
      />
      <button type="submit">{t("account.makeCode")}</button>
    </form>
  );
}
