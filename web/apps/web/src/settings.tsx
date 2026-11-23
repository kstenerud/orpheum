// SPDX-License-Identifier: MPL-2.0

import "./settings.css";
import "./settings-words.ts";

import type { TFunction } from "i18next";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { AttentionList, pageNeedsYou, usePageAttention } from "./attention-list.tsx";
import type { AttentionItem } from "./attention-pages.ts";
import { browserStorage } from "./browser-storage.ts";
import { camelKey } from "./catalogue-key.ts";
import { Field } from "./field.tsx";
import { WarningIcon } from "./icons.tsx";
import { Link } from "./link.tsx";
import { Loaded } from "./loaded.tsx";
import { Page } from "./page.tsx";
import type { Resource } from "./resource.ts";
import type { Route } from "./router.ts";
import {
  isFound,
  networkSettingKeys,
  SectionSettingsView,
  settingDescription,
  settingLabel,
  useServerSettings,
  useSettingChanges,
  type Setting,
} from "./server-settings.tsx";
import {
  KeysView,
  secretDescription,
  useKeyChanges,
  useSecrets,
  type Secret,
} from "./settings-keys.tsx";
import { sectionOrder, type SettingsPage, type SettingsSection } from "./settings-pages.ts";
import { UnreadLine, UnreadSettingsFold, useUnreadSettings } from "./settings-unread.tsx";
import { Switch } from "./switch.tsx";
import type { Upward } from "./upward.ts";

/** Where this browser keeps whether "Show advanced settings" is on, for every section's page. */
export const advancedShownKey = "orpheum.settings.advancedShown";

/** A section's name in the person's words. */
function sectionName(t: TFunction, section: SettingsSection): string {
  return t(`settings.sections.${camelKey(section)}`);
}

/** A page of Settings' place: beneath the index, which its trail leads up to. */
function settingsUpward(t: TFunction, here: string): Upward {
  return {
    crumbs: [
      {
        label: t("settings.heading"),
        to: { screen: "settings" },
        isIt: (route: Route) => route.screen === "settings" && route.page === undefined,
      },
    ],
    here,
  };
}

/**
 * Settings, for an admin (D538 §11, pages-spec-3): the index at /settings, a page to each section,
 * and Keys and passwords; a page's address may name the setting focus arrives at.
 */
export function Settings(props: {
  readonly page?: SettingsPage | undefined;
  readonly setting?: string | undefined;
}): JSX.Element {
  if (props.page === "keys") {
    return <KeysPage arrival={props.setting} />;
  }
  if (props.page !== undefined) {
    return <SectionPage section={props.page} arrival={props.setting} />;
  }
  return <SettingsIndex />;
}

/** One setting or key a search found: its label, its page's name, and where it leads. */
interface Found {
  readonly key: string;
  readonly label: string;
  readonly pageName: string;
  readonly to: Route;
}

/** Every setting and key the search finds, advanced ones included, keys first. */
export function foundBySearch(
  t: TFunction,
  search: string,
  loaded: { readonly settings: readonly Setting[]; readonly secrets: readonly Secret[] },
): readonly Found[] {
  const { settings, secrets } = loaded;
  const keys = secrets
    .map((secret) => ({ secret, label: settingLabel(t, "secretLabels", secret.name) }))
    .filter(({ secret, label }) =>
      isFound(search, [label, secret.name, secretDescription(t, secret)]),
    )
    .map(({ secret, label }) => ({
      key: secret.name,
      label,
      pageName: t("settings.secretsHeading"),
      to: { screen: "settings", page: "keys", setting: secret.name } as const,
    }));
  const rest = [...settings]
    .sort((first, second) => {
      const bySection = sectionOrder.indexOf(first.section) - sectionOrder.indexOf(second.section);
      return bySection === 0 ? first.key.localeCompare(second.key) : bySection;
    })
    .map((setting) => ({ setting, label: settingLabel(t, "labels", setting.key) }))
    .filter(({ setting, label }) =>
      isFound(search, [label, setting.key, settingDescription(t, setting)]),
    )
    .map(({ setting, label }): Found =>
      // The address in the home is set on Network, not on its section's page.
      networkSettingKeys.has(setting.key)
        ? {
            key: setting.key,
            label,
            pageName: t("settings.networkPage"),
            to: { screen: "network" },
          }
        : {
            key: setting.key,
            label,
            pageName: sectionName(t, setting.section),
            to: { screen: "settings", page: setting.section, setting: setting.key },
          },
    );
  return [...keys, ...rest];
}

