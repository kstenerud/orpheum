// SPDX-License-Identifier: MPL-2.0

import "./people-words.ts";

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import {
  cancelRegistrationLinks,
  getUser,
  issueRegistrationInvitation,
  listUserDevices,
  removeUser,
  removeUserDevice,
  renameUser,
  revokeOwnLink,
  setAdminNote,
  type Device,
  type UserPageBody,
} from "./accounts.ts";
import { BusyGroup } from "./busy-group.tsx";
import { ConfirmButton } from "./confirm-button.tsx";
import { dateText, dateTimeText } from "./date-time-text.ts";
import { ActivateDevice } from "./device-activation.tsx";
import { FormRow } from "./form-row.tsx";
import { tenthsText } from "./number-text.ts";
import { IssuedInvitationView, type IssuedInvitation } from "./users.tsx";
import { personWords, registrationText } from "./person-words.ts";
import { dataOrThrow, fieldProblemText } from "./problem.ts";
import { listRoles, roleLabel, setUserRole } from "./roles.ts";
import { NoLibraryNote } from "./roles-section.tsx";
import { useServices } from "./services.tsx";
import { Loaded } from "./loaded.tsx";
import { SaveNote, useSaveState } from "./save-note.tsx";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";
import { RecentDevices } from "./your-devices.tsx";

/**
 * One person, for the admin: their name, which can be changed; a reset password link, one at a
 * time, until when it is live, Cancel and Renew (D466), or for one not registered yet a new
 * registration link; a device of theirs activated by the code it shows (D288 §2); their devices, which can be removed;
 * what they have watched (the owner, 2026-09-27); and removing them, which an admin cannot do to
 * themselves (S40).
 */
