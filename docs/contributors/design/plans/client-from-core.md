> **ABOUTME:** D308's build: app.orpheum-app.com serves a small loader, and each core's own client
> runs at that core's origin under orpheum-client.com, reached through the loader, checked by a
> manifest the core signs.

# Each core's client, behind a loader (D308)

- **Status:** BUILT — begun 2026-10-17 on the owner's word (*"agreed. Proceed."*), and built the
  same day: §3's four steps, live on nas.example since 2026-10-17 (`v0.2.0-nas.9`), with
  `prod-api` repackaged and `app.orpheum-app.com` serving the loader. Two follow-ups came the same
  night: the loader at `app.` never tries the LAN, and Forget asks first.
- **Depends on:** D308, D273, D292, D293, D297; [discovery-core.md](discovery-core.md);
  [webrtc-transport.md](webrtc-transport.md).
- **Blocks:** —

The build plan for [D308](../../decisions/working-notes.md#d308--apporpheum-appcom-serves-a-small-loader-and-each-core-serves-its-own-client).
The research behind it is
[client-from-core-2026-10-17.md](../research/client-from-core-2026-10-17.md), and the independent
audit, whose controls this builds, is
[client-from-core-security-audit-2026-10-17.md](../research/client-from-core-security-audit-2026-10-17.md).

## 1. The shape

- **`app.orpheum-app.com` serves the loader.** It does the first contact and nothing more:
  - it opens a link, or signs in by realm;
  - it keeps the household list (D308 §8);
  - it hands the browser on to the core's own client origin.

  It keeps no core's session (D308 §3).
- **`<coreId>.orpheum-client.com` is that core's client origin.** The discovery serves the loader
  there too. At that origin the loader:
  - takes the core id from its own hostname and refuses any other;
  - reaches the core by the race it runs today;
  - fetches the core's signed client manifest and checks it;
  - loads the client's files through its service worker, keeping them by their hashed names;
  - hands the client one versioned interface.

  Staging's client origins are `<coreId>.orpheum-site.com`, staging's own client zone, one level
  down, which the free certificate covers (D308 §6), all of them behind one wildcard route on
  staging's tunnel (D319; until 2026-10-19 `<coreId>-staging.orpheum-client.com`, a route by hand
  for each core id, which every wipe of staging changed).
- **The core's own address is unchanged.** At home the core serves its client itself, as today.

## 2. What each side builds

### 2.1 The core (`orpheum`)

1. **Two origins named by the discovery.** `discoveryproto.StatusResponse` gains `LoaderOrigin`
   (additive, D273 §6), and `ClientOrigin` becomes the core's own client origin.
   - The core's CORS allows the client origin for its whole API.
   - It allows the loader origin only for `/health`, which the path race probes, and for the
     hand-off code (2.1.2).
   - Links the core makes lead straight to the client origin. `accounts.go` already builds them on
     `ClientOrigin`.
2. **The hand-off code** (D308 §4).
   - `POST /session/handoff`, with a name, a password and a device, answers a code: 128 random
     bits, one use, a life of 60 s, kept in memory. No session is made. It is limited as a sign-in
     is.
   - `POST /session/redeem`, with the code and a device, answers what `POST /session` answers. It
     makes the device then, at the client origin, where the session will be kept. Whether the
     session administers is decided at redemption, by D297 §6's rule, from where that request came.
   - A redeemed or expired code is refused, and so is a second redemption.
   - **Reaching the core before the code is taken.** The client origin finds the core with the
     bootstrap token `app.`'s realm sign-in got from the discovery, which the redirect carries
     beside the code. That token ends in five minutes and lets it reach no further than the core's
     paths and offers.
     - Over a direct name it needs nothing more.
     - Over WebRTC it proves its offers under `proof.HandOffOfferKey(code)`. The core accepts that
       key for the person the realm names, beside their realm key, while the code is live, so a
       core behind carrier NAT can still be reached.
     - Once the code is taken, the session's own locator and key take over, as today.
3. **The signed client manifest** (D308 §6).
   - `make generate` writes, beside the embedded `dist/`, the client's version and each file's
     path and SHA-256.
   - `GET /client-manifest`, unauthenticated, answers that manifest's bytes and their Ed25519
     signature under the core's identity key.
   - The loader believes the key only when its fingerprint is the one the link or the realm gave
     (`proof.Fingerprint`), as it already does for a WebRTC answer.
4. **The byte cookies** (done): a byte path takes a cookie only from the core's own
   pages, and over TLS only by its `__Host-` name.

### 2.2 The web (`orpheum/web`, MPL-2.0)

