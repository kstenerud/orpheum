// SPDX-License-Identifier: MPL-2.0

import "./libraries-words.ts";

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { CreateLibrary } from "./create-library.tsx";
import { Link } from "./link.tsx";
import { LibraryOffer } from "./library-offer.tsx";
import { stepAfter } from "./library-offer-kinds.ts";
import type { WelcomeStep } from "./router.ts";
import { useServices } from "./services.tsx";
import { useReadLibrariesAgain } from "./libraries-reached.ts";

/**
 * The first-time pages the first admin meets after being made (D527 §3), in order: libraries, then
 * discovery. Posters have no page: Orpheum ships its own TMDB key (D527 §1.3), and Settings holds
 * the key of a person who wants theirs. Each page is at an address of its own, and each can be
 * skipped; no menu leads to them.
 */
export function Welcome(props: { readonly step: WelcomeStep | undefined }): JSX.Element {
  return props.step === "discovery" ? <DiscoveryStep /> : <LibrariesStep />;
}

/** Goes on to the page after step, in its place, so Back does not return to one already done. */
function useGoOn(step: WelcomeStep | undefined): () => void {
  const { navigation } = useServices();
  return () => {
    navigation.replace(stepAfter(step));
  };
}

/** The first page: a library for each folder of the media folder, or one of a whole folder. */
function LibrariesStep(): JSX.Element {
  const { t } = useTranslation();
  const goOn = useGoOn(undefined);
  const readLibrariesAgain = useReadLibrariesAgain();
  return (
    <section>
      <h1>{t("welcome.librariesHeading")}</h1>
      <p>{t("welcome.librariesExplain")}</p>
      <LibraryOffer onMade={goOn} />
      <h2>{t("welcome.wholeFolderHeading")}</h2>
      <CreateLibrary
        isNested
        onCreated={() => {
          readLibrariesAgain();
          goOn();
        }}
      />
      <p>
        <button type="button" onClick={goOn}>
          {t("welcome.skip")}
        </button>
      </p>
    </section>
  );
}

/** The last page: what a discovery gives, that the server works without one, and where to connect. */
function DiscoveryStep(): JSX.Element {
  const { t } = useTranslation();
  const goOn = useGoOn("discovery");
  return (
    <section>
      <h1>{t("welcome.discoveryHeading")}</h1>
      <p>{t("welcome.discoveryExplain")}</p>
      <ul>
        <li>{t("welcome.discoveryAway")}</li>
        <li>{t("welcome.discoveryTv")}</li>
        <li>{t("welcome.discoveryInvite")}</li>
      </ul>
      <p>{t("welcome.discoveryNotNeeded")}</p>
      <p>
        <Link to={{ screen: "system" }}>{t("welcome.discoveryLater")}</Link>
      </p>
      <p>
        <button type="button" onClick={goOn}>
          {t("welcome.finish")}
        </button>
      </p>
    </section>
  );
}