/** What a search found, each leading to its row on its page; or that nothing matches. */
function SearchResults(props: { readonly found: readonly Found[] }): JSX.Element {
  const { t } = useTranslation();
  if (props.found.length === 0) {
    return <p>{t("settings.noneFound")}</p>;
  }
  return (
    <ul className="settings-index">
      {props.found.map((found) => (
        <li key={found.key}>
          <Link to={found.to} className="settings-index__row">
            <span className="settings-index__name">{found.label}</span>
            <span className="settings-index__summary">{found.pageName}</span>
          </Link>
        </li>
      ))}
    </ul>
  );
}

/** The keys' states in a line, Keys and passwords' summary: "TMDB API key set · …". */
function keysSummary(t: TFunction, secrets: readonly Secret[]): string {
  return secrets
    .map((secret) => {
      const name = settingLabel(t, "secretLabels", secret.name);
      return secret.isSet ? t("settings.keySet", { name }) : t("settings.keyUnset", { name });
    })
    .join(" · ");
}

/** The pages, one row each: Keys and passwords first, its keys' states, then every section. */
function SectionList(props: {
  readonly secrets: readonly Secret[] | undefined;
  readonly isKeysNeeded: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <ul className="settings-index">
      <li>
        <Link to={{ screen: "settings", page: "keys" }} className="settings-index__row">
          <span className="settings-index__name">
            {t("settings.secretsHeading")}
            {props.isKeysNeeded ? <WarningIcon /> : null}
          </span>
          <span className="settings-index__summary">
            {props.secrets === undefined ? "" : keysSummary(t, props.secrets)}
          </span>
        </Link>
      </li>
      {sectionOrder.map((section) => (
        <li key={section}>
          <Link to={{ screen: "settings", page: section }} className="settings-index__row">
            <span className="settings-index__name">{sectionName(t, section)}</span>
            <span className="settings-index__summary">
              {t(`settings.summaries.${camelKey(section)}`)}
            </span>
          </Link>
        </li>
      ))}
    </ul>
  );
}

/**
 * The index's Needs you: each key's line, then the settings this release does not read, leading to
 * their fold; undefined while nothing needs the admin.
 */
function indexNeedsYou(
  items: readonly AttentionItem[],
  unreadCount: number,
): JSX.Element | undefined {
  if (items.length === 0 && unreadCount === 0) {
    return undefined;
  }
  return (
    <AttentionList items={items}>
      <UnreadLine count={unreadCount} />
    </AttentionList>
  );
}

/**
 * Settings' index as it is drawn (pages-spec-3): one search over every setting and key, advanced
 * ones included, whose results replace the list, or that nothing matches; else the pages, one row
 * each. What it reads is given, so the gallery draws it too.
 */