1. **`web/apps/loader`**, built apart from the client. *(As built, one build is both, §2.2a; there is
   no `web/apps/loader`.)* These modules move out of the client:
   `discovery-start.ts`, `discovery-gate.tsx`, the realm sign-in, and the SDK's discovery, realm,
   direct-path, rtc-transport, rtc-media and path-keeper modules, which both may import. Its
   service worker replaces `sw.js`.
   - **At `app.`:**
     - the household list: the last used first; straight to the core when it knows one, unless
       the address asks for the list; a way to the realm sign-in; Forget on each entry;
     - the link and realm sign-ins, which end in a hand-off code;
     - the redirect, built only as `https://` + a checked core id + the client domain, never from a
       URL it was given;
     - on its first run, it deletes the session keys the whole-client `app.` kept.
   - **At a client origin:**
     - the core id from the hostname;
     - the fingerprint from the fragment, remembered;
     - the path race;
     - the hand-off code redeemed;
     - the manifest checked: its signature, its version against the discovery's floor, and each
       file's hash before the service worker serves it;
     - the client run.
   - **At both, at its start:** it deletes every cookie it can see, and every storage key under
     `orpheum.` that the list of keys it owns does not name (the owner, 2026-10-17: *"ok, do
     so"*).
2. **The loader's interface**, `window.orpheumLoader`. Major version 1. Changes to it are
   additive, and a breaking change raises the major version, in D273 §6's way. It carries:
   - `fetch(Request)`;
   - `openUpgrade(path)`;
   - the path in use and its changes;
   - the core's id, realm and fingerprint;
   - `switchHousehold()`, which goes to `app.` asking for the list.

   A client that needs a newer major than the loader has says so, and does not fail obscurely.
3. **The client.** `main.tsx` takes the loader's interface where it builds its fetch today:
   - Its discovery branch shrinks to an adapter.
   - Its menu offers **Switch household** whenever a loader runs it.
   - It runs on the core's own origin without a loader, as now.
   - It never writes `document.cookie`, a lint holds that, and the client keeps one list of the
     storage keys it owns.

### 2.2a How the loader works

- **One build, two roles.** `web/apps/web`'s one build is both the core's client and the
  discovery's loader, so the loader imports the gate's modules as they are. `main.tsx` takes the
  page's word for which it is: a discovery's page names the loader's configuration in meta
  elements, and a client a loader runs finds the loader on the window. Each then loads only its own
  part. A build mode would have been the client reading its environment, which D204 forbids. The
  discovery's package carries the same `dist`, with the meta elements put in its page.
- **What a redirect carries.** Every fragment names the core whose origin it opens, and the loader
  refuses one that names another:
  - an invitation, a person's own link or a Watch Together link: the link's fragment as today,
    `#<coreId>.<secret>.<fingerprint>`, on the client origin's page the link names;
  - a realm sign-in's hand-off:
    `#handoff=<code>&bootstrap=<token>&fingerprint=<fingerprint>&realm=<realm>`.

    The code is used once, the bootstrap token ends in five minutes, and the loader clears the
    fragment as soon as it reads it.
- **Session storage.** At the client origin the loader keeps the session, the device and the
  locator, under `orpheum.core.<coreId>.` as `session.ts` does today. The client reads and writes
  them only through the interface's `session`, so the two never disagree about the storage.
- **Running the client.**
  1. Once a path answers, the loader fetches `/client-manifest` over it and checks:
     - the key's fingerprint against the one the link, the hand-off or this origin's memory named;
     - the signature;
     - the version against the floor;
     - then the client's `index.html`, by its hash.
  2. It gives the checked manifest to its service worker.
  3. It copies the client's `<link>` and `<script>` elements into its own document.
  4. The service worker answers every `/assets/…` load from its cache, where files are kept by
     their SHA-256. On a miss it asks the page to fetch the file over the kept path, checks the
     hash against the manifest, keeps it, and answers it. A file whose hash does not match is
     never answered.
  5. The loader's own files are also under `/assets/`. A file the client's manifest names comes
     from the core, checked. Any other comes from the discovery, kept by the worker beside the
     loader's page, but only once a loader has named a client's files to it. At a core's own
     origin, the worker answers neither.
- **The household list** (D308 §8) lives in `localStorage` at `app.`: each entry has the core id,
  fingerprint, realm and name, and when it was last used.
  - A realm sign-in's hand-off adds its core.
  - **Switch household** at a client origin goes to `app.`'s list, naming the core it came from.
    `app.` adds that core only when `document.referrer` is that core's client origin, which the
    link sends with `referrerpolicy="origin"`. So no other page can put an entry in the list.
  - A household joined by a link appears there after its first Switch.
- **At `app.`, its first run** unregisters the whole client's old service worker and deletes the
  keys it kept.

### 2.3 The discovery (`orpheum-connect`)

1. **The loader at `app.` and at the client domain.**
   - `ByHost` routes the client domain's names to the loader.
   - Each client name is checked as one core id's.
   - It sends no `Set-Cookie`, which a test holds.
   - It adds `Origin-Agent-Cluster: ?1` and `Cross-Origin-Opener-Policy: same-origin` to the CSP it
     sends.
   - Each origin's `connect-src` names the core's zone and the discovery's API.
2. **The two origins in the status.** For each core, `ClientOrigin` is
   `https://<coreId>.orpheum-client.com`, and `LoaderOrigin` is `app.`.
3. **The version floor**: `GET /client/v1/client-floor`, the client versions refused, set by the
   operator.
4. **`package.sh`** packs the loader's `dist`, not the client's. Repackaging is then needed only
   when the loader or the discovery changes.

### 2.4 Tests

- `dev discovery` (`cmd/orpheum/dev_discovery.go`) serves the loader at a loader origin and at
  client origins, both mapped by Playwright's host routing.
- `through-discovery.spec.ts` covers every path through the loader:
  - a link;
  - a realm sign-in with its hand-off;
  - WebRTC and direct;
  - Watch Together;
  - reconnects;
  - the household list;
  - a manifest that fails its check, which loads nothing;
  - a floor that refuses the core's version.

### 2.5 The owner

At Cloudflare:
- `orpheum-client.com`'s Universal SSL on;
- a proxied wildcard record to `prod-api`'s tunnel, and its ingress;
- for staging, the same pointing at staging's tunnel, for the `-staging` names.

## 3. Order

1. The core: §2.1 1–3, each with its tests. Nothing yet uses them.
2. The loader and the client's adapter, against `dev discovery` (§2.2, §2.4).
3. The discovery (§2.3), then staging through the new domain, once the owner's records exist.
4. Production: `prod-api` repackaged, nas.example updated. Every session starts again (D308 §5).
