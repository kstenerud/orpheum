// SPDX-License-Identifier: MPL-2.0

import "./settings.css";
import "./settings-words.ts";

import type { components } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { memo, useCallback, useEffect, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { camelKey } from "./catalogue-key.ts";
import { Field } from "./field.tsx";
import { arrivalMark } from "./focus-navigation.ts";
import { idempotent, newIdempotencyKey } from "./idempotent.ts";
import { dataOrThrow, fieldProblemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import type { SettingsSection } from "./settings-pages.ts";
import { SettingInfo } from "./setting-info.tsx";
import { shownSaveTold, StatusNote } from "./status-note.tsx";
import { Stepper } from "./stepper.tsx";
import { Switch } from "./switch.tsx";
import { useResource, type Resource } from "./use-resource.ts";

/** A server setting, with its value and its starting value. */
export type Setting = components["schemas"]["SettingBody"];
/** The part of Settings a setting is listed in. */
type Section = SettingsSection;
/** A setting's value, by its kind. */
export type SettingValue = string | number | boolean;

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
export function parseSettingText(kind: Setting["kind"], text: string): SettingValue {
  if (kind === "bool") {
    return text === "true";
  }
  return kind === "integer" ? Number(text) : text;
}

/**
 * A setting's or a key's name in the person's words, the label of its control: the catalogue's, or
 * its key where the catalogue has none (ui-audit-2026-11-04.md §3, Settings).
 */
export function settingLabel(
  t: TFunction,
  catalogue: "labels" | "secretLabels",
  key: string,
): string {
  return t(`settings.${catalogue}.${camelKey(key)}`, { defaultValue: key });
}

/** What a setting is for, in the person's words: the catalogue's, or the server's for one it lacks. */
export function settingDescription(t: TFunction, setting: Setting): string {
  return t(`settings.descriptions.${camelKey(setting.key)}`, { defaultValue: setting.description });
}

/**
 * Whether a setting or key is found by every word of a search, in any of its texts: its label, its
 * key or its description.
 */
export function isFound(search: string, texts: readonly string[]): boolean {
  const words = search
    .toLowerCase()
    .split(/\s+/u)
    .filter((word) => word !== "");
  const haystack = texts.join(" ").toLowerCase();
  return words.every((word) => haystack.includes(word));
}

/**
 * A setting's field and what saves it: the text in the field, and commit, which saves it when it
 * differs from the server's. The server's value, saved or reset, becomes the field's. Nothing saves
 * as the page closes or the person moves to another (D538 §9): what is typed and left unsaved is
 * dropped, as a form's is.
 */
function useSavedField(
  setting: Setting,
  onSave: (key: string, value: SettingValue) => void,
): {
  readonly text: string;
  readonly setText: (text: string) => void;
  readonly commit: (text?: string) => void;
} {
  const valueText = settingText(setting.value);
  const [fieldText, setFieldText] = useState(valueText);
  const [shownValue, setShownValue] = useState(valueText);
  if (shownValue !== valueText) {
    setShownValue(valueText);
    setFieldText(valueText);
  }
  // What the field holds and what the server holds, for the save the field's commit makes.
  const typedRef = useRef(valueText);
  const savedRef = useRef(valueText);
  useEffect(() => {
    typedRef.current = valueText;
    savedRef.current = valueText;
  }, [valueText]);
  const setText = useCallback((next: string): void => {
    typedRef.current = next;
    setFieldText(next);
  }, []);
  const { key, kind } = setting;
  const commit = useCallback(
    (committed?: string): void => {
      const toSave = committed ?? typedRef.current;
      if (toSave !== savedRef.current) {
        savedRef.current = toSave;
        onSave(key, parseSettingText(kind, toSave));
      }
    },
    [onSave, key, kind],
  );
  return { text: fieldText, setText, commit };
}

/**
 * The most steps of one a whole number is a Stepper over: past it, typing the number is quicker than
 * stepping to it, and it is a number field (the owner, 2026-11-22: "a Stepper only where stepping it
 * makes sense"). A week in hours, 168, is stepped, as a library's hours between checks are.
 */
const mostSteps = 200;

/** A whole-number setting with both its bounds, which the server sends where it has them. */
type RangedSetting = Setting & { readonly minimum: number; readonly maximum: number };

/**
 * Whether a setting is a Stepper: a whole number whose bounds the server sends, no more than
 * mostSteps apart, its step one (the server sends none).
 */
export function isStepped(setting: Setting): setting is RangedSetting {
  return (
    setting.kind === "integer" &&
    setting.minimum !== undefined &&
    setting.maximum !== undefined &&
    setting.maximum - setting.minimum <= mostSteps
  );
}

/** An on-or-off setting's Switch, saving at once; a question not yet answered says so (D184). */
function SettingSwitch(props: {
  readonly setting: Setting;
  readonly label: string;
  readonly onSave: (key: string, value: SettingValue) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { setting } = props;
  return (
    <Switch
      label={props.label}
      status={setting.value === null ? t("settings.unanswered") : undefined}
      isOn={setting.value === true}
      onSwitch={(isOn) => {
        props.onSave(setting.key, isOn);
      }}
    />
  );
}

/** A narrow whole number's Stepper (isStepped), saving at each step and at a number typed. */
function SettingStepper(props: {
  readonly setting: RangedSetting;
  readonly value: number;
  readonly label: string;
  readonly onSave: (key: string, value: SettingValue) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { setting, label } = props;
  return (
    <Stepper
      label={label}
      fieldLabel={t("settings.stepperField", {
        label,
        min: setting.minimum,
        max: setting.maximum,
      })}
      valueText={t("settings.stepperValue", { label, value: props.value })}
      value={props.value}
      min={setting.minimum}
      max={setting.maximum}
      step={1}
      // The label carries the unit, "(hours)", in the catalogue's words.
      unit=""
      stepDownLabel={t("settings.stepperLess", { label })}
      stepUpLabel={t("settings.stepperMore", { label })}
      onChange={(value) => {
        props.onSave(setting.key, value);
      }}
    />
  );
}

/**
 * A text or wide whole number's Field: typing is saved on Enter or on leaving the field; a number's
 * arrows, which no one types, as they are let go. Its bounds, where the server sends them, are its
 * hint. Nothing is saved as the page closes (D538 §9).
 */
function SettingTextField(props: {
  readonly setting: Setting;
  readonly label: string;
  readonly onSave: (key: string, value: SettingValue) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { setting } = props;
  const { text, setText, commit } = useSavedField(setting, props.onSave);
  const isNumber = setting.kind === "integer";
  const hint =
    isNumber && setting.minimum !== undefined && setting.maximum !== undefined
      ? t("settings.range", { min: setting.minimum, max: setting.maximum })
      : undefined;
  return (
    <form
      className="settings__text"
      noValidate
      onSubmit={(event) => {
        event.preventDefault();
        commit();
      }}
    >
      <Field label={props.label} hint={hint}>
        {(control) => (
          <input
            {...control}
            className="settings__value"
            type={isNumber ? "number" : "text"}
            inputMode={isNumber ? "numeric" : undefined}
            min={setting.minimum}
            max={setting.maximum}
            value={text}
            onChange={(event) => {
              setText(event.target.value);
            }}
            onBlur={() => {
              commit();
            }}
            onPointerUp={() => {
              commit();
            }}
          />
        )}
      </Field>
    </form>
  );
}

/**
 * A setting's control, by its kind: a Switch, a Stepper, or a Field. The UI gallery draws a wide
 * whole number's Field with it, as Settings does (ui-gallery-forms.tsx).
 */
export function SettingControl(props: {
  readonly setting: Setting;
  readonly label: string;
  readonly onSave: (key: string, value: SettingValue) => void;
}): JSX.Element {
  const { setting } = props;
  if (setting.kind === "bool") {
    return <SettingSwitch {...props} />;
  }
  if (isStepped(setting) && typeof setting.value === "number") {
    return <SettingStepper {...props} setting={setting} value={setting.value} />;
  }
  return <SettingTextField {...props} />;
}

/**
 * A starting value as a person reads it: on or off for a choice, nothing for empty text, and
 * unanswered for a question (D184), which empty text would not say.
 */
export function startText(t: TFunction, value: unknown): string {
  if (value === null) {
    return t("settings.unanswered");
  }
  if (typeof value === "boolean") {
    return value ? t("settings.on") : t("settings.off");
  }
  return value === "" ? t("settings.nothing") : settingText(value);
}

/**
 * One setting's row (D538 §1, pages-spec-3): its control, labelled in plain words, the raw key left
 * to What it is for; its info button; then how its last change went, "Saved" or the refusal. A
 * change is saved without a button (the owner, 2026-11-04, Q5; 2026-11-22): a switch and a stepper
 * as they change, typing on Enter or on leaving the field. The row the page's address names is where
 * focus arrives.
 */
const SettingRow = memo(function SettingRow(props: {
  readonly setting: Setting;
  /** Why the server refused this setting's last change; "" when it did not. */
  readonly problem: string;
  /** Whether its last change was saved, so it says so. */
  readonly isSaved: boolean;
  readonly isArrival: boolean;
  readonly onSave: (key: string, value: SettingValue) => void;
  readonly onReset: (key: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { setting, onReset } = props;
  const label = settingLabel(t, "labels", setting.key);
  return (
    <div
      className="settings__row"
      id={`setting-${setting.key}`}
      {...(props.isArrival ? arrivalMark : {})}
    >
      <div className="settings__field">
        <SettingControl setting={setting} label={label} onSave={props.onSave} />
        <SettingInfo
          label={label}
          configName={setting.key}
          description={setting.question ?? settingDescription(t, setting)}
          default={{
            text: startText(t, setting.startingValue),
            isModified: setting.isModified,
            onUse: () => {
              onReset(setting.key);
            },
          }}
        />
      </div>
      <StatusNote told={shownSaveTold(t, props.problem, props.isSaved)} />
    </div>
  );
});

/**
 * The keys of the remote access settings whose home is Network, which sets the address in the home
 * by its own request (pages-spec-3): the section's page leads there in their place.
 */
export const networkSettingKeys: ReadonlySet<string> = new Set([
  "addressAtHome",
  "addressAtHomeLearnedFrom",
]);

/** The settings a section's page shows: its own, the important ones or with advanced every one. */
export function sectionSettings(
  settings: readonly Setting[],
  section: Section,
  isAdvanced: boolean,
): readonly Setting[] {
  return [...settings]
    .filter(
      (setting) =>
        setting.section === section &&
        (isAdvanced || setting.isImportant) &&
        !networkSettingKeys.has(setting.key),
    )
    .sort((first, second) => first.key.localeCompare(second.key));
}

/**
 * One section's settings, one to a row (D200, D538 §11): the important ones, or with advanced every
 * one; with none to show, that every one here is advanced.
 */
export function SectionSettingsView(props: {
  readonly settings: readonly Setting[];
  readonly section: Section;
  /** Whether every setting shows, not only the important ones ("Show advanced settings"). */
  readonly isAdvanced: boolean;
  /** The setting the page's address names, where focus arrives. */
  readonly arrival?: string | undefined;
  /** Why the server refused a setting's last change, by key. */
  readonly problems: ReadonlyMap<string, string>;
  /** The settings whose last change was saved, which say so. */
  readonly saved?: ReadonlySet<string>;
  readonly onSave: (key: string, value: SettingValue) => void;
  readonly onReset: (key: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const shown = sectionSettings(props.settings, props.section, props.isAdvanced);
  return (
    <>
      {shown.length === 0 ? <p>{t("settings.allAdvanced")}</p> : null}
      {shown.map((setting) => (
        <SettingRow
          key={setting.key}
          setting={setting}
          problem={props.problems.get(setting.key) ?? ""}
          isSaved={props.saved?.has(setting.key) ?? false}
          isArrival={setting.key === props.arrival}
          onSave={props.onSave}
          onReset={props.onReset}
        />
      ))}
    </>
  );
}

/** A set with value added, or taken out. */
export function withMember(
  set: ReadonlySet<string>,
  value: string,
  isMember: boolean,
): ReadonlySet<string> {
  const next = new Set(set);
  if (isMember) {
    next.add(value);
  } else {
    next.delete(value);
  }
  return next;
}

/**
 * The settings as loaded, with what saves and resets each on its own: only the setting a change is
 * about takes the server's answer, so no other row is drawn again, nor a field anyone types in.
 */
export function useSettingChanges(loaded: readonly Setting[]): {
  readonly settings: readonly Setting[];
  readonly problems: ReadonlyMap<string, string>;
  readonly saved: ReadonlySet<string>;
  readonly resetting: ReadonlySet<string>;
  readonly save: (key: string, value: SettingValue) => void;
  readonly reset: (key: string) => void;
} {
  const { t } = useTranslation();
  const { api } = useServices();
  const [settings, setSettings] = useState(loaded);
  const [problems, setProblems] = useState<ReadonlyMap<string, string>>(() => new Map());
  const [resetting, setResetting] = useState<ReadonlySet<string>>(() => new Set());
  const [saved, setSaved] = useState<ReadonlySet<string>>(() => new Set());
  /** Takes the server's answer for one setting, or why it refused, beside it; it says it saved. */
  const settle = useCallback(
    async (key: string, send: () => Promise<Setting | undefined>): Promise<void> => {
      let problem = "";
      try {
        const answered = await send();
        if (answered !== undefined) {
          setSettings((previous) => previous.map((each) => (each.key === key ? answered : each)));
        }
      } catch (cause: unknown) {
        problem = fieldProblemText(t, cause, { fields: { [`body.valuesByKey.${key}`]: key } });
      }
      setProblems((previous) =>
        (previous.get(key) ?? "") === problem ? previous : new Map(previous).set(key, problem),
      );
      setSaved((previous) => withMember(previous, key, problem === ""));
    },
    [t],
  );
  const save = useCallback(
    (key: string, value: SettingValue): void => {
      void settle(key, async () => {
        const body = { valuesByKey: { [key]: value } };
        const answer = dataOrThrow(await api.PUT("/config", { body }));
        return answer.settings?.find((each) => each.key === key);
      });
    },
    [api, settle],
  );
  const reset = useCallback(
    (key: string): void => {
      setResetting((previous) => withMember(previous, key, true));
      void settle(key, async () =>
        dataOrThrow(
          await api.POST("/config/{key}/reset", {
            params: { path: { key } },
            ...idempotent(newIdempotencyKey()),
          }),
        ),
      ).finally(() => {
        setResetting((previous) => withMember(previous, key, false));
      });
    },
    [api, settle],
  );
  return { settings, problems, saved, resetting, save, reset };
}

/** The server's settings, as the tab last read them. */
export function useServerSettings(): Resource<readonly Setting[]> {
  const { api } = useServices();
  const [loaded] = useResource("config", async () => {
    const configBody = dataOrThrow(await api.GET("/config"));
    return configBody.settings ?? [];
  });
  return loaded;
}
