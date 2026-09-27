// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { useState, type JSX, type SyntheticEvent } from "react";
import { useTranslation } from "react-i18next";

import { ApiProblem, problemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { useResource } from "./use-resource.ts";

/** A key or password, as the server shows it: never its value (D194). */
export type Secret = components["schemas"]["SecretBody"];
/** A server setting, with its value and its starting value. */
export type Setting = components["schemas"]["SettingBody"];

/** A refusal, as a request's answer carries it (cut-a §5.2). */
type Refused = { readonly error: components["schemas"]["ErrorDetail"] } | undefined;

/** A setting's value as a person reads it. */
export function shown(value: unknown): string {
  if (typeof value === "string") {
    return value;
  }
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  return "";
}

/** The value a field's text means for a setting of this kind. */
export function parsed(kind: Setting["kind"], text: string): string | number | boolean {
  if (kind === "bool") {
    return text === "true";
  }
  return kind === "integer" ? Number(text) : text;
}

/** Whether a secret is set, in words: with its hint where it has one. */
function SecretState(props: { readonly secret: Secret }): JSX.Element {
  const { t } = useTranslation();
  const { secret } = props;
  if (!secret.isSet) {
    return <p>{t("settings.secretUnset")}</p>;
  }
  return (
    <p>
      {secret.hint === undefined
        ? t("settings.secretSetNoHint")
        : t("settings.secretSet", { hint: secret.hint })}
    </p>
  );
}

/** One secret: whether it is set, and setting or removing it. */
function SecretRow(props: {
  readonly secret: Secret;
  readonly note: string;
  readonly onSave: (value: string) => void;
  readonly onRemove: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { secret } = props;
  const [value, setValue] = useState("");
  const save = (event: SyntheticEvent): void => {
    event.preventDefault();
    props.onSave(value);
    setValue("");
  };
  return (
    <form onSubmit={save} aria-labelledby={`secret-${secret.name}`}>
      <h3 id={`secret-${secret.name}`}>{secret.name}</h3>
      <p>{secret.description}</p>
      <SecretState secret={secret} />
      {secret.needsReentry ? <p role="alert">{t("settings.secretReentry")}</p> : null}
      <label>
        {t("settings.newValue")}
        <input
          type="password"
          autoComplete="off"
          required
          value={value}
          onChange={(event) => {
            setValue(event.target.value);
          }}
        />
      </label>
      <button type="submit">{t("settings.save")}</button>{" "}
      {secret.isSet ? (
        <button type="button" onClick={props.onRemove}>
          {t("settings.remove")}
        </button>
      ) : null}
      {props.note === "" ? null : <output>{props.note}</output>}
    </form>
  );
}

/** The keys and passwords: each one set, replaced or removed, never shown. */
export function SecretsView(props: {
  readonly secrets: readonly Secret[];
  readonly notes: ReadonlyMap<string, string>;
  readonly onSave: (name: string, value: string) => void;
  readonly onRemove: (name: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <section aria-labelledby="secrets-heading">
      <h2 id="secrets-heading">{t("settings.secretsHeading")}</h2>
      <p>{t("settings.secretsIntro")}</p>
      {props.secrets.map((secret) => (
        <SecretRow
          key={secret.name}
          secret={secret}
          note={props.notes.get(secret.name) ?? ""}
          onSave={(value) => {
            props.onSave(secret.name, value);
          }}
          onRemove={() => {
            props.onRemove(secret.name);
          }}
        />
      ))}
    </section>
  );
}

/** One setting: its value, edited by its kind, and a way back to its starting value. */
function SettingRow(props: {
  readonly setting: Setting;
  readonly onSave: (value: string | number | boolean) => void;
  readonly onReset: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { setting } = props;
  const [text, setText] = useState(() => shown(setting.value));
  const save = (event: SyntheticEvent): void => {
    event.preventDefault();
    props.onSave(parsed(setting.kind, text));
  };
  return (
    <form onSubmit={save} aria-labelledby={`setting-${setting.key}`}>
      <h3 id={`setting-${setting.key}`}>
        <code>{setting.key}</code>
      </h3>
      <p>{setting.question ?? setting.description}</p>
      <label>
        {setting.kind === "bool" ? t("settings.on") : setting.key}
        {setting.kind === "bool" ? (
          <input
            type="checkbox"
            checked={text === "true"}
            onChange={(event) => {
              setText(String(event.target.checked));
            }}
          />
        ) : (
          <input
            type={setting.kind === "integer" ? "number" : "text"}
            min={setting.minimum}
            max={setting.maximum}
            value={text}
            onChange={(event) => {
              setText(event.target.value);
            }}
          />
        )}
      </label>
      <button type="submit">{t("settings.save")}</button>{" "}
      {setting.isModified ? (
        <>
          <span>{t("settings.changed", { start: shown(setting.startingValue) })}</span>{" "}
          <button type="button" onClick={props.onReset}>
            {t("settings.reset")}
          </button>
        </>
      ) : null}
    </form>
  );
}

/** Every server setting, beside its starting value (D200). */
export function SettingsView(props: {
  readonly settings: readonly Setting[];
  readonly onSave: (key: string, value: string | number | boolean) => void;
  readonly onReset: (key: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <section aria-labelledby="server-settings-heading">
      <h2 id="server-settings-heading">{t("settings.settingsHeading")}</h2>
      {props.settings.map((setting) => (
        <SettingRow
          key={`${setting.key}/${shown(setting.value)}`}
          setting={setting}
          onSave={(value) => {
            props.onSave(setting.key, value);
          }}
          onReset={() => {
            props.onReset(setting.key);
          }}
        />
      ))}
    </section>
  );
}

/** Settings, for an admin: the keys and passwords, and every server setting. */
export function Settings(): JSX.Element {
  const { t } = useTranslation();
  return (
    <section aria-labelledby="settings-heading">
      <h1 id="settings-heading">{t("settings.heading")}</h1>
      <Secrets />
      <ServerSettings />
    </section>
  );
}

/** The secrets, loaded, with saving and removing. */
function Secrets(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [notes, setNotes] = useState<ReadonlyMap<string, string>>(() => new Map());
  const [secrets, reload] = useResource("secrets", async () => {
    const { data, error } = await api.GET("/secrets");
    if (data === undefined) {
      throw new ApiProblem(error.error);
    }
    return data.secrets ?? [];
  });
  const answered =
    (name: string, done: string) =>
    ({ error }: { error?: Refused }): void => {
      setNotes((previous) =>
        new Map(previous).set(
          name,
          error === undefined ? done : problemText(t, new ApiProblem(error.error)),
        ),
      );
      reload();
    };
  return (
    <Loaded
      resource={secrets}
      render={(value) => (
        <SecretsView
          secrets={value}
          notes={notes}
          onSave={(name, secret) => {
            void api
              .PUT("/secrets/{name}", { params: { path: { name } }, body: { value: secret } })
              .then(answered(name, t("settings.saved")));
          }}
          onRemove={(name) => {
            void api
              .DELETE("/secrets/{name}", { params: { path: { name } } })
              .then(answered(name, t("settings.removed")));
          }}
        />
      )}
    />
  );
}

/** The server's settings, loaded, with saving and resetting. */
function ServerSettings(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [problem, setProblem] = useState("");
  const [config, reload] = useResource("config", async () => {
    const { data, error } = await api.GET("/config");
    if (data === undefined) {
      throw new ApiProblem(error.error);
    }
    return data.settings ?? [];
  });
  const answered = ({ error }: { error?: Refused }): void => {
    setProblem(error === undefined ? "" : problemText(t, new ApiProblem(error.error)));
    reload();
  };
  return (
    <>
      {problem === "" ? null : <p role="alert">{problem}</p>}
      <Loaded
        resource={config}
        render={(value) => (
          <SettingsView
            settings={value}
            onSave={(key, setting) => {
              void api.PUT("/config", { body: { values: { [key]: setting } } }).then(answered);
            }}
            onReset={(key) => {
              void api.POST("/config/{key}/reset", { params: { path: { key } } }).then(answered);
            }}
          />
        )}
      />
    </>
  );
}
