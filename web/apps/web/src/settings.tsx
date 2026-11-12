// SPDX-License-Identifier: MPL-2.0

import "./settings.css";

import type { components } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { memo, useCallback, useEffect, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { camelKey } from "./catalogue-key.ts";
import { ConfirmQuestion } from "./confirm-button.tsx";
import { ResetIcon } from "./icons.tsx";
import { fieldProblemText, dataOrThrow, throwIfRefused } from "./problem.ts";
import { Link } from "./link.tsx";
import { SaveNote, shownSaveState } from "./save-note.tsx";
import {
  AboutButton,
  isFound,
  RowName,
  ServerSettings,
  settingLabel,
  SettingsFilters,
  withMember,
} from "./server-settings.tsx";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { UpdateSettings } from "./update-page.tsx";
import { useResource } from "./use-resource.ts";

/** A key or password, as the server shows it: never its value (D194). */
export type Secret = components["schemas"]["SecretBody"];

/** What a secret is for, in the person's words: the catalogue's, or the server's for one it lacks. */
function secretDescription(t: TFunction, secret: Secret): string {
  return t(`settings.secretDescriptions.${camelKey(secret.name)}`, {
    defaultValue: secret.description,
  });
}

/** Whether a secret is set, in words: with its last characters where it has them. */
export function secretState(t: TFunction, secret: Secret): string {
  if (!secret.isSet) {
    return t("settings.secretUnset");
  }
  return secret.lastChars === undefined
    ? t("settings.secretSetNoLastChars")
    : t("settings.secretSet", { lastChars: secret.lastChars });
}

/**
 * A secret's field and what saves it: what is typed is sent on Enter or on leaving the field, the
 * page included (a move to another screen unmounts the row; a reload or a closed tab only hides the
 * page), and the field is emptied, never showing it again.
 */
function useSecretField(
  name: string,
  onSave: (name: string, value: string) => void,
): {
  readonly text: string;
  readonly setText: (text: string) => void;
  readonly commit: () => void;
} {
  const [fieldText, setFieldText] = useState("");
  const typedRef = useRef("");
  const setText = useCallback((next: string): void => {
    typedRef.current = next;
    setFieldText(next);
  }, []);
  const commit = useCallback((): void => {
    if (typedRef.current === "") {
      return;
    }
    onSave(name, typedRef.current);
    typedRef.current = "";
    setFieldText("");
  }, [name, onSave]);
  useEffect(() => {
    const onPageHide = (): void => {
      commit();
    };
    window.addEventListener("pagehide", onPageHide);
    return () => {
      window.removeEventListener("pagehide", onPageHide);
      commit();
    };
  }, [commit]);
  return { text: fieldText, setText, commit };
}

/**
 * The bin's question before a secret is removed, which cannot be undone (D278 §4), drawn below the
 * row while the bin that asked stays drawn, disabled.
 */
function RemoveQuestion(props: {
  readonly onRemove: () => void;
  readonly onKeep: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <p className="settings__question">
      <ConfirmQuestion
        consequence={t("settings.removeConsequence")}
        confirmLabel={t("settings.removeConfirm")}
        onKeep={props.onKeep}
        onConfirm={props.onRemove}
      />
    </p>
  );
}

/**
 * A secret's buttons: what it is for, and a bin that asks before removing it while it is set.
 */
function SecretControls(props: {
  readonly secret: Secret;
  readonly isAsking: boolean;
  readonly onAsk: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { name, isSet } = props.secret;
  const remove = t("settings.removeSecret", { key: name });
  return (
    <span className="settings__controls">
      <AboutButton name={name} description={secretDescription(t, props.secret)} />
      <button
        type="button"
        className="settings__control"
        disabled={!isSet || props.isAsking}
        aria-label={remove}
        data-tip={isSet ? t("settings.tipRemove") : t("settings.tipNotSet")}
        onClick={props.onAsk}
      >
        <ResetIcon />
      </button>
    </span>
  );
}

/**
 * What a secret's row says below it: the bin's question, why it must be entered again, and how its
 * last change went: "Saved", or why it was refused.
 */
function SecretNotes(props: {
  readonly isAsking: boolean;
  readonly needsReentry: boolean;
  /** Why the server refused the secret's last change; "" when it did not. */
  readonly problem: string;
  readonly isSaved: boolean;
  readonly onRemove: () => void;
  readonly onKeep: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <>
      {props.isAsking ? <RemoveQuestion onRemove={props.onRemove} onKeep={props.onKeep} /> : null}
      {props.needsReentry ? (
        <p className="settings__problem" role="alert">
          {t("settings.secretReentry")}
        </p>
      ) : null}
      <SaveNote state={shownSaveState(props.problem, props.isSaved)} />
    </>
  );
}

/** A secret's field: what is typed, never what is set, which its placeholder says is. */
function SecretField(props: {
  readonly id: string;
  readonly name: string;
  readonly placeholder: string;
  readonly text: string;
  readonly onText: (text: string) => void;
  readonly onCommit: () => void;
}): JSX.Element {
  return (
    <input
      id={props.id}
      className="settings__value"
      type="password"
      autoComplete="off"
      aria-labelledby={`secret-${props.name}`}
      placeholder={props.placeholder}
      value={props.text}
      onChange={(event) => {
        props.onText(event.target.value);
      }}
      onBlur={props.onCommit}
    />
  );
}

/**
 * One secret's row, in one column as a setting's: its name, its key under it, a field to type a new
 * one in, which says whether one is set, what it is for, and a bin that removes it once asked again.
 */
const SecretRow = memo(function SecretRow(props: {
  readonly secret: Secret;
  /** Why the server refused this secret's last change; "" when it did not. */
  readonly problem: string;
  /** Whether its last change was saved, so it says so. */
  readonly isSaved: boolean;
  readonly onSave: (name: string, value: string) => void;
  readonly onRemove: (name: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { secret } = props;
  const { text, setText, commit } = useSecretField(secret.name, props.onSave);
  const [isAsking, setIsAsking] = useState(false);
  const fieldId = `secret-field-${secret.name}`;
  return (
    <form
      className="settings__row"
      aria-labelledby={`secret-${secret.name}`}
      onSubmit={(event) => {
        event.preventDefault();
        commit();
      }}
    >
      <RowName
        id={`secret-${secret.name}`}
        fieldId={fieldId}
        label={settingLabel(t, "secretLabels", secret.name)}
        settingKey={secret.name}
      />
      <span className="settings__field">
        <SecretField
          id={fieldId}
          name={secret.name}
          placeholder={secretState(t, secret)}
          text={text}
          onText={setText}
          onCommit={commit}
        />
        <SecretControls
          secret={secret}
          isAsking={isAsking}
          onAsk={() => {
            setIsAsking(true);
          }}
        />
      </span>
      <SecretNotes
        isAsking={isAsking}
        needsReentry={secret.needsReentry}
        problem={props.problem}
        isSaved={props.isSaved}
        onRemove={() => {
          setIsAsking(false);
          props.onRemove(secret.name);
        }}
        onKeep={() => {
          setIsAsking(false);
        }}
      />
    </form>
  );
});

/** The keys and passwords, each set, replaced or removed, never shown, found by the search. */
export function SecretsView(props: {
  readonly secrets: readonly Secret[];
  /** What the person searched for: a secret shows when it has every word. */
  readonly search: string;
  /** Why the server refused a secret's last change, by name. */
  readonly problems: ReadonlyMap<string, string>;
  /** The secrets whose last change was saved, which say so. */
  readonly saved?: ReadonlySet<string>;
  readonly onSave: (name: string, value: string) => void;
  readonly onRemove: (name: string) => void;
}): JSX.Element | null {
  const { t } = useTranslation();
  const shown = props.secrets
    .filter((secret) => isFound(props.search, secret.name, secretDescription(t, secret)))
    .sort((first, second) => first.name.localeCompare(second.name));
  if (shown.length === 0) {
    return null;
  }
  return (
    <section aria-labelledby="secrets-heading">
      <h2 className="settings__section" id="secrets-heading">
        {t("settings.secretsHeading")}
      </h2>
      {shown.map((secret) => (
        <SecretRow
          key={secret.name}
          secret={secret}
          problem={props.problems.get(secret.name) ?? ""}
          isSaved={props.saved?.has(secret.name) ?? false}
          onSave={props.onSave}
          onRemove={props.onRemove}
        />
      ))}
    </section>
  );
}

/**
 * Settings, for an admin: one list of rows by section, the keys and passwords first, then the
 * server's settings; the important ones, or with Advanced every one, found by the search.
 */
export function Settings(): JSX.Element {
  const { t } = useTranslation();
  // Unticked at every visit: the page opens on the few most households set.
  const [isAdvanced, setIsAdvanced] = useState(false);
  const [search, setSearch] = useState("");
  return (
    <section className="settings" aria-labelledby="settings-heading">
      <h1 id="settings-heading">{t("settings.heading")}</h1>
      <p>
        {t("settings.intro")} <Link to={{ screen: "update" }}>{t("update.howTo")}</Link>
      </p>
      <SettingsFilters
        isAdvanced={isAdvanced}
        onAdvanced={setIsAdvanced}
        search={search}
        onSearch={setSearch}
      />
      <UpdateSettings />
      <Secrets search={search} />
      <ServerSettings isAdvanced={isAdvanced} search={search} />
    </section>
  );
}

/** The secrets, loaded, with saving and removing; a refusal is shown beside its secret. */
function Secrets(props: { readonly search: string }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [problems, setProblems] = useState<ReadonlyMap<string, string>>(() => new Map());
  const [saved, setSaved] = useState<ReadonlySet<string>>(() => new Set());
  const [secrets, reload] = useResource("secrets", async () => {
    const secretsBody = dataOrThrow(await api.GET("/secrets"));
    return secretsBody.secrets ?? [];
  });
  /** Shows a change's refusal beside its secret, or clears it, then loads the secrets again. */
  const settle = useCallback(
    async (name: string, send: () => Promise<Parameters<typeof throwIfRefused>[0]>) => {
      let problem = "";
      try {
        throwIfRefused(await send());
      } catch (cause: unknown) {
        problem = fieldProblemText(t, cause, { fields: { "body.value": name } });
      }
      setProblems((previous) => new Map(previous).set(name, problem));
      setSaved((previous) => withMember(previous, name, problem === ""));
      reload();
    },
    [t, reload],
  );
  const save = useCallback(
    (name: string, value: string): void => {
      // Kept alive, so a save the page's leaving sends outlives the page.
      void settle(name, () =>
        api.PUT("/secrets/{name}", {
          params: { path: { name } },
          body: { value },
          keepalive: true,
        }),
      );
    },
    [api, settle],
  );
  const remove = useCallback(
    (name: string): void => {
      void settle(name, () => api.DELETE("/secrets/{name}", { params: { path: { name } } }));
    },
    [api, settle],
  );
  return (
    <Loaded
      resource={secrets}
      render={(loaded) => (
        <SecretsView
          secrets={loaded}
          search={props.search}
          problems={problems}
          saved={saved}
          onSave={save}
          onRemove={remove}
        />
      )}
    />
  );
}
