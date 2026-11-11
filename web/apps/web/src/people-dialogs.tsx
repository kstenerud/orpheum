// SPDX-License-Identifier: MPL-2.0

import "./people-words.ts";

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { createAccount, invite, type Invitation, type NewAccount } from "./accounts.ts";
import { BusyGroup } from "./busy-group.tsx";
import { FormRow } from "./form-row.tsx";
import { ModalDialog } from "./modal-dialog.tsx";
import { personWords } from "./person-words.ts";
import { useFieldMarks, type FieldMarks } from "./field-marks.ts";
import { failedFields, fieldProblemText } from "./problem.ts";
import { roleLabel, type Role } from "./roles.ts";
import { NoLibraryNote } from "./roles-section.tsx";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";

/** The roles a person may be given, for a form's chooser: their names, and the roles themselves. */
export interface RoleChoices {
  readonly roleNames: readonly string[];
  readonly roles: readonly Role[] | undefined;
}

/** A role chooser over the roles there are, saying when the one chosen reaches no library. */
function RoleSelect(props: {
  readonly choices: RoleChoices;
  readonly role: string;
  readonly onChange: (role: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <>
      <label>
        {t("people.role")}{" "}
        <select
          value={props.role}
          onChange={(event) => {
            props.onChange(event.target.value);
          }}
        >
          {props.choices.roleNames.map((roleName) => (
            <option key={roleName} value={roleName}>
              {roleLabel(t, roleName)}
            </option>
          ))}
        </select>
      </label>
      <NoLibraryNote roles={props.choices.roles} roleName={props.role} />
    </>
  );
}

/**
 * The dialog that invites someone to make their own account (D399): the role it will have, and the
 * admin's note of who it is for, which the admin must write and only the admin sees (D246 §2). Once
 * made, it tells the page, which opens the invitation's own dialog at once, with the link to pass
 * on.
 */
export function InviteDialog(props: {
  readonly choices: RoleChoices;
  readonly onClose: () => void;
  readonly onInvited: (
    invitation: Invitation,
    link: components["schemas"]["IssuedSecretBody"],
  ) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { isBusy, problem, failure, runRequest } = useRequest((cause) =>
    fieldProblemText(t, cause, personWords(t)),
  );
  const marks = useFieldMarks(failure, failedFields(failure, personWords(t)));
  const [role, setRole] = useState("user");
  const [adminNote, setAdminNote] = useState("");
  return (
    <ModalDialog label={t("people.invite")} shouldFocusFirstField onClose={props.onClose}>
      <BusyGroup isBusy={isBusy}>
        <FormRow
          onSubmit={() => {
            runRequest(async (idempotencyKey) => {
              const made = await invite(api, { role, adminNote }, idempotencyKey);
              props.onInvited(made.invitation, made.link);
            });
          }}
        >
          <h2>{t("people.invite")}</h2>
          <p>{t("people.inviteExplain")}</p>
          <label>
            {t("people.adminNote")}{" "}
            <input
              required
              maxLength={200}
              placeholder={t("people.adminNoteExample")}
              value={adminNote}
              {...marks.markOf("body.adminNote")}
              onChange={(event) => {
                marks.editing("body.adminNote", setAdminNote)(event.target.value);
              }}
            />
          </label>
          <RoleSelect choices={props.choices} role={role} onChange={setRole} />
          <button type="submit">{t("people.makeLink")}</button>
        </FormRow>
      </BusyGroup>
      {problem === undefined ? null : (
        <p role="alert" id={marks.problemId}>
          {problem}
        </p>
      )}
    </ModalDialog>
  );
}

/** The fields of an account set up in full. */
function AccountFields(props: {
  readonly choices: RoleChoices;
  readonly account: NewAccount;
  readonly set: (change: Partial<NewAccount>) => void;
  /** The marks on the fields the failure shown is about. */
  readonly marks: FieldMarks;
}): JSX.Element {
  const { t } = useTranslation();
  const { account, set, marks } = props;
  return (
    <>
      <label>
        {t("people.name")}{" "}
        <input
          required
          maxLength={100}
          value={account.name}
          {...marks.markOf("body.name")}
          onChange={(event) => {
            marks.editing("body.name", set)({ name: event.target.value });
          }}
        />
      </label>
      <label>
        {t("people.password")}{" "}
        <input
          required
          type="password"
          autoComplete="new-password"
          value={account.password}
          {...marks.markOf("body.password")}
          onChange={(event) => {
            marks.editing("body.password", set)({ password: event.target.value });
          }}
        />
      </label>
      <RoleSelect
        choices={props.choices}
        role={account.role}
        onChange={(role) => {
          set({ role });
        }}
      />
      <label>
        {t("people.adminNote")}{" "}
        <input
          maxLength={200}
          value={account.adminNote}
          {...marks.markOf("body.adminNote")}
          onChange={(event) => {
            marks.editing("body.adminNote", set)({ adminNote: event.target.value });
          }}
        />
      </label>
    </>
  );
}

/**
 * The dialog that sets an account up in full: a name, a password, a role, and the admin's note
 * (D246 §2). Once made, the dialog closes and the page lists it; nothing else opens (D399).
 */
export function CreateAccountDialog(props: {
  readonly choices: RoleChoices;
  readonly onClose: () => void;
  readonly onCreated: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { isBusy, problem, failure, runRequest } = useRequest((cause) =>
    fieldProblemText(t, cause, personWords(t)),
  );
  const marks = useFieldMarks(failure, failedFields(failure, personWords(t)));
  const [account, setAccount] = useState<NewAccount>({
    name: "",
    password: "",
    role: "user",
    adminNote: "",
  });
  const set = (change: Partial<NewAccount>): void => {
    setAccount({ ...account, ...change });
  };
  return (
    <ModalDialog label={t("people.setUp")} shouldFocusFirstField onClose={props.onClose}>
      <BusyGroup isBusy={isBusy}>
        <FormRow
          onSubmit={() => {
            runRequest(async (idempotencyKey) => {
              await createAccount(api, account, idempotencyKey);
              props.onCreated();
            });
          }}
        >
          <h2>{t("people.setUp")}</h2>
          <AccountFields choices={props.choices} account={account} set={set} marks={marks} />
          <button type="submit">{t("people.create")}</button>
        </FormRow>
      </BusyGroup>
      {problem === undefined ? null : (
        <p role="alert" id={marks.problemId}>
          {problem}
        </p>
      )}
    </ModalDialog>
  );
}
