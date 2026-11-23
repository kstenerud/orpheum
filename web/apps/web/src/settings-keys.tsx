// SPDX-License-Identifier: MPL-2.0

import "./settings.css";
import "./settings-words.ts";

import type { components } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { memo, useCallback, useEffect, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { camelKey } from "./catalogue-key.ts";
import { ConfirmDialog } from "./confirm-button.tsx";
import { arrivalMark } from "./focus-navigation.ts";
import { PasswordField } from "./password-field.tsx";
import { dataOrThrow, fieldProblemText, throwIfRefused } from "./problem.ts";
import { settingLabel, withMember } from "./server-settings.tsx";
import { useServices } from "./services.tsx";
import { SettingInfo } from "./setting-info.tsx";
import { shownSaveTold, StatusNote } from "./status-note.tsx";
import { useResource, type Resource } from "./use-resource.ts";

/** A key or password, as the server shows it: never its value (D194). */
export type Secret = components["schemas"]["SecretBody"];

/** What a secret is for, in the person's words: the catalogue's, or the server's for one it lacks. */
export function secretDescription(t: TFunction, secret: Secret): string {
  return t(`settings.secretDescriptions.${camelKey(secret.name)}`, {
    defaultValue: secret.description,
  });
}

/** Whether a secret is set, in words: with its last characters where it has them, an API key's. */
export function secretState(t: TFunction, secret: Secret): string {
  if (!secret.isSet) {
    return t("settings.secretUnset");
  }
  return secret.lastChars === undefined
    ? t("settings.secretSetNoLastChars")
    : t("settings.secretSet", { lastChars: secret.lastChars });
}

/** The keys and passwords, in the server's order, as the tab last read them. */
export function useSecrets(): readonly [Resource<readonly Secret[]>, () => void] {
  const { api } = useServices();
  return useResource("secrets", async () => {
    const secretsBody = dataOrThrow(await api.GET("/secrets"));
    return secretsBody.secrets ?? [];
  });
}

/**
 * A secret's field and what saves it: what is typed is sent on Enter or as focus leaves the field and
 * its Show password button, and the field is emptied, never showing it again. Nothing is sent as the
 * page closes or the person moves to another (D538 §9).
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
  return { text: fieldText, setText, commit };
}

/**
 * Remove…, while a key is set: the irreversible confirm (ui.md rule 8), its focus on Cancel, given
 * back to Remove… as it closes.
 */
function RemoveKey(props: { readonly label: string; readonly onRemove: () => void }): JSX.Element {
  const { t } = useTranslation();
  const [isAsking, setIsAsking] = useState(false);
  return (
    <>
      <button
        type="button"
        aria-haspopup="dialog"
        onClick={() => {
          setIsAsking(true);
        }}
      >
        {t("settings.removeKey")}
      </button>
      {isAsking ? (
        <ConfirmDialog
          question={t("settings.removeQuestion", { key: props.label })}
          consequence={t("settings.removeConsequence")}
          confirmLabel={t("settings.removeConfirm")}
          onConfirm={props.onRemove}
          onClose={() => {
            setIsAsking(false);
          }}
        />
      ) : null}
    </>
  );
}

/**
 * A key's field for a new value, sent on Enter or as focus leaves it; leaving the field for its own
 * Show password button is not leaving it.
 */
function KeyField(props: {
  readonly label: string;
  readonly text: string;
  readonly onText: (text: string) => void;
  readonly onCommit: () => void;
}): JSX.Element {
  const formRef = useRef<HTMLFormElement>(null);
  const { onCommit } = props;
  useEffect(() => {
    const form = formRef.current;
    if (form === null) {
      return undefined;
    }
    const left = (event: FocusEvent): void => {
      const toward = event.relatedTarget;
      if (!(toward instanceof Node && form.contains(toward))) {
        onCommit();
      }
    };
    form.addEventListener("focusout", left);
    return () => {
      form.removeEventListener("focusout", left);
    };
  }, [onCommit]);
  return (
    <form
      ref={formRef}
      className="settings__text"
      noValidate
      onSubmit={(event) => {
        event.preventDefault();
        onCommit();
      }}
    >
      <PasswordField
        label={props.label}
        value={props.text}
        onChange={props.onText}
        autoComplete="new-password"
      />
    </form>
  );
}

/**
 * One key's row (pages-spec-3): its label and whether it is set, an API key's last four; a field for
 * a new value, never showing what is set; why it must be entered again; how its last change went;
 * its info button, with no default; and Remove… while it is set.
 */
const KeyRow = memo(function KeyRow(props: {
  readonly secret: Secret;
  /** Why the server refused this key's last change; "" when it did not. */
  readonly problem: string;
  /** Whether its last change was saved, so it says so. */
  readonly isSaved: boolean;
  readonly isArrival: boolean;
  readonly onSave: (name: string, value: string) => void;
  readonly onRemove: (name: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { secret, onRemove } = props;
  const { text, setText, commit } = useSecretField(secret.name, props.onSave);
  const label = settingLabel(t, "secretLabels", secret.name);
  const headingId = `secret-${secret.name}`;
  return (
    <section
      className="settings__row"
      aria-labelledby={headingId}
      {...(props.isArrival ? arrivalMark : {})}
    >
      <h2 className="settings__key-name" id={headingId}>
        {label} <span className="settings__secret-state">{secretState(t, secret)}</span>
      </h2>
      {secret.needsReentry ? (
        <StatusNote told={{ kind: "problem", text: t("settings.secretReentry") }} />
      ) : null}
      <div className="settings__field">
        <KeyField label={t("settings.newValue")} text={text} onText={setText} onCommit={commit} />
        <SettingInfo
          label={label}
          configName={secret.name}
          description={secretDescription(t, secret)}
        />
      </div>
      <StatusNote told={shownSaveTold(t, props.problem, props.isSaved)} />
      {secret.isSet ? (
        <RemoveKey
          label={label}
          onRemove={() => {
            onRemove(secret.name);
          }}
        />
      ) : null}
    </section>
  );
});

/** The keys and passwords, each set, replaced or removed, never shown, in the server's order. */
export function KeysView(props: {
  readonly secrets: readonly Secret[];
  /** The key the page's address names, where focus arrives. */
  readonly arrival?: string | undefined;
  /** Why the server refused a key's last change, by name. */
  readonly problems: ReadonlyMap<string, string>;
  /** The keys whose last change was saved, which say so. */
  readonly saved?: ReadonlySet<string>;
  readonly onSave: (name: string, value: string) => void;
  readonly onRemove: (name: string) => void;
}): JSX.Element {
  return (
    <>
      {props.secrets.map((secret) => (
        <KeyRow
          key={secret.name}
          secret={secret}
          problem={props.problems.get(secret.name) ?? ""}
          isSaved={props.saved?.has(secret.name) ?? false}
          isArrival={secret.name === props.arrival}
          onSave={props.onSave}
          onRemove={props.onRemove}
        />
      ))}
    </>
  );
}

/** What saves and removes each key, with each one's refusal or "Saved", then reads them again. */
export function useKeyChanges(reload: () => void): {
  readonly problems: ReadonlyMap<string, string>;
  readonly saved: ReadonlySet<string>;
  readonly save: (name: string, value: string) => void;
  readonly remove: (name: string) => void;
} {
  const { t } = useTranslation();
  const { api } = useServices();
  const [problems, setProblems] = useState<ReadonlyMap<string, string>>(() => new Map());
  const [saved, setSaved] = useState<ReadonlySet<string>>(() => new Set());
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
      void settle(name, () =>
        api.PUT("/secrets/{name}", { params: { path: { name } }, body: { value } }),
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
  return { problems, saved, save, remove };
}
