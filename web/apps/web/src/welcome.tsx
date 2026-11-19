// SPDX-License-Identifier: MPL-2.0

import "./libraries-words.ts";

import { useEffect, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { CreateLibrary } from "./create-library.tsx";
import { Link } from "./link.tsx";
import { LibraryOffer } from "./library-offer.tsx";
import { stepAfter } from "./library-offer-kinds.ts";
import { Loaded } from "./loaded.tsx";
import { dataOrThrow, fieldProblemText, throwIfRefused } from "./problem.ts";
import type { WelcomeStep } from "./router.ts";
import { useServices } from "./services.tsx";
import { useReadLibrariesAgain } from "./libraries-reached.ts";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";

/** The secret the metadata is fetched with (the server's tmdb_api_key). */
const tmdbSecret = "tmdb_api_key";

/**
 * The first-time pages the first admin meets after being made (D527 §3), in order: libraries, then
 * posters (skipped when a TMDB key is set already), then discovery. Each is at an address of its
 * own, and each can be skipped; no menu leads to them.
 */
export function Welcome(props: { readonly step: WelcomeStep | undefined }): JSX.Element {
  if (props.step === "posters") {
    return <PostersStep />;
  }
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

/** The second page: posters, descriptions and cast come from TMDB, which needs a free key. */
function PostersStep(): JSX.Element {
  const { api } = useServices();
  const goOn = useGoOn("posters");
  const [isKeySet] = useResource("welcome-tmdb-key", async () => {
    const secrets = dataOrThrow(await api.GET("/secrets")).secrets ?? [];
    return secrets.some((secret) => secret.name === tmdbSecret && secret.isSet);
  });
  const isSet = isKeySet.state === "loaded" && isKeySet.value;
  useEffect(() => {
    if (isSet) {
      goOn();
    }
  });
  const { t } = useTranslation();
  return (
    <Loaded
      resource={isKeySet}
      render={(isAlready) =>
        isAlready ? <output>{t("welcome.postersAlready")}</output> : <PostersAsk goOn={goOn} />
      }
    />
  );
}

/** The steps to a TMDB key, and the field to paste it in. */
function PostersAsk(props: { readonly goOn: () => void }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [token, setToken] = useState("");
  const saving = useRequest((cause) =>
    fieldProblemText(t, cause, { fields: { "body.value": t("welcome.postersField") } }),
  );
  const save = (): void => {
    saving.runRequest(async () => {
      throwIfRefused(
        await api.PUT("/secrets/{name}", {
          params: { path: { name: tmdbSecret } },
          body: { value: token.trim() },
        }),
      );
      props.goOn();
    });
  };
  return (
    <section>
      <h1>{t("welcome.postersHeading")}</h1>
      <p>{t("welcome.postersExplain")}</p>
      <ol>
        <li>{t("welcome.postersStep1")}</li>
        <li>{t("welcome.postersStep2")}</li>
        <li>{t("welcome.postersStep3")}</li>
        <li>{t("welcome.postersStep4")}</li>
      </ol>
      <label>
        {t("welcome.postersField")}
        <input
          type="password"
          autoComplete="off"
          value={token}
          onChange={(event) => {
            setToken(event.target.value);
          }}
        />
      </label>
      {saving.problem === undefined ? null : <p role="alert">{saving.problem}</p>}
      <p>
        <button type="button" disabled={token.trim() === "" || saving.isBusy} onClick={save}>
          {t("welcome.postersSave")}
        </button>{" "}
        <button type="button" onClick={props.goOn}>
          {t("welcome.postersSkip")}
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
