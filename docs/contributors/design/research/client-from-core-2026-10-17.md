# Each core serves its own client: a design for D308

*2026-10-17. The agent's proposal for D308 §2's open design. Nothing here is decided until the
owner settles the questions in §6.*

## 1. What is there now

- **The discovery's page is the whole web client.** `orpheum-connect`'s `package.sh` copies
  Orpheum's `web/apps/web/dist` into the image. `connectd`'s `webclient` handler serves it at
  `app.orpheum-app.com`, adding the `orpheum-discovery-client-api` meta tag and its own CSP.
- **The page finds and reaches its core** (`discovery-start.ts`, the SDK's `discovery.ts`,
  `realm.ts`, `direct-path.ts`, `rtc-transport.ts` and `path-keeper.ts`):
  - It starts from a link fragment, a realm sign-in or a remembered core.
  - It races the direct names against WebRTC, and moves to a better path when one answers.
  - Every API call goes through one `fetch(Request)`.
  - Media, posters and subtitles are same-origin `/api/v1/…` loads, which the service worker
    passes to the page and the page sends over that same fetch.
  - Watch Together has its own upgrade channel.
- **The path is already authenticated.**
  - Over WebRTC, the core signs its answer with its identity key, and the page checks it against
    the fingerprint the link or the realm gave it.
  - Over a direct name, TLS for the core's own name under `orpheum-core.com` vouches for it.
- **The core serves its own client** at its own origin (`internal/core/webclient`). Over a data
  channel the whole mux is reachable, the static files included.
- **Nothing checks that the two match.** On a discovery page the client's own reload-when-outdated
  check is off (`app-update.ts`). The only coupling is the version pairing in `package.sh`.
- **One origin holds every household's credentials.** `app.orpheum-app.com` keeps the session
  of each core this browser has used, prefixed per core (`session.ts`), and the last core it
  reached.

## 2. The proposal

The discovery serves only the **loader**, a small page that changes rarely. It:

1. starts from the link, the realm or the remembered core;
2. reaches the core by the race it already runs;
3. loads the core's client over the path that won, through the service worker, which today
   already carries media that way. The client's scripts are then same-origin loads, so the CSP's
   `script-src 'self'` holds, and the client's split chunks and workers load as they do now;
4. hands the client one small, versioned interface. In outline: `fetch(Request)`,
   `openUpgrade(path)`, the path in use and its changes, the core's id and realm, and the start it
   came from.

The client then runs as it does on the core's own origin, sending everything through that
interface. The core needs no new endpoint: it already serves its client's files at `/` and
`/assets/…`.

The discovery's package then changes only when the loader or the discovery changes, and a core
runs the client it was built with.

## 3. The problem this opens: whose code runs on whose origin

Today every byte of script at `app.orpheum-app.com` is ours. With D308, each core supplies the
script that runs there, and any core can be claimed by anyone. A core's client at the shared
origin could read every other household's session from `localStorage`. A friend's core you joined
once for Watch Together could take your own household's session.

The transport check of §1 doesn't help. It proves the code came from that core, not that the core
is benign.

**So each core's client needs an origin of its own.** The loader at `app.` finds the core, then
sends the browser on to that core's own client origin, keeping the link's fragment. A copy of the
loader there reaches the core and loads its client. Storage, the service worker and the session
are then that one core's alone, as they are on the core's own name today.

There is a precedent: D292 put the cores' names under a separate registered domain,
`orpheum-core.com`, for the same kind of isolation.

That origin needs a name per core, served by the discovery:

- **Under `orpheum-app.com` at one level** (`<core>-app.orpheum-app.com`):
  - Free Cloudflare certificates cover one level (D292 §2).
  - But such a name is same-site with `api.orpheum-app.com`. A core's code would send the
    discovery's SameSite cookies with its requests to the discovery's API.
- **Under a new registered domain** (for example `*.orpheum-client.com`):
  - This gives the same isolation `orpheum-core.com` gives the cores.
  - It's another domain to buy and set up at Cloudflare.
- **Under `orpheum-core.com`** (`<core>.app.orpheum-core.com`, or similar):
  - The free certificates don't cover a second level.
  - Every core's code would also be same-site with every other core's name.

## 4. Smaller costs

- **Loading the client over WebRTC.** The bundle is about 1.2 MB (380 KB compressed). Its
  `assets/` names are content hashes, so the service worker can keep them in the Cache API per
  core. After the first visit only `index.html` comes over the path.
- **The loader's interface is a protocol**, as D273 §6's is:
  - It's versioned.
  - Changes are additive.
  - A breaking change raises the major version, and the loader keeps answering the previous ones
    for D273 §7's window.
  - A client that needs a newer loader says so, instead of failing obscurely.
- **What moves where.** `discovery-start.ts`, `discovery-gate.tsx` and the SDK's discovery,
  realm, direct-path, rtc-transport, rtc-media and path-keeper modules become the loader's: a new
  `web/apps/loader`, MPL-2.0 like the rest of `web/`. The client keeps a thin adapter that takes
  the loader's interface where it builds its fetch today.
- **Testing.** `through-discovery.spec.ts` already stands up a discovery and a core. The dev
  discovery would serve the loader and a per-core origin, both mapped by Playwright's host
  routing as `nameTheCoreDirectly` does.

## 5. What doesn't change

- The core's own address: the core serves its client there already.
- The discovery's protocol with the cores.
- The checks on the path (§1).

## 6. Questions for the owner

1. **A client origin per core (§3)?** I recommend yes. Without it, one bad core can take every
   other household's sessions on any browser that ever opened it.
2. **If so, under which name?** I recommend a new registered domain, for the same reason
   `orpheum-core.com` is separate. Using one level under `orpheum-app.com` costs nothing new but
   is same-site with the discovery's API.
3. **Keep each core's client in the browser's cache by its hashed names (§4)?** I recommend yes.
