// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import {
  getPerson,
  issueRegistrationInvitation,
  listPersonDevices,
  removePerson,
  removePersonDevice,
  renamePerson,
  revokeOwnLink,
  setAdminNote,
  type Device,
  type Person,
} from "./accounts.ts";
import { BusyGroup } from "./busy-group.tsx";
import { ConfirmButton } from "./confirm-button.tsx";
import { dateText } from "./date-time-text.ts";
import { ActivateDevice } from "./device-activation.tsx";
import { FormRow } from "./form-row.tsx";
import { tenthsText } from "./number-text.ts";
import { IssuedInvitationView, type IssuedInvitation } from "./people.tsx";
import { personWords, registrationText } from "./person-words.ts";
import { dataOrThrow, fieldProblemText } from "./problem.ts";
import { listRoles, roleLabel, setPersonsRole } from "./roles.ts";
import { NoLibraryNote } from "./roles-section.tsx";
import { useServices } from "./services.tsx";
import { Loaded } from "./loaded.tsx";
import { SaveNote, useSaveState } from "./save-note.tsx";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";
import { RecentDevices } from "./your-devices.tsx";

/**
 * One person, for the admin: their name, which can be changed; a new registration invitation, or a
 * device of theirs activated by the code it shows (D288 §2); their devices, which can be removed;
 * what they have watched (the owner, 2026-09-27); and removing them, which an admin cannot do to
 * themselves (S40).
 */
export function PersonPage(props: { readonly userId: number }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [me] = useResource("me", async () => dataOrThrow(await api.GET("/me")));
  const isRemovable = me.state === "loaded" && me.value.id !== props.userId;
  const [person, reloadPerson] = useResource(`person/${String(props.userId)}`, () =>
    getPerson(api, props.userId),
  );
  const [devices, reloadDevices] = useResource(`person-devices/${String(props.userId)}`, () =>
    listPersonDevices(api, props.userId),
  );
  const { actions, issued, isBusy, problem } = usePersonActions(props.userId, {
    reloadPerson,
    reloadDevices,
  });
  return (
    <BusyGroup isBusy={isBusy}>
      <section className="person">
        <Loaded
          resource={person}
          render={(value) => (
            <>
              <PersonHead
                person={value}
                actions={actions}
                isRemovable={isRemovable}
                onActivated={reloadDevices}
              />
              <OwnLink person={value} onRevoke={actions.revokeOwnLink} />
            </>
          )}
        />
        {issued === undefined ? null : <IssuedInvitationView issued={issued} />}
        {problem === undefined ? null : <p role="alert">{problem}</p>}
        <h2>{t("person.devices")}</h2>
        <Loaded
          resource={devices}
          render={(list) => <RecentDevices devices={list} onRemove={actions.remove} />}
        />
      </section>
    </BusyGroup>
  );
}

/** What the admin does to a person. */
interface PersonActions {
  readonly rename: (name: string) => Promise<void>;
  readonly setNote: (note: string) => Promise<void>;
  readonly giveRole: (role: string) => Promise<void>;
  readonly issueLink: (name: string) => void;
  readonly remove: (device: Device) => void;
  readonly removePerson: () => void;
  readonly revokeOwnLink: () => void;
}

/** What the admin does to a person, one request at a time, and what it issued and why one failed. */
function usePersonActions(
  userId: number,
  reloads: { readonly reloadPerson: () => void; readonly reloadDevices: () => void },
): {
  readonly actions: PersonActions;
  readonly issued: IssuedInvitation | undefined;
  readonly isBusy: boolean;
  readonly problem: string | undefined;
} {
  const { t } = useTranslation();
  const { api, navigation } = useServices();
  const [issued, setIssued] = useState<IssuedInvitation | undefined>(undefined);
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    fieldProblemText(t, cause, personWords(t)),
  );
  // A change to the person, after which their page is read again.
  const change = (request: () => Promise<void>): void => {
    runRequest(async () => {
      await request();
      reloads.reloadPerson();
    });
  };
  // A setting saved at once: its field says how it went (SaveNote), and the page is read again.
  const saved = async (request: Promise<void>): Promise<void> => {
    await request;
    reloads.reloadPerson();
  };
  const actions = {
    rename: (name: string) => saved(renamePerson(api, userId, name)),
    setNote: (note: string) => saved(setAdminNote(api, userId, note)),
    giveRole: (role: string) => saved(setPersonsRole(api, userId, role)),
    issueLink: (name: string): void => {
      runRequest(async (idempotencyKey) => {
        const invitation = await issueRegistrationInvitation(api, userId, idempotencyKey);
        setIssued({ name, invitation });
      });
    },
    remove: (device: Device): void => {
      runRequest(async () => {
        await removePersonDevice(api, userId, device.id);
        reloads.reloadDevices();
      });
    },
    revokeOwnLink: (): void => {
      change(() => revokeOwnLink(api, userId));
    },
    removePerson: (): void => {
      runRequest(async () => {
        await removePerson(api, userId);
        navigation.go({ screen: "people" });
      });
    },
  };
  return { actions, issued, isBusy, problem };
}

/**
 * Until when the person's own link leads them back through a discovery, and ending it; nothing while
 * they have none (D292 §4).
 */
