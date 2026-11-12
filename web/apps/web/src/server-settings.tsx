// SPDX-License-Identifier: MPL-2.0

import "./settings.css";

import type { components } from "@orpheum/sdk";
import type { TFunction } from "i18next";
import { memo, useCallback, useEffect, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { camelKey } from "./catalogue-key.ts";
import { AboutIcon, ResetIcon } from "./icons.tsx";
import { idempotent, newIdempotencyKey } from "./idempotent.ts";
import { ModalDialog } from "./modal-dialog.tsx";
import { dataOrThrow, fieldProblemText } from "./problem.ts";
import { SaveNote, shownSaveState } from "./save-note.tsx";
import { useServices } from "./services.tsx";
import { Loaded } from "./title.tsx";
import { useResource } from "./use-resource.ts";

/** A server setting, with its value and its starting value. */
export type Setting = components["schemas"]["SettingBody"];
/** The part of the page a setting is listed in. */
type Section = Setting["section"];
/** A setting's value, by its kind. */
type SettingValue = string | number | boolean;

/** The sections in the server's order (settings.Sections), which a test holds to the API document's. */
export const sectionOrder: readonly Section[] = [
  "general",
  "accounts",
  "libraries",
  "metadata",
  "playback",
  "slowLinks",
  "upload",
  "together",
  "nodes",
  "jobs",
  "remoteAccess",
  "backups",
  "diagnostics",
];

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
 * A setting's or a key's name in the person's words, shown above its field: the catalogue's, or its
 * key where the catalogue has none (ui-audit-2026-11-04.md §3, Settings).
 */
export function settingLabel(
  t: TFunction,
  catalogue: "labels" | "secretLabels",
  key: string,
): string {
  return t(`settings.${catalogue}.${camelKey(key)}`, { defaultValue: key });
}

/**
 * The top of a setting's row, in one column (ui-audit-2026-11-04.md §3): its name in the person's
 * words, which names its field and its row, and its key in small type under it.
 */
export function RowName(props: {
  /** The id its row and field are named by. */
  readonly id: string;
  readonly fieldId: string;
  readonly label: string;
  readonly settingKey: string;
}): JSX.Element {
  return (
    <>
      <label className="settings__label" id={props.id} htmlFor={props.fieldId}>
        {props.label}
      </label>
      <code className="settings__key">{props.settingKey}</code>
    </>
  );
}

/** What a setting is for, in the person's words: the catalogue's, or the server's for one it lacks. */
export function settingDescription(t: TFunction, setting: Setting): string {
  return t(`settings.descriptions.${camelKey(setting.key)}`, { defaultValue: setting.description });
}

/**
 * The ⓘ button, and what it opens: a modal over the page, scrolling inside where it is long, which a
 * press outside it or Escape closes (the owner, 2026-10-21). It says the name, what it is for in
 * short, and, after a blank line, more, for a setting or key the short words cannot explain.
 */
export function AboutButton(props: {
  readonly name: string;
  readonly description: string;
}): JSX.Element {
  const { t, i18n } = useTranslation();
  const [isOpen, setIsOpen] = useState(false);
  const about = t("settings.about", { key: props.name });
  const detailsKey = `settings.details.${camelKey(props.name)}`;
  const details = i18n.exists(detailsKey) ? t(detailsKey) : "";
  return (
    <>
      <button
        type="button"
        className="settings__control"
        aria-label={about}
        data-tip={t("settings.tipAbout")}
        onClick={() => {
          setIsOpen(true);
        }}
      >
        <AboutIcon />
      </button>
      {isOpen ? (
        <ModalDialog
          label={about}
          isWide
          onClose={() => {
            setIsOpen(false);
          }}
        >
          <h2 className="settings__about-name">
            <code>{props.name}</code>
          </h2>
          <p>{props.description}</p>
          {details === "" ? null : <p>{details}</p>}
        </ModalDialog>
      ) : null}
    </>
  );
}

/** Whether a setting is found by every word of a search, in its key or its description. */
export function isFound(search: string, key: string, description: string): boolean {
  const words = search
    .toLowerCase()
    .split(/\s+/u)
    .filter((word) => word !== "");
  const haystack = `${key} ${description}`.toLowerCase();
  return words.every((word) => haystack.includes(word));
}

/** The settings by their section, in the sections' order, each section's in its keys' order. */
export function bySection(settings: readonly Setting[]): [Section, Setting[]][] {
  const sections = new Map<Section, Setting[]>();
  for (const setting of [...settings].sort((first, second) =>
    first.key.localeCompare(second.key),
  )) {
    sections.set(setting.section, [...(sections.get(setting.section) ?? []), setting]);
  }
  return [...sections.entries()].sort(
    ([first], [second]) => sectionOrder.indexOf(first) - sectionOrder.indexOf(second),
  );
}

/**
 * A setting's field and what saves it: the text in the field, and commit, which saves it when it
 * differs from the server's. The server's value, saved or reset, becomes the field's; leaving the
 * page saves what was typed: a move to another screen unmounts the row, and a reload or a closed
 * tab hides the page without unmounting anything.
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
  // What the field holds and what the server holds, for the saves the field's leaving makes.
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

/** The field a setting's value is typed or chosen in, by its kind. */
function ValueField(props: {
  readonly id: string;
  readonly setting: Setting;
  readonly text: string;
  readonly onText: (text: string) => void;
  readonly onCommit: (text?: string) => void;
}): JSX.Element {
  const { setting } = props;
  if (setting.kind === "bool") {
    return (
      <input
        id={props.id}
        type="checkbox"
        aria-labelledby={`setting-${setting.key}`}
        checked={props.text === "true"}
        onChange={(event) => {
          // A choice is the change itself: it is saved at once.
          const text = String(event.target.checked);
          props.onText(text);
          props.onCommit(text);
        }}
      />
    );
  }
  return (
    <input
      id={props.id}
      className="settings__value"
      type={setting.kind === "integer" ? "number" : "text"}
      aria-labelledby={`setting-${setting.key}`}
      min={setting.minimum}
      max={setting.maximum}
      value={props.text}
      onChange={(event) => {
        props.onText(event.target.value);
      }}
      // Typing is saved on Enter (the row's submit) or on leaving the field; a number's arrows,
      // which no one types, as they are let go.
      onBlur={() => {
        props.onCommit();
      }}
      onPointerUp={() => {
        props.onCommit();
      }}
    />
  );
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
 * The bin's tooltip: what pressing it does, or why it cannot be pressed. The row says which setting
 * it is, so the tooltip does not (the owner, 2026-10-21); its label, for a screen reader, does.
 */
function resetTip(
  t: TFunction,
  bin: { readonly isModified: boolean; readonly isResetting: boolean; readonly start: string },
): string {
  if (bin.isResetting) {
    return t("settings.tipResetting");
  }
  return bin.isModified ? t("settings.tipReset", { start: bin.start }) : t("settings.tipAtDefault");
}

/** A row's buttons: what the setting is for, and its starting value back while it differs. */
function RowControls(props: {
  readonly setting: Setting;
  readonly isResetting: boolean;
  readonly onReset: (key: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { setting } = props;
  const start = startText(t, setting.startingValue);
  const reset = setting.isModified
    ? t("settings.reset", { key: setting.key, start })
    : t("settings.isStart", { key: setting.key });
  return (
    <span className="settings__controls">
      <AboutButton
        name={setting.key}
        description={setting.question ?? settingDescription(t, setting)}
      />
      <button
        type="button"
        className="settings__control"
        disabled={!setting.isModified || props.isResetting}
        aria-label={reset}
        data-tip={resetTip(t, {
          isModified: setting.isModified,
          isResetting: props.isResetting,
          start,
        })}
        onClick={() => {
          props.onReset(setting.key);
        }}
      >
        <ResetIcon />
      </button>
    </span>
  );
}

/**
 * One setting's row, in one column: its name, its key in small type under it, then its value with
 * what it is for and a way back to its starting value, then how its last change went. A change is
 * saved without a button (the owner, 2026-11-04, Q5): a choice at once, typing on Enter or on
 * leaving the field, the page included; "Saved" says it was, or the refusal shows in its place.
 */
const SettingRow = memo(function SettingRow(props: {
  readonly setting: Setting;
  /** Why the server refused this setting's last change; "" when it did not. */
  readonly problem: string;
  /** Whether its last change was saved, so it says so. */
  readonly isSaved: boolean;
  readonly isResetting: boolean;
  readonly onSave: (key: string, value: SettingValue) => void;
  readonly onReset: (key: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { setting } = props;
  const { text, setText, commit } = useSavedField(setting, props.onSave);
  const fieldId = `setting-field-${setting.key}`;
  return (
    <form
      className="settings__row"
      aria-labelledby={`setting-${setting.key}`}
      onSubmit={(event) => {
        event.preventDefault();
        commit();
      }}
    >
      <RowName
        id={`setting-${setting.key}`}
        fieldId={fieldId}
        label={settingLabel(t, "labels", setting.key)}
        settingKey={setting.key}
      />
      <span className="settings__field">
        <ValueField id={fieldId} setting={setting} text={text} onText={setText} onCommit={commit} />
        <RowControls setting={setting} isResetting={props.isResetting} onReset={props.onReset} />
      </span>
      <SaveNote state={shownSaveState(props.problem, props.isSaved)} />
    </form>
  );
});

/**
 * The search, and Advanced, which shows every setting rather than the important ones: a search
 * landmark (`<search>`), which a screen reader goes to by its list of landmarks.
 */
export function SettingsFilters(props: {
  readonly isAdvanced: boolean;
  readonly onAdvanced: (isAdvanced: boolean) => void;
  readonly search: string;
  readonly onSearch: (search: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <search className="settings__filters">
      <label>
        {t("settings.search")}{" "}
        <input
          type="search"
          placeholder={t("settings.searchHint")}
          value={props.search}
          onChange={(event) => {
            props.onSearch(event.target.value);
          }}
        />
      </label>
      <label>
        <input
          type="checkbox"
          checked={props.isAdvanced}
          onChange={(event) => {
            props.onAdvanced(event.target.checked);
          }}
        />{" "}
        {t("settings.advanced")}
      </label>
    </search>
  );
}

/**
 * The server settings, one to a row by section (D200): the important ones, or with Advanced every
 * one, found by a search of their keys and descriptions.
 */
export function SettingsView(props: {
  readonly settings: readonly Setting[];
  /** Whether every setting shows, not only the important ones (Advanced). */
  readonly isAdvanced: boolean;
  /** What the person searched for: a setting shows when it has every word. */
  readonly search: string;
  /** Why the server refused a setting's last change, by key. */
  readonly problems: ReadonlyMap<string, string>;
  /** The settings whose last change was saved, which say so. */
  readonly saved?: ReadonlySet<string>;
  /** The settings a reset of is running, whose bins wait. */
  readonly resetting: ReadonlySet<string>;
  readonly onSave: (key: string, value: SettingValue) => void;
  readonly onReset: (key: string) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const shown = props.settings.filter(
    (setting) =>
      (props.isAdvanced || setting.isImportant) &&
      isFound(props.search, setting.key, settingDescription(t, setting)),
  );
  return (
    <>
      {shown.length === 0 ? <p>{t("settings.noneFound")}</p> : null}
      {bySection(shown).map(([section, settings]) => (
        <section key={section} aria-labelledby={`settings-section-${section}`}>
          <h2 className="settings__section" id={`settings-section-${section}`}>
            {t(`settings.sections.${camelKey(section)}`)}
          </h2>
          {settings.map((setting) => (
            <SettingRow
              key={setting.key}
              setting={setting}
              problem={props.problems.get(setting.key) ?? ""}
              isSaved={props.saved?.has(setting.key) ?? false}
              isResetting={props.resetting.has(setting.key)}
              onSave={props.onSave}
              onReset={props.onReset}
            />
          ))}
        </section>
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
function useSettingChanges(loaded: readonly Setting[]): {
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
        // Kept alive, so a save the page's leaving sends outlives the page.
        const body = { valuesByKey: { [key]: value } };
        const answer = dataOrThrow(await api.PUT("/config", { body, keepalive: true }));
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

/** The settings as loaded, each saved or reset on its own. */
function ChangingSettings(props: {
  readonly loaded: readonly Setting[];
  readonly isAdvanced: boolean;
  readonly search: string;
}): JSX.Element {
  const changes = useSettingChanges(props.loaded);
  return (
    <SettingsView
      isAdvanced={props.isAdvanced}
      search={props.search}
      settings={changes.settings}
      problems={changes.problems}
      saved={changes.saved}
      resetting={changes.resetting}
      onSave={changes.save}
      onReset={changes.reset}
    />
  );
}

/** The server's settings, loaded, with saving and resetting; a refusal is shown beside its setting. */
export function ServerSettings(props: {
  readonly isAdvanced: boolean;
  readonly search: string;
}): JSX.Element {
  const { api } = useServices();
  const [loaded] = useResource("config", async () => {
    const configBody = dataOrThrow(await api.GET("/config"));
    return configBody.settings ?? [];
  });
  return (
    <Loaded
      resource={loaded}
      render={(settings) => (
        <ChangingSettings loaded={settings} isAdvanced={props.isAdvanced} search={props.search} />
      )}
    />
  );
}
