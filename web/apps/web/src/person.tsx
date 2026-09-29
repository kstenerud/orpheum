// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import {
  getPerson,
  issueDeviceCode,
  issueRegistrationLink,
  listPersonDevices,
  removePersonDevice,
  renamePerson,
  type Device,
  type Person,
} from "./accounts.ts";
import { FormRow } from "./form-row.tsx";
import { IssuedInvitationView, type IssuedInvitation } from "./people.tsx";
import { problemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { useResource } from "./use-resource.ts";
import { DeviceList } from "./your-devices.tsx";

/**
 * One person, for the admin: their name, which can be changed; a new registration link or a
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
  const [issued, setIssued] = useState<IssuedInvitation | undefined>(undefined);
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const showProblem = (cause: unknown): void => {
    setProblem(problemText(t, cause, { conflict: t("people.nameTaken") }));
  };
  const actions = {
    rename: (name: string): void => {
      renamePerson(api, props.userId, name).then(reloadPerson, showProblem);
    },
    issueLink: (name: string): void => {
      issueRegistrationLink(api, props.userId).then((invitation) => {
        setIssued({ kind: "link", name, invitation });
      }, showProblem);
    },
    issueCode: (name: string, device: { deviceName: string } | { deviceId: number }): void => {
      issueDeviceCode(api, device, props.userId).then((invitation) => {
        setIssued({ kind: "code", name, invitation });
      }, showProblem);
    },
    remove: (device: Device): void => {
      removePersonDevice(api, props.userId, device.id).then(reloadDevices, showProblem);
    },
  };
  return (
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
  );
}

/** A person's name, which can be changed, what they have watched, and what the admin can make. */
function PersonHead(props: {
  readonly person: Person;
  readonly actions: {
    readonly rename: (name: string) => void;
    readonly issueLink: (name: string) => void;
    readonly issueCode: (name: string, device: { deviceName: string }) => void;
  };
}): JSX.Element {
  const { t, i18n } = useTranslation();
  const { account, watchStats } = props.person;
  const hours = (watchStats.watchedMs / 3_600_000).toLocaleString(i18n.language, {
    maximumFractionDigits: 1,
  });
  const lastWatched =
    watchStats.lastWatchedAt === undefined
      ? t("person.neverWatched")
      : new Intl.DateTimeFormat(i18n.language, { dateStyle: "medium" }).format(
          new Date(watchStats.lastWatchedAt),
        );
  return (
    <>
      <h1>{account.name}</h1>
      <p>
        {t(account.hasPassword ? "people.registered" : "people.notRegistered")} ·{" "}
        {t("person.watched", { hours, titles: watchStats.titlesWatchedCount, last: lastWatched })}
      </p>
      <OneField
        label={t("people.name")}
        initialValue={account.name}
        submitLabel={t("person.rename")}
        onSubmit={props.actions.rename}
      />
      <button
        type="button"
        onClick={() => {
          props.actions.issueLink(account.name);
        }}
      >
        {t("people.newLink")}
      </button>
      <OneField
        label={t("account.deviceName")}
        initialValue=""
        placeholder={t("account.deviceNameExample")}
        submitLabel={t("account.makeCode")}
        onSubmit={(deviceName) => {
          props.actions.issueCode(account.name, { deviceName });
        }}
      />
    </>
  );
}

/** A form of one text field and its button. */
function OneField(props: {
  readonly label: string;
  readonly initialValue: string;
  readonly placeholder?: string;
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
          placeholder={props.placeholder}
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
