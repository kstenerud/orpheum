// SPDX-License-Identifier: MPL-2.0

import "./ui-gallery.css";
import "./gallery-words.ts";
import "./play-words.ts";
import "./cast-words.ts";
import "./player-overlay.css";

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { ActionRow } from "./action-row.tsx";
import type { UpNext } from "./around-file.ts";
import { OfferCard } from "./offer-card.tsx";
import { Page } from "./page.tsx";
import { CornerNotes } from "./corner-notes.tsx";
import { OverThePicture } from "./over-the-picture.tsx";
import { ActionPill, Pill } from "./pill.tsx";
import type { PlayerSource } from "./player-controls.tsx";
import { SoundOnPill } from "./sound-on-pill.tsx";
import { Section } from "./section.tsx";
import { UpNextCard } from "./up-next.tsx";

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

/** The episode the gallery's Up next offers. */
function galleryUpNext(name: string): UpNext {
  return {
    episode: { itemId: 3, fileId: 3, groupNumber: 1, numberInGroup: 3, isPartway: false, name },
    ordering: "aired",
  };
}

/** A film playing at 70 %, as the gallery's centre Pill says it. */
const placesSource: PlayerSource = {
  atMs: 0,
  durationMs: 0,
  isPaused: false,
  isReady: true,
  playOrPause: () => undefined,
  seekTo: () => undefined,
  sound: {
    volume: 0.7,
    isMuted: false,
    canSetVolume: true,
    setVolume: () => undefined,
    setMuted: () => undefined,
  },
};

/**
 * A film with each of its places over it (player-spec, the three places; D538 §1): the sound-on
 * Pill at its top centre; the centre's one Pill, the volume, and a double tap's "+10" in the third
 * tapped; the offer, Up next counting, in the top-right corner; and the notes at the bottom-left,
 * the newest lowest. Each part is the player's own, placed by its own rules.
 */
function GalleryPlaces(props: { readonly press: Press }): JSX.Element {
  const { t } = useTranslation();
  const { press } = props;
  return (
    <div className="ui-gallery__film ui-gallery__places">
      <SoundOnPill onPress={press("gallery.layoutParts.soundOn")} />
      <OverThePicture
        state={{ shown: "hidden", shownBy: "pointer" }}
        source={placesSource}
        isVolumeShown
        seekFeedback={{ direction: 1, seconds: 10, isShown: true }}
        isStartRefused={false}
      />
      <div className="ui-gallery__offer-place">
        <UpNextCard
          upNext={galleryUpNext(t("gallery.layoutParts.episodeName"))}
          secondsLeft={8}
          play={{ onPlay: press("upNext.playNow") }}
        />
      </div>
      <CornerNotes
        notes={[
          { id: 0, text: t("gallery.layoutParts.joined") },
          { id: 1, text: t("gallery.layoutParts.left") },
          { id: 2, text: t("cast.controlledFrom", { name: t("gallery.layoutParts.device") }) },
        ]}
        onGone={() => undefined}
      />
    </div>
  );
}

/**
 * The offer cards' other states: Up next with no count, as it stops or on a cast's remote, with no
 * ✕ (Back puts it away); Skip, the card itself the button; and Still watching with its buttons.
 */
function GalleryOffers(props: { readonly press: Press }): JSX.Element {
  const { t } = useTranslation();
  const { press } = props;
  return (
    <div className="ui-gallery__offers">
      <UpNextCard
        upNext={galleryUpNext(t("gallery.layoutParts.episodeName"))}
        play={{ onPlay: press("upNext.playNow") }}
      />
      <OfferCard words={t("playerControls.skipIntro")} onTake={press("playerControls.skipIntro")} />
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
  );
}

/** The film's places, then each pill and offer card in its states, on a film's ground. */
function GalleryOverFilm(props: { readonly press: Press }): JSX.Element {
  const { t } = useTranslation();
  const { press } = props;
  return (
    <>
      <GalleryPlaces press={press} />
      <div className="ui-gallery__film ui-gallery__over-film">
        <div className="ui-gallery__pills">
          <Pill>{t("gallery.layoutParts.seeked")}</Pill>
          <Pill>{t("gallery.layoutParts.joined")}</Pill>
          <Pill>
            {t("skipped.told", {
              stretches: "42:10–42:28",
            })}
          </Pill>
          <ActionPill onPress={press("gallery.layoutParts.soundOn")}>
            {t("gallery.layoutParts.soundOn")}
          </ActionPill>
          <ActionPill isDangerous onPress={press("cast.stopHere")}>
            {t("cast.stopHere")}
          </ActionPill>
          <ActionPill isDangerous isDisabled onPress={press("cast.stopHere")}>
            {t("cast.stopHere")}
          </ActionPill>
          <Pill>{t("playerControls.gettingReady")}</Pill>
          <Pill>{t("play.reopening")}</Pill>
        </div>
        <div className="ui-gallery__prompts">
          <SoundOnPill isForVoices onPress={press("together.voice.tapToHear")} />
          <SoundOnPill />
        </div>
        <GalleryOffers press={press} />
      </div>
    </>
  );
}