export function SettingsIndexView(props: {
  readonly search: string;
  readonly onSearch: (search: string) => void;
  /** Every setting, as read, for the search alone. */
  readonly settings: Resource<readonly Setting[]>;
  /** The keys and passwords, once read, for Keys and passwords' line. */
  readonly secrets: readonly Secret[] | undefined;
  /** Whether a key needs the admin, which marks Keys and passwords' row. */
  readonly isKeysNeeded: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const { search, onSearch, secrets } = props;
  return (
    <>
      <search className="settings__filters">
        <Field label={t("settings.search")}>
          {(control) => (
            <input
              {...control}
              type="search"
              value={search}
              onChange={(event) => {
                onSearch(event.target.value);
              }}
            />
          )}
        </Field>
      </search>
      {search.trim() === "" ? (
        <SectionList secrets={secrets} isKeysNeeded={props.isKeysNeeded} />
      ) : (
        <Loaded
          resource={props.settings}
          render={(loaded) => (
            <SearchResults
              found={foundBySearch(t, search, { settings: loaded, secrets: secrets ?? [] })}
            />
          )}
        />
      )}
    </>
  );
}

/**
 * Settings' index (pages-spec-3): Needs you while a key needs the admin or a setting is one this
 * release does not read; the search and the pages (SettingsIndexView); then the unread settings,
 * folded.
 */
function SettingsIndex(): JSX.Element {
  const { t } = useTranslation();
  const [search, setSearch] = useState("");
  const attention = usePageAttention("settings");
  const unread = useUnreadSettings();
  const settings = useServerSettings();
  const [secrets] = useSecrets();
  return (
    <Page
      heading={t("settings.heading")}
      job={t("settings.job")}
      needsYou={indexNeedsYou(attention, unread.listed.length)}
    >
      <SettingsIndexView
        search={search}
        onSearch={setSearch}
        settings={settings}
        secrets={secrets.state === "loaded" ? secrets.value : undefined}
        isKeysNeeded={attention.length > 0}
      />
      <UnreadSettingsFold unread={unread} />
    </Page>
  );
}

/** Whether this browser shows every setting on a section's page, not only the important ones. */
function useAdvancedShown(): readonly [boolean, (isShown: boolean) => void] {
  const [isShown, setIsShown] = useState(
    () => browserStorage().getItem(advancedShownKey) === "true",
  );
  return [
    isShown,
    (shouldShow: boolean): void => {
      if (shouldShow) {
        browserStorage().setItem(advancedShownKey, "true");
      } else {
        browserStorage().removeItem(advancedShownKey);
      }
      setIsShown(shouldShow);
    },
  ];
}

/** A section's settings as loaded, each saved or reset on its own. */
function ChangingSection(props: {
  readonly loaded: readonly Setting[];
  readonly section: SettingsSection;
  readonly isAdvanced: boolean;
  readonly arrival: string | undefined;
}): JSX.Element {
  const changes = useSettingChanges(props.loaded);
  return (
    <SectionSettingsView
      settings={changes.settings}
      section={props.section}
      isAdvanced={props.isAdvanced}
      arrival={props.arrival}
      problems={changes.problems}
      saved={changes.saved}
      onSave={changes.save}
      onReset={changes.reset}
    />
  );
}

/**
 * A section's page (pages-spec-3): up to Settings, the section's name its heading and no job line;
 * "Show advanced settings", remembered in this browser for every section; then its settings. Remote
 * access leads to Network for the address in the home, which is set there.
 */
function SectionPage(props: {
  readonly section: SettingsSection;
  readonly arrival: string | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const name = sectionName(t, props.section);
  const settings = useServerSettings();
  const [isAdvanced, setIsAdvanced] = useAdvancedShown();
  // A setting the address names is shown, advanced or not, for focus to arrive at.
  const isShowingAll =
    isAdvanced ||
    (settings.state === "loaded" &&
      settings.value.some((each) => each.key === props.arrival && !each.isImportant));
  return (
    <Page heading={name} upward={settingsUpward(t, name)}>
      <div className="settings__advanced">
        <Switch label={t("settings.advanced")} isOn={isShowingAll} onSwitch={setIsAdvanced} />
      </div>
      {props.section === "remoteAccess" ? (
        <p>
          <Link to={{ screen: "network" }}>{t("settings.onNetwork")}</Link>
        </p>
      ) : null}
      <Loaded
        resource={settings}
        render={(loaded) => (
          <ChangingSection
            loaded={loaded}
            section={props.section}
            isAdvanced={isShowingAll}
            arrival={props.arrival}
          />
        )}
      />
    </Page>
  );
}

/**
 * Keys and passwords (pages-spec-3): up to Settings, what they are and that none is shown again,
 * Needs you while one must be entered again or was refused, then a row to each key.
 */
function KeysPage(props: { readonly arrival: string | undefined }): JSX.Element {
  const { t } = useTranslation();
  const heading = t("settings.secretsHeading");
  const attention = usePageAttention("settings");
  const [secrets, reload] = useSecrets();
  const changes = useKeyChanges(reload);
  return (
    <Page
      heading={heading}
      upward={settingsUpward(t, heading)}
      job={t("settings.keysJob")}
      needsYou={pageNeedsYou(attention)}
    >
      <Loaded
        resource={secrets}
        render={(loaded) => (
          <KeysView
            secrets={loaded}
            arrival={props.arrival}
            problems={changes.problems}
            saved={changes.saved}
            onSave={changes.save}
            onRemove={changes.remove}
          />
        )}
      />
    </Page>
  );
}
