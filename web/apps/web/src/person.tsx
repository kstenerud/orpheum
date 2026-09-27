// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX, type SyntheticEvent } from "react";
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
import { MadeView, type Made } from "./people.tsx";
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
  const [made, setMade] = useState<Made | undefined>(undefined);
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const failed = (cause: unknown): void => {
    setProblem(problemText(t, cause, { conflict: t("people.nameTaken") }));
  };
  const act = {
    rename: (name: string): void => {
      renamePerson(api, props.userId, name).then(reloadPerson, failed);
    },
    link: (name: string): void => {
      issueRegistrationLink(api, props.userId).then((invitation) => {
        setMade({ what: "link", name, invitation });
      }, failed);
    },
    code: (name: string, device: { deviceName: string } | { deviceId: number }): void => {
      issueDeviceCode(api, device, props.userId).then((invitation) => {
        setMade({ what: "code", name, invitation });
      }, failed);
    },
    remove: (device: Device): void => {
      removePersonDevice(api, props.userId, device.id).then(reloadDevices, failed);
    },
  };
  return (
    <section className="person">
      <Loaded resource={person} render={(value) => <PersonHead person={value} act={act} />} />
      {made === undefined ? null : <MadeView made={made} />}
      {problem === undefined ? null : <p role="alert">{problem}</p>}
      <h2>{t("person.devices")}</h2>
      <Loaded
        resource={devices}
        render={(list) => (
          <DeviceList
            devices={list}
            onRemove={act.remove}
            onCodeFor={(device) => {
              act.code(device.name, { deviceId: device.id });
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
  readonly act: {
    readonly rename: (name: string) => void;
    readonly link: (name: string) => void;
    readonly code: (name: string, device: { deviceName: string }) => void;
  };
}): JSX.Element {
  const { t, i18n } = useTranslation();
  const { account, watched } = props.person;
  const hours = (watched.watchedMs / 3_600_000).toLocaleString(i18n.language, {
    maximumFractionDigits: 1,
  });
  const lastWatched =
    watched.lastWatchedAt === undefined
      ? t("person.neverWatched")
      : new Intl.DateTimeFormat(i18n.language, { dateStyle: "medium" }).format(
          new Date(watched.lastWatchedAt),
        );
  return (
    <>
      <h1>{account.name}</h1>
      <p>
        {t(account.hasPassword ? "people.registered" : "people.waiting")} ·{" "}
        {t("person.watched", { hours, titles: watched.titlesWatched, last: lastWatched })}
      </p>
      <OneField
        label={t("people.name")}
        start={account.name}
        submit={t("person.rename")}
        onSubmit={props.act.rename}
      />
      <button
        type="button"
        onClick={() => {
          props.act.link(account.name);
        }}
      >
        {t("people.newLink")}
      </button>
      <OneField
        label={t("account.deviceName")}
        start=""
        placeholder={t("account.deviceNameExample")}
        submit={t("account.makeCode")}
        onSubmit={(deviceName) => {
          props.act.code(account.name, { deviceName });
        }}
      />
    </>
  );
}

/** A form of one text field and its button. */
function OneField(props: {
  readonly label: string;
  readonly start: string;
  readonly placeholder?: string;
  readonly submit: string;
  readonly onSubmit: (value: string) => void;
}): JSX.Element {
  const [value, setValue] = useState(props.start);
  const submit = (event: SyntheticEvent): void => {
    event.preventDefault();
    props.onSubmit(value);
  };
  return (
    <form className="account__part" onSubmit={submit}>
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
      <button type="submit">{props.submit}</button>
    </form>
  );
}
