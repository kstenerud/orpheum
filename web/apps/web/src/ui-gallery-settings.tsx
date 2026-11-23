// SPDX-License-Identifier: MPL-2.0

import "./gallery-words.ts";
import "./settings-words.ts";

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { SectionSettingsView, type Setting } from "./server-settings.tsx";
import { SettingInfo } from "./setting-info.tsx";
import { KeysView, type Secret } from "./settings-keys.tsx";
import { SettingsIndexView } from "./settings.tsx";

/** A setting of the gallery's General section, important and at its starting value unless changed. */
function gallerySetting(changed: Partial<Setting> & Pick<Setting, "key" | "kind">): Setting {
  return {
    description: "",
    value: "",
    startingValue: "",
    isModified: false,
    isImportant: true,
    section: "general",
    setAt: 0,
    ...changed,
  };
}

/** A setting of each kind a section's page draws: a Switch, a Stepper and a text Field. */
const gallerySettings: readonly Setting[] = [
  gallerySetting({ key: "shouldCheckForUpdates", kind: "bool", value: true, startingValue: true }),
  gallerySetting({
    key: "continueWatchingMaxWorks",
    kind: "integer",
    value: 20,
    startingValue: 20,
    minimum: 1,
    maximum: 50,
  }),
  gallerySetting({ key: "timeZone", kind: "string", value: "Europe/Berlin", isModified: true }),
];

/** The keys and passwords: one set, with its last four; one not; one to be entered again. */
const gallerySecrets: readonly Secret[] = [
  { name: "tmdbApiKey", description: "", isSet: true, lastChars: "4f2a", needsReentry: false },
  { name: "opensubtitlesUsername", description: "", isSet: false, needsReentry: false },
  { name: "opensubtitlesPassword", description: "", isSet: true, needsReentry: true },
];

const ignore = (): void => undefined;

/** Settings' index as searched for search, which can be searched again. */
function GalleryIndex(props: { readonly search: string }): JSX.Element {
  const [search, setSearch] = useState(props.search);
  return (
    <SettingsIndexView
      search={search}
      onSearch={setSearch}
      settings={{ state: "loaded", value: gallerySettings }}
      secrets={gallerySecrets}
      isKeysNeeded
    />
  );
}

/** A section's settings as each change went: saved, refused, or not yet answered. */
function GallerySection(props: {
  readonly isSaved?: boolean;
  readonly refusal?: string;
  readonly isAdvanced?: boolean;
  readonly settings?: readonly Setting[];
}): JSX.Element {
  const { refusal } = props;
  const settings = props.settings ?? gallerySettings;
  const keys = settings.map((setting) => setting.key);
  const problems = new Map<string, string>();
  if (refusal !== undefined) {
    for (const key of keys) {
      problems.set(key, refusal);
    }
  }
  return (
    <SectionSettingsView
      settings={settings}
      section="general"
      isAdvanced={props.isAdvanced ?? true}
      problems={problems}
      saved={new Set(props.isSaved === true ? keys : [])}
      onSave={ignore}
      onReset={ignore}
    />
  );
}

/** One state of Settings' pages, under its heading. */
function GalleryState(props: {
  readonly heading: string;
  readonly children: JSX.Element;
}): JSX.Element {
  return (
    <div className="ui-gallery__notes">
      <h3>{props.heading}</h3>
      {props.children}
    </div>
  );
}

/**
 * Settings' pages (D538 §11, pages-spec-3), drawn by their own views: the index's rows, a search's
 * results and a search nothing matches; a section's page with a setting of each kind, saved, refused
 * and before its change is answered, and with every setting advanced; Keys and passwords; and What
 * it is for, at a setting's default and changed from it.
 */
export function GallerySettings(): JSX.Element {
  const { t } = useTranslation();
  return (
    <section aria-labelledby="gallery-settings">
      <h2 id="gallery-settings">{t("gallery.settings")}</h2>
      <GalleryState heading={t("gallery.settingsIndex")}>
        <GalleryIndex search="" />
      </GalleryState>
      <GalleryState heading={t("gallery.settingsFound")}>
        <GalleryIndex search={t("gallery.settingsFoundSearch")} />
      </GalleryState>
      <GalleryState heading={t("gallery.settingsNoneFound")}>
        <GalleryIndex search={t("gallery.settingsNoneFoundSearch")} />
      </GalleryState>
      <GalleryState heading={t("gallery.settingsSaved")}>
        <GallerySection isSaved />
      </GalleryState>
      <GalleryState heading={t("gallery.settingsRefused")}>
        <GallerySection refusal={t("gallery.settingsRefusal")} />
      </GalleryState>
      <GalleryState heading={t("gallery.settingsUnsaved")}>
        <GallerySection />
      </GalleryState>
      <GalleryState heading={t("gallery.settingsAllAdvanced")}>
        <GallerySection
          isAdvanced={false}
          settings={gallerySettings.map((setting) => ({ ...setting, isImportant: false }))}
        />
      </GalleryState>
      <GalleryState heading={t("gallery.settingsKeys")}>
        <KeysView secrets={gallerySecrets} problems={new Map()} onSave={ignore} onRemove={ignore} />
      </GalleryState>
      <GalleryInfo />
    </section>
  );
}

/** What it is for, at a setting's default, Done alone, and changed from it, with Use the default. */
function GalleryInfo(): JSX.Element {
  const { t } = useTranslation();
  const label = t("settings.labels.backupEveryHours");
  const description = t("settings.descriptions.backupEveryHours");
  return (
    <>
      <GalleryState heading={t("gallery.settingsInfoAtDefault")}>
        <SettingInfo
          label={label}
          configName="backupEveryHours"
          description={description}
          default={{ text: "6", isModified: false, onUse: ignore }}
        />
      </GalleryState>
      <GalleryState heading={t("gallery.settingsInfoChanged")}>
        <SettingInfo
          label={label}
          configName="backupEveryHours"
          description={description}
          default={{ text: "6", isModified: true, onUse: ignore }}
        />
      </GalleryState>
    </>
  );
}