export function UserPage(props: { readonly userId: number }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [me] = useResource("me", async () => dataOrThrow(await api.GET("/me")));
  const isRemovable = me.state === "loaded" && me.value.id !== props.userId;
  const [person, reloadUser] = useResource(`user/${String(props.userId)}`, () =>
    getUser(api, props.userId),
  );
  const [devices, reloadDevices] = useResource(`person-devices/${String(props.userId)}`, () =>
    listUserDevices(api, props.userId),
  );
  const { actions, issued, isBusy, problem } = usePersonActions(props.userId, {
    reloadUser,
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
              <ResetLink person={value} actions={actions} />
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
  readonly issueLink: (name: string, isReset: boolean) => void;
  readonly cancelResetLink: () => void;
  readonly remove: (device: Device) => void;
  readonly removeUser: () => void;
  readonly revokeOwnLink: () => void;
}

/** What the admin does to a person, one request at a time, and what it issued and why one failed. */
function usePersonActions(
  userId: number,
  reloads: { readonly reloadUser: () => void; readonly reloadDevices: () => void },
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
      reloads.reloadUser();
    });
  };
  // A setting saved at once: its field says how it went (SaveNote), and the page is read again.
  const saved = async (request: Promise<void>): Promise<void> => {
    await request;
    reloads.reloadUser();
  };
  const actions = {
    rename: (name: string) => saved(renameUser(api, userId, name)),
    setNote: (note: string) => saved(setAdminNote(api, userId, note)),
    giveRole: (role: string) => saved(setUserRole(api, userId, role)),
    // A reset link replaces the outstanding one (D466 §2); the page, read again, says until when.
    issueLink: (name: string, isReset: boolean): void => {
      runRequest(async (idempotencyKey) => {
        const invitation = await issueRegistrationInvitation(api, userId, idempotencyKey);
        setIssued({ name, invitation, isReset });
        reloads.reloadUser();
      });
    },
    // The link shown, cancelled, no longer works, so it is no longer shown (D466 §4).
    cancelResetLink: (): void => {
      change(async () => {
        await cancelRegistrationLinks(api, userId);
        setIssued(undefined);
      });
    },
    remove: (device: Device): void => {
      runRequest(async () => {
        await removeUserDevice(api, userId, device.id);
        reloads.reloadDevices();
      });
    },
    revokeOwnLink: (): void => {
      change(() => revokeOwnLink(api, userId));
    },
    removeUser: (): void => {
      runRequest(async () => {
        await removeUser(api, userId);
        navigation.go({ screen: "users" });
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
  readonly person: UserPageBody;
  readonly onRevoke: () => void;
}): JSX.Element | null {
  const { t, i18n } = useTranslation();
  const { user, ownLinkExpiresAt } = props.person;
  if (ownLinkExpiresAt === undefined) {
    return null;
  }
  return (
    <div className="person__own-link">
      <p>{t("person.ownLink", { date: dateText(ownLinkExpiresAt, i18n.language) })}</p>
      <ConfirmButton
        label={t("person.revokeOwnLink")}
        consequence={t("person.revokeOwnLinkConsequence", { name: user.name })}
        confirmLabel={t("person.revokeOwnLinkConfirm")}
        onConfirm={props.onRevoke}
      />
    </div>
  );
}

/**
 * Until when a registered person's reset password link is live, with Cancel, which ends it, and
 * Renew, which makes a new one in its place and shows it; never the outstanding link itself, and
 * nothing while there is none (D466 §4).
 */
function ResetLink(props: {
  readonly person: UserPageBody;
  readonly actions: PersonActions;
}): JSX.Element | null {
  const { t, i18n } = useTranslation();
  const { user, resetLinkExpiresAt } = props.person;
  if (!user.hasPassword || resetLinkExpiresAt === undefined) {
    return null;
  }
  return (
    <div className="person__reset-link">
      <p>{t("person.resetLink", { date: dateTimeText(resetLinkExpiresAt, i18n.language) })}</p>
      <div className="page-actions">
        <ConfirmButton
          label={t("person.cancelResetLink")}
          consequence={t("person.cancelResetLinkConsequence", { name: user.name })}
          confirmLabel={t("person.cancelResetLinkConfirm")}
          onConfirm={props.actions.cancelResetLink}
        />
        <ConfirmButton
          label={t("person.renewResetLink")}
          consequence={t("person.renewResetLinkConsequence", { name: user.name })}
          confirmLabel={t("person.renewResetLinkConfirm")}
          onConfirm={() => {
            props.actions.issueLink(user.name, true);
          }}
        />
      </div>
    </div>
  );
}

/**
 * The button that makes a person's link: for a registered person a reset password link (D466 §1),
 * which, while one is outstanding, ResetLink's Renew stands in for; for one not yet registered a
 * first link, any sent before staying live (D318 §1).
 */
function NewLinkButton(props: {
  readonly person: UserPageBody;
  readonly actions: PersonActions;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { user, resetLinkExpiresAt } = props.person;
  if (!user.hasPassword) {
    return (
      <ConfirmButton
        label={t("people.newLink")}
        consequence={t("people.newLinkConsequence", { name: user.name })}
        confirmLabel={t("people.newLinkConfirm")}
        onConfirm={() => {
          props.actions.issueLink(user.name, false);
        }}
      />
    );
  }
  if (resetLinkExpiresAt !== undefined) {
    return null;
  }
  return (
    <ConfirmButton
      label={t("person.newResetLink")}
      consequence={t("person.newResetLinkConsequence", { name: user.name })}
      confirmLabel={t("person.newResetLinkConfirm")}
      onConfirm={() => {
        props.actions.issueLink(user.name, true);
      }}
    />
  );
}

/**
 * A person's name, what they have watched, the admin's actions on them in one row under it (New
 * reset password link, or New link for one not yet registered, and Remove where they may be
 * removed; ui-audit-2026-11-04.md §3, A person), their settings, each saved at once, and
 * activating a device of theirs.
 */
function PersonHead(props: {
  readonly person: UserPageBody;
  readonly actions: PersonActions;
  readonly isRemovable: boolean;
  readonly onActivated: () => void;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  const { user, watchStats } = props.person;
  const hours = tenthsText(watchStats.watchedMs / 3_600_000, i18n.language);
  const lastWatched =
    watchStats.lastWatchedAt === undefined
      ? t("person.neverWatched")
      : dateText(watchStats.lastWatchedAt, i18n.language);
  return (
    <>
      <h1>{user.name}</h1>
      <p>
        {registrationText(t, user)} ·{" "}
        {t("person.watched", { hours, titles: watchStats.titlesWatchedCount, last: lastWatched })}
      </p>
      <div className="page-actions person__actions">
        <NewLinkButton person={props.person} actions={props.actions} />
        {props.isRemovable ? (
          <ConfirmButton
            label={t("person.remove", { name: user.name })}
            consequence={t("person.removeConsequence", { name: user.name })}
            confirmLabel={t("person.removeConfirm")}
            onConfirm={props.actions.removeUser}
          />
        ) : null}
      </div>
      <SavedField label={t("people.name")} initialValue={user.name} onSave={props.actions.rename} />
      <SavedField
        label={t("people.adminNote")}
        initialValue={user.adminNote ?? ""}
        onSave={props.actions.setNote}
        isOptional
        maxLength={200}
      />
      <RoleChoice current={user.role} onSave={props.actions.giveRole} />
      <ActivateDevice userId={user.id} onActivated={props.onActivated} />
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
