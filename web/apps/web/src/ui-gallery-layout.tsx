// SPDX-License-Identifier: MPL-2.0

import "./ui-gallery.css";
import "./gallery-words.ts";

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { ActionRow } from "./action-row.tsx";
import { OfferCard } from "./offer-card.tsx";
import { Page } from "./page.tsx";
import { ActionPill, Pill } from "./pill.tsx";
import { Section } from "./section.tsx";

/** Writes down a press, by the key of the words of what was pressed. */
type Press = (key: string) => () => void;

/**
 * The UI gallery's layout parts (D538 §1): a Page with its Needs you place and a Section, an
 * ActionRow, and the pills and offer cards drawn over a film, each pressable, the last press named.
 */
export function GalleryLayoutParts(): JSX.Element {
  const { t } = useTranslation();
  const [pressed, setPressed] = useState("");
  const press: Press = (key) => () => {
    setPressed(t(key));
  };
  return (
    <section aria-labelledby="gallery-layout-parts">
      <h2 id="gallery-layout-parts">{t("gallery.layoutParts.title")}</h2>
      <p>{t("gallery.layoutParts.lead")}</p>
      <GalleryPage press={press} />
      <h3>{t("gallery.layoutParts.overFilm")}</h3>
      <GalleryOverFilm press={press} />
      <output>{pressed === "" ? null : t("gallery.layoutParts.pressed", { what: pressed })}</output>
    </section>
  );
}

/** A button whose words are the key's, pressed as press writes down. */
function Pressable(props: { readonly words: string; readonly press: Press }): JSX.Element {
  const { t } = useTranslation();
  return (
    <button type="button" onClick={props.press(props.words)}>
      {t(props.words)}
    </button>
  );
}

/** A page in a frame of its own: its heading, job and action, Needs you, a section, an action row. */
function GalleryPage(props: { readonly press: Press }): JSX.Element {
  const { t } = useTranslation();
  const { press } = props;
  return (
    <div className="ui-gallery__frame">
      <Page
        heading={t("gallery.layoutParts.pageHeading")}
        job={t("gallery.layoutParts.pageJob")}
        actions={<Pressable words="gallery.layoutParts.pageAction" press={press} />}
        needsYou={<p>{t("gallery.layoutParts.needsYouNote")}</p>}
      >
        <Section
          heading={t("gallery.layoutParts.section")}
          actions={<Pressable words="gallery.layoutParts.sectionAction" press={press} />}
        >
          <p>{t("gallery.layoutParts.sectionNote")}</p>
        </Section>
        <Section heading={t("gallery.layoutParts.actionRow")}>
          <ActionRow
            secondary={<Pressable words="gallery.layoutParts.cancel" press={press} />}
            main={<Pressable words="gallery.layoutParts.save" press={press} />}
          />
        </Section>
      </Page>
    </div>
  );
}

/** The pills and the offer cards, on a film's ground. */
function GalleryOverFilm(props: { readonly press: Press }): JSX.Element {
  const { t } = useTranslation();
  const { press } = props;
  return (
    <div className="ui-gallery__film ui-gallery__over-film">
      <div className="ui-gallery__pills">
        <Pill>{t("gallery.layoutParts.seeked")}</Pill>
        <Pill voice="volume">{t("gallery.layoutParts.volume")}</Pill>
        <Pill voice="silent">{t("gallery.layoutParts.seekForward")}</Pill>
        <Pill>{t("gallery.layoutParts.joined")}</Pill>
        <Pill>
          {t("skipped.told", {
            stretches: "42:10–42:28",
          })}
        </Pill>
        <ActionPill onPress={press("gallery.layoutParts.soundOn")}>
          {t("gallery.layoutParts.soundOn")}
        </ActionPill>
      </div>
      <div className="ui-gallery__offers">
        <OfferCard
          words={t("gallery.layoutParts.upNext")}
          detail={t("gallery.layoutParts.upNextDetail")}
          still={<span className="ui-gallery__poster--medium ui-gallery__still" />}
          onTake={press("gallery.layoutParts.upNext")}
          dismissal={{
            label: t("gallery.layoutParts.dismissUpNext"),
            onDismiss: press("gallery.layoutParts.dismissUpNext"),
          }}
        />
        <OfferCard
          words={t("gallery.layoutParts.stillWatching")}
          buttons={
            <>
              <Pressable words="gallery.layoutParts.continue" press={press} />
              <Pressable words="gallery.layoutParts.stop" press={press} />
            </>
          }
        />
      </div>
    </div>
  );
}
