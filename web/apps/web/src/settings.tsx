// SPDX-License-Identifier: MPL-2.0

import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { BusyGroup } from "./busy-group.tsx";
import { useRequest } from "./use-request.ts";
import { idempotent } from "./idempotent.ts";
import { submitWith } from "./form-row.tsx";
import { dataOrThrow, fieldProblemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { useResource } from "./use-resource.ts";

/** A key or password, as the server shows it: never its value (D194). */
export type Secret = components["schemas"]["SecretBody"];
/** A server setting, with its value and its starting value. */
export type Setting = components["schemas"]["SettingBody"];

/** A request's answer, as the SDK gives it: a refusal carries the server's error. */
type RefusableAnswer = Parameters<typeof throwIfRefused>[0];

/** A setting's value as a person reads it. */
export function settingText(value: unknown): string {
  if (typeof value === "string") {
    return value;
  }
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  return "";
}

/** The value a field's text means for a setting of this kind. */
export function parseSettingText(kind: Setting["kind"], text: string): string | number | boolean {
  if (kind === "bool") {
    return text === "true";
  }
  return kind === "integer" ? Number(text) : text;
}

/** Whether a secret is set, in words: with its last characters where it has them. */
function SecretState(props: { readonly secret: Secret }): JSX.Element {
  const { t } = useTranslation();
  const { secret } = props;
  if (!secret.isSet) {
    return <p>{t("settings.secretUnset")}</p>;
  }
  return (
    <p>
      {secret.lastChars === undefined
        ? t("settings.secretSetNoLastChars")
        : t("settings.secretSet", { lastChars: secret.lastChars })}
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
  const save = submitWith(() => {
    props.onSave(value);
    setValue("");
  });
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
  /** Why the server refused this setting's last change, beside it; "" when it did not. */
  readonly problem: string;
  readonly onSave: (value: string | number | boolean) => void;
  readonly onReset: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { setting } = props;
  const [text, setText] = useState(() => settingText(setting.value));
  const save = submitWith(() => {
    props.onSave(parseSettingText(setting.kind, text));
  });
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
          <span>{t("settings.changed", { start: settingText(setting.startingValue) })}</span>{" "}
          <button type="button" onClick={props.onReset}>
            {t("settings.reset")}
          </button>
        </>
      ) : null}
      {props.problem === "" ? null : <p role="alert">{props.problem}</p>}
    </form>
  );
}

/** Every server setting, beside its starting value (D200), and a refusal beside its setting. */
export function SettingsView(props: {
  readonly settings: readonly Setting[];
  /** Why the server refused a setting's last change, by key. */
  readonly problems: ReadonlyMap<string, string>;
  readonly onSave: (key: string, value: string | number | boolean) => void;
  readonly onReset: (key: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <section aria-labelledby="server-settings-heading">
      <h2 id="server-settings-heading">{t("settings.settingsHeading")}</h2>
      {props.settings.map((setting) => (
        <SettingRow
          key={`${setting.key}/${settingText(setting.value)}`}
          setting={setting}
          problem={props.problems.get(setting.key) ?? ""}
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
    const secretsBody = dataOrThrow(await api.GET("/secrets"));
    return secretsBody.secrets ?? [];
  });
  // Each secret's own note says how its change went; the request only makes the buttons wait.
  const changing = useRequest(() => "");
  /** Shows how a change to a secret went, then loads the secrets again. */
  const noteOutcome = (
    name: string,
    successNote: string,
    send: (idempotencyKey: string) => Promise<RefusableAnswer>,
  ): void => {
    const showNote = (text: string): void => {
      setNotes((previous) => new Map(previous).set(name, text));
      reload();
    };
    changing.runRequest(async (idempotencyKey) => {
      try {
        throwIfRefused(await send(idempotencyKey));
        showNote(successNote);
      } catch (cause: unknown) {
        showNote(fieldProblemText(t, cause, { fields: { "body.value": t("settings.newValue") } }));
      }
    });
  };
  return (
    <BusyGroup isBusy={changing.isBusy}>
      <Loaded
        resource={secrets}
        render={(loadedSecrets) => (
          <SecretsView
            secrets={loadedSecrets}
            notes={notes}
            onSave={(name, value) => {
              noteOutcome(name, t("settings.saved"), () =>
                api.PUT("/secrets/{name}", { params: { path: { name } }, body: { value } }),
              );
            }}
            onRemove={(name) => {
              noteOutcome(name, t("settings.removed"), () =>
                api.DELETE("/secrets/{name}", { params: { path: { name } } }),
              );
            }}
          />
        )}
      />
    </BusyGroup>
  );
}

/** The server's settings, loaded, with saving and resetting; a refusal is shown beside its setting. */
function ServerSettings(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [problems, setProblems] = useState<ReadonlyMap<string, string>>(() => new Map());
  const [settings, reload] = useResource("config", async () => {
    const configBody = dataOrThrow(await api.GET("/config"));
    return configBody.settings ?? [];
  });
  // Each setting's own problem says how its change went; the request only makes the buttons wait.
  const changing = useRequest(() => "");
  /** Shows a change's refusal beside its setting, or clears it, then loads the settings again. */
  const showRefusalAndReload = (
    key: string,
    send: (idempotencyKey: string) => Promise<RefusableAnswer>,
  ): void => {
    const showProblem = (text: string): void => {
      setProblems((previous) => new Map(previous).set(key, text));
      reload();
    };
    changing.runRequest(async (idempotencyKey) => {
      try {
        throwIfRefused(await send(idempotencyKey));
        showProblem("");
      } catch (cause: unknown) {
        showProblem(fieldProblemText(t, cause, { fields: { [`body.valuesByKey.${key}`]: key } }));
      }
    });
  };
  return (
    <BusyGroup isBusy={changing.isBusy}>
      <Loaded
        resource={settings}
        render={(loadedSettings) => (
          <SettingsView
            settings={loadedSettings}
            problems={problems}
            onSave={(key, newValue) => {
              showRefusalAndReload(key, () =>
                api.PUT("/config", { body: { valuesByKey: { [key]: newValue } } }),
              );
            }}
            onReset={(key) => {
              showRefusalAndReload(key, (idempotencyKey) =>
                api.POST("/config/{key}/reset", {
                  params: { path: { key } },
                  ...idempotent(idempotencyKey),
                }),
              );
            }}
          />
        )}
      />
    </BusyGroup>
  );
}
