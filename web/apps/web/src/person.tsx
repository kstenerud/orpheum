// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import {
  getPerson,
  issueDeviceCode,
  issueRegistrationInvitation,
  listPersonDevices,
  removePersonDevice,
  renamePerson,
  type Device,
  type Person,
} from "./accounts.ts";
import { BusyGroup } from "./busy-group.tsx";
import { dateText } from "./date-time-text.ts";
import { NewDeviceCodeForm } from "./device-code.tsx";
import { FormRow } from "./form-row.tsx";
import { tenthsText } from "./number-text.ts";
import {
  IssuedInvitationView,
  personWords,
  registrationText,
  type IssuedInvitation,
} from "./people.tsx";
import { fieldProblemText } from "./problem.ts";
import { listRoles, roleLabel, setPersonsRole } from "./roles.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";
import { DeviceList } from "./your-devices.tsx";

/**
 * One person, for the admin: their name, which can be changed; a new registration invitation or a
 * code for one of their devices; their devices, which can be removed or signed back in by a code;
 * and what they have watched (the owner, 2026-09-27).
 */
export function PersonPage(props: { readonly userId: number }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
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
          render={(value) => <PersonHead person={value} actions={actions} />}
        />
        {issued === undefined ? null : <IssuedInvitationView issued={issued} />}
        {problem === undefined ? null : <p role="alert">{problem}</p>}
        <h2>{t("person.devices")}</h2>
        <Loaded
          resource={devices}
          render={(list) => (
            <DeviceList
              devices={list}
              onRemove={actions.remove}
              onMakeCode={(device) => {
                actions.issueCode(device.name, { deviceId: device.id });
              }}
            />
          )}
        />
      </section>
    </BusyGroup>
  );
}

/** What the admin does to a person, one request at a time, and what it issued and why one failed. */
function usePersonActions(
  userId: number,
  reloads: { readonly reloadPerson: () => void; readonly reloadDevices: () => void },
): {
  readonly actions: {
    readonly rename: (name: string) => void;
    readonly giveRole: (role: string) => void;
    readonly issueLink: (name: string) => void;
    readonly issueCode: (
      name: string,
      device: { deviceName: string } | { deviceId: number },
    ) => void;
    readonly remove: (device: Device) => void;
  };
  readonly issued: IssuedInvitation | undefined;
  readonly isBusy: boolean;
  readonly problem: string | undefined;
} {
  const { t } = useTranslation();
  const { api } = useServices();
  const [issued, setIssued] = useState<IssuedInvitation | undefined>(undefined);
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    fieldProblemText(t, cause, personWords(t)),
  );
  const actions = {
    rename: (name: string): void => {
      runRequest(async () => {
        await renamePerson(api, userId, name);
        reloads.reloadPerson();
      });
    },
    giveRole: (role: string): void => {
      runRequest(async () => {
        await setPersonsRole(api, userId, role);
        reloads.reloadPerson();
      });
    },
    issueLink: (name: string): void => {
      runRequest(async (idempotencyKey) => {
        const invitation = await issueRegistrationInvitation(api, userId, idempotencyKey);
        setIssued({ kind: "link", name, invitation });
      });
    },
    issueCode: (name: string, device: { deviceName: string } | { deviceId: number }): void => {
      runRequest(async (idempotencyKey) => {
        const invitation = await issueDeviceCode(api, { target: device, userId }, idempotencyKey);
        setIssued({ kind: "code", name, invitation });
      });
    },
    remove: (device: Device): void => {
      runRequest(async () => {
        await removePersonDevice(api, userId, device.id);
        reloads.reloadDevices();
      });
    },
  };
  return { actions, issued, isBusy, problem };
}

/** A person's name, which can be changed, what they have watched, and what the admin can make. */
function PersonHead(props: {
  readonly person: Person;
  readonly actions: {
    readonly rename: (name: string) => void;
    readonly giveRole: (role: string) => void;
    readonly issueLink: (name: string) => void;
    readonly issueCode: (name: string, device: { deviceName: string }) => void;
  };
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
      <OneField
        label={t("people.name")}
        initialValue={account.name}
        submitLabel={t("person.rename")}
        onSubmit={props.actions.rename}
      />
      <RoleChoice current={account.role} onGive={props.actions.giveRole} />
      <button
        type="button"
        onClick={() => {
          props.actions.issueLink(account.name);
        }}
      >
        {t("people.newLink")}
      </button>
      <NewDeviceCodeForm
        onMakeCode={(deviceName) => {
          props.actions.issueCode(account.name, { deviceName });
        }}
        isMaking={false}
      />
    </>
  );
}

/** The person's role, their whole access, which the admin may change (D243 §4). */
function RoleChoice(props: {
  readonly current: string;
  readonly onGive: (role: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [roles] = useResource("person-roles", () => listRoles(api));
  const [role, setRole] = useState(props.current);
  const names =
    roles.state === "loaded" ? roles.value.roles.map((each) => each.name) : [props.current];
  return (
    <FormRow
      onSubmit={() => {
        props.onGive(role);
      }}
    >
      <label>
        {t("people.role")}{" "}
        <select
          value={role}
          onChange={(event) => {
            setRole(event.target.value);
          }}
        >
          {names.map((name) => (
            <option key={name} value={name}>
              {roleLabel(t, name)}
            </option>
          ))}
        </select>
      </label>
      <button type="submit">{t("person.changeRole")}</button>
    </FormRow>
  );
}

/** A form of one text field and its button. */
function OneField(props: {
  readonly label: string;
  readonly initialValue: string;
  readonly submitLabel: string;
  readonly onSubmit: (value: string) => void;
}): JSX.Element {
  const [value, setValue] = useState(props.initialValue);
  const submit = (): void => {
    props.onSubmit(value);
  };
  return (
    <FormRow onSubmit={submit}>
      <label>
        {props.label}{" "}
        <input
          required
          maxLength={100}
          value={value}
          onChange={(event) => {
            setValue(event.target.value);
          }}
        />
      </label>
      <button type="submit">{props.submitLabel}</button>
    </FormRow>
  );
}