function OwnLink(props: {
  readonly person: Person;
  readonly onRevoke: () => void;
}): JSX.Element | null {
  const { t, i18n } = useTranslation();
  const { account, ownLinkExpiresAt } = props.person;
  if (ownLinkExpiresAt === undefined) {
    return null;
  }
  return (
    <div className="person__own-link">
      <p>{t("person.ownLink", { date: dateText(ownLinkExpiresAt, i18n.language) })}</p>
      <ConfirmButton
        label={t("person.revokeOwnLink")}
        consequence={t("person.revokeOwnLinkConsequence", { name: account.name })}
        confirmLabel={t("person.revokeOwnLinkConfirm")}
        onConfirm={props.onRevoke}
      />
    </div>
  );
}

/**
 * A person's name, what they have watched, the admin's actions on them in one row under it (New
 * link, and Remove where they may be removed; ui-audit-2026-11-04.md §3, A person), their settings,
 * each saved at once, and activating a device of theirs.
 */
function PersonHead(props: {
  readonly person: Person;
  readonly actions: PersonActions;
  readonly isRemovable: boolean;
  readonly onActivated: () => void;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  const { account, watchStats } = props.person;
  const hours = tenthsText(watchStats.watchedMs / 3_600_000, i18n.language);
  const lastWatched =
    watchStats.lastWatchedAt === undefined
      ? t("person.neverWatched")
      : dateText(watchStats.lastWatchedAt, i18n.language);
  return (
    <>
      <h1>{account.name}</h1>
      <p>
        {registrationText(t, account)} ·{" "}
        {t("person.watched", { hours, titles: watchStats.titlesWatchedCount, last: lastWatched })}
      </p>
      <div className="page-actions person__actions">
        <ConfirmButton
          label={t("people.newLink")}
          consequence={t("people.newLinkConsequence", { name: account.name })}
          confirmLabel={t("people.newLinkConfirm")}
          onConfirm={() => {
            props.actions.issueLink(account.name);
          }}
        />
        {props.isRemovable ? (
          <ConfirmButton
            label={t("person.remove", { name: account.name })}
            consequence={t("person.removeConsequence", { name: account.name })}
            confirmLabel={t("person.removeConfirm")}
            onConfirm={props.actions.removePerson}
          />
        ) : null}
      </div>
      <SavedField
        label={t("people.name")}
        initialValue={account.name}
        onSave={props.actions.rename}
      />
      <SavedField
        label={t("people.adminNote")}
        initialValue={account.adminNote ?? ""}
        onSave={props.actions.setNote}
        isOptional
        maxLength={200}
      />
      <RoleChoice current={account.role} onSave={props.actions.giveRole} />
      <ActivateDevice userId={account.id} onActivated={props.onActivated} />
    </>
  );
}

/**
 * The person's role, their whole access, which the admin may change (D243 §4); saved as it is chosen
 * (the owner, 2026-11-04, Q5), saying Saved, or the refusal, beside it.
 */
function RoleChoice(props: {
  readonly current: string;
  readonly onSave: (role: string) => Promise<void>;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [roles] = useResource("person-roles", () => listRoles(api));
  const [role, setRole] = useState(props.current);
  const { state, save } = useSaveState();
  const names =
    roles.state === "loaded" ? roles.value.roles.map((each) => each.name) : [props.current];
  return (
    <div className="form-row">
      <label>
        {t("people.role")}{" "}
        <select
          value={role}
          onChange={(event) => {
            const chosen = event.target.value;
            setRole(chosen);
            void save(() => props.onSave(chosen));
          }}
        >
          {names.map((name) => (
            <option key={name} value={name}>
              {roleLabel(t, name)}
            </option>
          ))}
        </select>
      </label>
      <SaveNote state={state} />
      <NoLibraryNote
        roles={roles.state === "loaded" ? roles.value.roles : undefined}
        roleName={role}
      />
    </div>
  );
}

/**
 * One text field of the person's, saved at once (the owner, 2026-11-04, Q5): on Enter or on leaving
 * it, and only a value not already sent; an optional one may be left empty. Saved, or the refusal,
 * shows beside it.
 */
function SavedField(props: {
  readonly label: string;
  readonly initialValue: string;
  readonly onSave: (value: string) => Promise<void>;
  readonly isOptional?: boolean;
  readonly maxLength?: number;
}): JSX.Element {
  const [value, setValue] = useState(props.initialValue);
  const [sent, setSent] = useState<string | undefined>(props.initialValue);
  const { state, save } = useSaveState();
  const send = (): void => {
    if (value === sent || (props.isOptional !== true && value.trim() === "")) {
      return;
    }
    setSent(value);
    void save(() => props.onSave(value)).then((isSaved) => {
      if (!isSaved) {
        setSent(undefined);
      }
    });
  };
  return (
    <FormRow onSubmit={send}>
      <label>
        {props.label}{" "}
        <input
          required={props.isOptional !== true}
          maxLength={props.maxLength ?? 100}
          value={value}
          onChange={(event) => {
            setValue(event.target.value);
          }}
          onBlur={send}
        />
      </label>
      <SaveNote state={state} />
    </FormRow>
  );
}
