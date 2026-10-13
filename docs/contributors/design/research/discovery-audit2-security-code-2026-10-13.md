> **ABOUTME:** The second-round security audit, 2026-10-13: a line-by-line review of the
> security-relevant code already built (auth, the API, Watch Together, TURN, rtcapi, the core's
> discovery client, the single-target discovery, the web client's token handling), at an earlier commit,
> with probes run outside the repository. Findings say whether they are exploitable today on the
> internet-facing staging server or only once a discovery runs.

# Security audit, round 2: the code as built (2026-10-13)

## Summary

The code was read, not its comments. Each claim marked **[V]** was confirmed by reading the code
or a library's source, or by a probe (a throwaway test in a copy of the tree under the session's
scratch directory, never in the repository). **[I]** is inferred and not run.

Staging today: the VPS forwards TCP 443 (IPv4 and IPv6), TURN and the relay ports into the tunnel
by DNAT, without rewriting the source (`deploy/vps/orpheum.nft`). The core reads the client's
address from the TCP connection only (`api/session.go:86`); no header is trusted. So no client can
spoof its address with `X-Forwarded-For` or the like. **[V]** But a real attacker has many real
addresses: any IPv6 host has a /64, and a /48 is cheap.

**Exploitable today, most severe first:**

- **K1 (high).** A sign-in or a device activation may name any of the person's device ids, and the
  new session then lives on that device. It is not listed as a new device, and "sign out everywhere"
  keeps it, because it keeps every session of the asking device. A password change or reset ends no
  session. Someone who once had the password keeps a hidden session that the person's tools cannot
  find or end, short of removing their own device. Probe confirmed every step.
- **K2 (medium).** Anyone can lock any account out by name: five failures in 15 minutes, from any
  addresses, refuse even the right password. The per-address key is a whole IPv6 address, so it
  limits nothing for an IPv6 attacker. The check-then-record limiter lets a burst of concurrent
  guesses through (53 of 60 in the probe).
- **K3 (medium).** Every sign-in attempt, known name or not, runs argon2id with 64 MiB, with no bound
  on how many run at once. A few hundred concurrent unauthenticated `POST /session` exhaust memory.
- **K4 (medium).** The idempotency cache keeps an entry for every POST with an `Idempotency-Key`,
  signed in or not, on any path, for 10 minutes, with no size bound. Each POST scans the whole map
  under one global mutex. At 20,000 entries each POST spent 0.29 ms in the scan (probe). That is
  memory growth and a server-wide stall that one unauthenticated client can cause.
- **K5 (medium).** Device activation: the global cap of 100 waiting pairings fills from 20
  networks (5 each). Then no television anywhere can get a code. Repeat every 10 minutes.

**Only once a discovery runs:** the single-target discovery (`discovery/single`) does not bind a
session token to a core. A stranger who enrolled first, or who enrols while the owner's core is
pending, can take over the signalling (and so man-in-the-middle every client's WebRTC connection
and harvest session tokens), set the ACME record for the owner's core's name, overwrite its
addresses, or delete it (**K13, critical**). The owner's core can also be shown the stranger's claim
code and claim the stranger's core (**K14**), and an unclaimed core can get certificates under the
owner's zone (**K15**). All three were confirmed by probe. Offers that come through a discovery are
answered with no sign-in, and rtcapi has no per-client bound, no idle timeout, and no bound on the
channels per connection or on how long a request body may sit half-sent (**K16**).

## Findings

### K1 — high — A session hides under another device of the person's and survives "sign out everywhere"; a password change ends nothing

- **Where:** `internal/core/auth/auth.go:287` (`resumeOrMakeDevice`), `auth.go:216` (`SignIn`),
  `pairing.go:143` and `pairing.go:231` (`RequestPairing`, `ClaimPairing` take
  `previousDeviceID` from the device not signed in), `devices.go:96` (`SignOutElsewhere`),
  `internal/store/queries/device.sql:36`, `accounts.go:171` (`ChangePassword`), `accounts.go:115`
  (`Register` with a registration link).
- **What the code does:** a client names `previousDeviceId`, a small sequential integer. If that is a
  live device of the same person, the new session's grant gets that `device_id`, and the device's
  platform is overwritten. Nothing proves the client was ever that device. `SignOutElsewhere`
  revokes every session whose `device_id` is not the caller's device. `ChangePassword` and the
  registration link only rewrite the hash. **[V]**
- **Exploit:** an attacker who learns the password once (phishing, reuse, a shoulder) signs in. They
  read the device ids from `GET /devices`, sign in again naming the victim's phone's id, and remove
  their first device. The victim's device list now shows only their own devices. When the victim
  grows suspicious, changes their password and presses "sign out everywhere" on that phone, the
  attacker's session survives both. Probe (`auth` package): the attacker's session was on device 1
  (the victim's), the list held one device, and `Authenticate` still succeeded after
  `ChangePassword` and after `SignOutElsewhere(victim's device)`. **[V]**
- The pairing path gives the same through social engineering. A stranger's TV asks for a code with
  `previousDeviceId` set to a guess of the victim's TV's id. When the victim enters the code with the
  name left empty, the claim answers with the victim's own device's name ("Living room TV" in the
  probe), which makes a "please enter this code" message look legitimate. Once it is confirmed, the
  stranger's session sits on the victim's TV's device. **[V]**
- **Roles:** a paired session carries the person's whole role, admin included (`issueSession`, scope
  `role`). An admin activating a TV for themselves leaves an admin session on the TV. **[V]**
- **Today:** yes. It needs the password once, or one tricked activation.
- **Fix:** resume a device only on proof. Give each device a random secret at its first sign-in,
  kept by the client beside its id and stored hashed on the device row, and resume only when the
  client presents it. Never resume a device that has a live session of another client. Make
  `SignOutElsewhere` keep only the caller's session (its `SessionKey`), not its device. Have a
  password change and a reset end every other session by default, with D218 §6's separation as an
  opt-out rather than the default. List sessions, not only devices, with their last address.

### K2 — medium — Anyone can lock any account out; the address limit is per IPv6 address; a concurrent burst passes the limiter

- **Where:** `internal/core/auth/auth.go:220-243`, `limiter.go:27-44`.
- **What the code does:** the keys are `account:<name>` and `address:<full client address>`. A
  request is refused once either key has 5 failures in 900 s (settings `signin_failure_limit`,
  `signin_failure_window_seconds`). `isLimited` is checked, then the hash runs, then
  `recordFailure`, with the mutex let go between them. **[V]**
- **Exploit:** five wrong passwords for "karl" from five addresses, every 15 minutes, keep the owner
  out. Probe: after five failures from five IPv6 addresses, the right password from a fresh address
  was refused. The host's name shows in every Watch Together roster. With IPv6, the address key never
  binds, because every request can come from a new /128. Sixty concurrent wrong guesses for one
  account were all checked against the password, bar 7 (probe, at the tests' cheap argon2 costs;
  real costs widen the window). **[V]**
- **Today:** yes.
- **Fix:** key addresses by `clientnet.Of` (the /64). Reserve the attempt under the mutex before
  hashing (count it as a failure, refund it on success), so the limit holds under concurrency. Do
  not refuse a correct password outright: limit per (account, network), plus a slow global
  per-account delay; or keep the lockout but let a device that has signed in before (a device
  secret, as in K1) bypass it.

### K3 — medium — argon2id runs unbounded in parallel, unauthenticated

- **Where:** `internal/core/auth/auth.go:230-237`, `password.go:36,54`;
  `golang.org/x/crypto@v0.57.0/argon2/argon2.go:163` (`make([]block, memory)`).
- **What the code does:** each sign-in hashes or verifies with `m=65536` KiB (the setting's starting
  value), an unknown name too, for constant timing. Nothing bounds concurrency. The limiter does not
  help: each concurrent request passes the check before any failure is recorded (K2), and different
  names and addresses have different keys anyway. **[V]**
- **Exploit:** N concurrent `POST /session` allocate N × 64 MiB; 100 is 6.4 GB. **[I]** (not run at
  full cost).
- **Today:** yes, from the internet.
- **Fix:** a semaphore around argon2id (a few at a time, sized by memory); past it, answer 429 or wait
  with a short timeout. A global rate on unauthenticated sign-ins.

### K4 — medium — The idempotency cache grows without bound and is scanned whole on every POST

- **Where:** `internal/core/api/idempotency.go:51-91` (`idempotent`), `:95-117` (`take`).
- **What the code does:** for any POST with an `Idempotency-Key`, signed in or not, on any path
  (404s too, since it wraps the mux), it reads up to 1 MiB of the body and keeps an entry for 10
  minutes. Every `take` walks every entry to expire old ones, holding the one mutex every POST
  needs. **[V]**
- **Exploit:** a client sends POSTs with random keys. The probe filled 20,000 entries in 2.7 s, and
  each later POST then spent 0.29 ms in the scan. The cost grows linearly, so 200,000 entries is
  about 3 ms, serialised across the whole server: every sign-in, playback start and activation
  waits. **[V]**
- **Today:** yes.
- **Fix:** apply it only to signed-in requests on the operations that need it. Bound the entries (per
  caller and in total). Expire through a time-ordered list or a periodic sweep, not a full scan per
  request.

### K5 — medium — Device activation's global cap fills from 20 networks

- **Where:** `internal/core/auth/pairing.go:21-31,129-137`.
- **What the code does:** at most 100 pairings in total, and 5 not yet entered per network (an IPv4
  address or an IPv6 /64). **[V]**
- **Exploit:** 20 networks × 5 requests fill the 100. A television then gets "too many devices are
  waiting" for 10 minutes, and the attacker repeats. Probe: after 20 /64s took 5 each, a new address
  was refused. **[V]** D288 §1 says the core "limits nothing here" because "it's just the admin's own
  server and VPS, which needs no protection". But the staging core is that server, and its
  `POST /pairings` is open to the internet.
- **Today:** yes.
- **Fix:** a pairing is about 200 bytes, so raise the global cap to thousands. Cap per /48 for IPv6
  as well as per /64. Count claimed pairings in the per-network cap too.

### K6 — low — Changing a password has no failure limit

- **Where:** `internal/core/auth/accounts.go:171-191`.
- **What the code does:** a wrong current password returns `ErrWrongPassword` and records nothing.
  **[V]**
- **Exploit:** a stolen session (a shared TV left signed in, see K1's roles) can guess the
  account's password online, at argon2's pace, without limit, and then take the account fully.
- **Today:** yes, with a session.
- **Fix:** count wrong current passwords against the account's key, as a sign-in does.

### K7 — low — pprof would be public over HTTPS and WebRTC when the HTTP listener is loopback

- **Where:** `cmd/orpheum/serve.go:537-566` (`mountDebug`), `serve.go:56-60`, `https.go:163`.
- **What the code does:** pprof is mounted on `server.Mux()` when `should_serve_debug_pprof` is on and
  `server.listen` is loopback. The same mux is served by the HTTPS listener (`https.listen`, any
  address) and by rtcapi's data channels. **[V]**
- **Exploit:** `server.listen = "127.0.0.1:8440"`, `https.listen = ":443"`, pprof on: then
  `/debug/pprof/` (cmdline, a 30 s CPU profile, trace) is unauthenticated on the internet. Staging's
  `server.listen` is `0.0.0.0:8440`, so it is not exposed today. **[V]**
- **Fix:** serve debug endpoints on their own loopback listener, never on the API's mux.

### K8 — low — The web client sets no CSP, no frame-ancestors, no HSTS; the session token is in localStorage

- **Where:** `internal/core/webclient/webclient.go:21` (sets `nosniff` only),
  `web/apps/web/src/session.ts`, `main.tsx:39`.
- **What it means:** any XSS reads the bearer token, which is the full session (the media cookie
  carries the same token). No XSS sink was found: React escapes, and there is no
  `dangerouslySetInnerHTML`. **[V]** The device activation page can be framed. Browsers'
  third-party storage partitioning should keep a framed copy signed out, which blunts clickjacking.
  **[I]**
- **Fix:** a strict CSP (`default-src 'self'`, `frame-ancestors 'none'`), and HSTS on the HTTPS
  listener.

### K9 — low — The TURN relay is an open UDP relay for any member, to any public address, the VPS's own included, with no allocation quota

- **Where:** `internal/core/turn/relay.go:110-125` (`mayRelayTo`), `:77-85` (no `QuotaHandler`);
  pion `turn/v4@v4.1.4/internal/server/turn.go:303,408` (the permission handler gates
  CreatePermission and ChannelBind). **[V]**
- **What it means:** a guest let in by a link gets 24-hour credentials. While the membership lasts,
  they can send UDP from the VPS's address to any global address. Ports 49160–49199 give 40
  allocations in total, so one member can take them all and voice relaying stops for everyone. The
  VPS's public IPv4 is global unicast and allowed. Packets to it on ports other than the relay range
  reach the VPS's own UDP services from 10.77.0.2. **[I]** An allocation made before an ejection
  lives until it lapses, because pion re-authenticates only Allocate and Refresh. **[I]**
- **Fix:** a `QuotaHandler` (an allocation or two per membership). Refuse the VPS's own address
  except on the relay range. Shorter credentials (an hour, re-asked).

### K10 — low — An admin's activation as another person is not logged

- **Where:** `internal/core/api/pairing.go:145-166`.
- **What it means:** an admin may enter a code for any user, another admin included, and the device
  is signed in as them. The handler logs nothing of who did it for whom. **[V]**
- **Fix:** log `byUserId`, `forUserId` and the device at Info, as Watch Together's handlers log
  their acts.

### K11 — low — `persistent.db` is mode 0644

- **Where:** staging's `.staging/core/persistent/persistent.db` (and its `-wal`, `-shm`).
- **What it means:** it holds the password hashes, grant hashes and the sealed secrets. The directory
  is 0700, so today nothing else reads it. **[V]** A copy made out of the directory keeps the wide
  mode. **[I]**
- **Fix:** open the database with a 0600 umask.

### K12 — low — Watch Together: a guest's name is anything; one member's devices are unbounded

- **Where:** `internal/core/together/hub.go:366-386` (`Join`), `:479-514` (`Attach`).
- **What it means:** a guest may join as the host's name. A holder of a credential may attach any
  number of devices, and each attach broadcasts the roster to all of them. **[V]** It needs a link
  the host made.
- **Fix:** mark the host in the roster (the client does, by `isHost`), and cap the devices per member.

### K13 — critical (later) — Single-target discovery: a session token is not bound to its core, so a stale token takes over the claimed core

- **Where:** `discovery/single/single.go:97` (`tokens map[string]time.Time`), `:306-316`
  (`isSignedIn`), `:122-126` and `signal.go:47-68` (signalling accepted on any token, replacing the
  current connection), `:375-403` (ACME), `:353-372` (addresses), `:406-416` (leave).
- **What the code does:** `session` checks the signature against the core held now, then stores the
  token with only an expiry. Every later call checks only that the token exists and has not expired.
  **[V]**
- **Exploit:** the attacker enrols while the record is empty or pending and opens a session (token
  valid 1 h). The owner's core enrols, replacing it, and the owner claims it. With the stale token,
  the attacker can then (probe, all succeeded): read the claimed core's status; set
  `_acme-challenge.<owner's core id>.<zone>` (a certificate for the owner's core's names); overwrite
  its addresses; take the signalling connection, after which the client's offer was answered with
  the attacker's SDP; and `DELETE /core`, after which the record held no core. **[V]** Holding the
  signalling means every client connecting through the discovery reaches the attacker. The attacker
  then reads each request's `Authorization` header over the data channel: session tokens.
- **Today:** no. `discovery/single` is imported by nothing but its tests. **[V]**
- **Fix:** store the core id (and its public key) with each token, and check that it is the record's
  core on every call. Drop all tokens when the record's core changes. Accept the signalling only for
  the claimed core's token. Prune expired tokens.

### K14 — high (later) — Single-target discovery: a stranger's enrol replaces the owner's pending core, whose status then shows the stranger's claim code

- **Where:** `discovery/single/single.go:227-271` (`enrol`: a pending core is replaced by any key),
  `:319-350` (`status` answers with the record's core, whatever the token's core).
- **Exploit:** the owner's core enrols and shows its claim code. A stranger enrols. The owner's core
  asks for its status with its own still-valid token and is told the stranger's code (probe: owner's
  `cfz9-1mq0`, shown `kfv9-wcd1`, the stranger's). The owner types it into
  `orpheum discovery claim`, which claims the stranger's core into the owner's zone. **[V]**
- **Fix:** with K13's binding, a token whose core was replaced gets 401, and the core enrols again.
  Better: let a pending core be replaced only by `orpheum discovery` on the discovery's machine, or
  only once its claim code expires.

### K15 — medium (later) — Single-target discovery: an unclaimed core sets ACME records under the owner's zone

- **Where:** `discovery/single/single.go:375-403`. The name check uses `record.Core.ID` whatever the
  state.
- **Exploit:** anyone enrols (pending), opens a session, and sets
  `_acme-challenge.<their id>.<zone>`. Probe: 200, record set. **[V]** With a DNS provider configured,
  they get a valid certificate for `*.<id>.<owner's zone>`: phishing under the owner's domain.
- **Fix:** ACME (and addresses) only for a claimed core.

### K16 — medium (later; low today) — rtcapi: offers through a discovery are answered without sign-in, and nothing bounds a client

- **Where:** `internal/core/discovery/signal.go:84-97`; `internal/core/rtcapi/rtcapi.go:104-140`;
  `channel.go:64-148`.
- **What the code does:** `MaxConnections` (64) is global, with no per-client bound. A connection is
  forgotten only when it fails or closes. Pion's ICE fails an unconnected one after
  disconnected + failed timeouts (about 30 s; `ice/v4@v4.4.7/agent.go:686,741-752`), but a connected
  one stays as long as its keepalives answer. Every data channel starts a request. Its body is
  buffered up to 1 MiB until the empty message ends it, with no deadline: the goroutine waits on
  `reader.done` or the channel's close. The number of channels per connection is not bounded. **[V]**
- **Exploit:** today `POST /rtc/offer` needs a sign-in, so only a member of the household can hold
  the 64 connections or buffer 1 MiB × many channels. **[V]** Through a discovery, anyone who knows a
  core's locator can, unauthenticated: hold all 64 connections, so no family member's client
  connects, or exhaust memory with half-sent bodies. **[I]** Each offer's candidates also make the
  core send STUN checks to addresses the attacker lists, LAN ones included. **[I]**
- **Fix:** a per-source cap on connections. A connection that sends no signed-in request within some
  seconds is closed. Cap channels per connection and requests in flight. Put a deadline on a
  request's head and body.

### K17 — low (later) — Single-target discovery: maps that only grow, and a write per enrol

- **Where:** `discovery/single/single.go` `tokens` (never pruned) and `spent` (pruned only on a
  signature check); each `enrol` writes the record file.
- **What it means:** challenges keep nothing, as the comment says. But anyone can make a key, sign
  nonces and enrol: each enrol spends a nonce (kept 5 min) and rewrites the record. While the
  attacker is the pending core, it can open sessions without end, and each token is kept forever.
  **[V]**
- **Fix:** prune the tokens; bound enrols per source address.

### K18 — low (later) — The discovery's record is written in place; its mode is set only at creation

- **Where:** `discovery/single/record.go:88-100`.
- **What it means:** `os.WriteFile` truncates and writes the same file. A crash mid-write leaves it
  unreadable, and the discovery then fails every call until someone repairs it. A file that already
  exists keeps whatever mode it has. **[V]**
- **Fix:** write a temporary file at 0600, fsync it, and rename it over the record.

### K19 — low (later) — Over WebRTC, the client's address is its ICE candidate's

- **Where:** `internal/core/rtcapi/rtcapi.go:191-201`, `channel.go:190`.
- **What it means:** a client relayed by a TURN server shares the relay's address with every other
  client relayed by it. Then the per-network pairing cap, the sign-in limiter's address key and the
  playback speed keys are shared too. When no pair is selected, the address is `""`, one network for
  all. **[V]** Spoofing is not possible: ICE checks prove the address. **[I]**
- **Fix:** for relayed candidates, key the limits by the authenticated principal, or by the address
  the discovery saw the client at.

### K20 — low (later) — UPnP: the router's description may send the core's control requests anywhere

- **Where:** `internal/core/portmap/upnp.go:75-125,166-190`.
- **What it means:** the SSDP answer's sender must be the gateway. But the `Location`, `URLBase` and
  `controlURL` it names may be any `http` host, so a device on the LAN that spoofs the gateway's
  address over UDP directs the core's SOAP POSTs. It runs only while a discovery publishes the core.
  **[I]**
- **Fix:** accept only a location and control URLs on the gateway's own address.

## Checked and sound

- **Tokens and secrets:** session tokens, registration and invitation secrets, pairing secrets,
  Watch Together links and credentials are each 256 random bits (`randomSecret`,
  `randbytes.Base64URLSecret`). The database keeps only a session's SHA-256. Link and credential
  comparisons hash, then compare in constant time. **[V]**
- **Authenticate:** the grant's kind and user are checked, idle expiry is enforced, and the user,
  role and reach are re-read on every request. A removed person's sessions fail. **[V]**
- **Address:** `ClientFacts.Resolve` uses `RemoteAddr` alone, and the VPS's DNAT keeps the source.
  No proxy header is read anywhere in `internal/` or `cmd/`. **[V]**
- **Pairing logic:** `TakePairedSession` deletes under the mutex, so it is once only, and the secret
  is unguessable. A claim and an answer re-check `isClaimed`, `byUserID` and `isConfirmed` under the
  mutex, so two answers cannot both confirm, and only the person who claimed may answer. A non-admin
  cannot claim for someone else (`api/pairing.go:155`). A `previousDeviceID` of another person's
  device makes a new device, never a cross-person takeover (K1 is within one person). Codes are 40
  bits with at most 100 live, and a person's wrong codes are limited, so guessing a code is
  hopeless. The pairing lifetime is fixed from its making. **[V]**
- **Setup:** a 60-bit code compared in constant time; the user count is checked in the same
  transaction that inserts. **[V]**
- **Invitations:** spent in one write transaction, scope- and kind-checked. **[V]**
- **Media:** the byte paths re-check the holder on every request. A viewer's holder is the host's
  reach for the session's current film only (`OnlyFileID`), and media tokens are bound to the
  session key. The cookies are HttpOnly, SameSite Strict, path `/api/v1/bytes/`, and Secure over
  TLS. The API never reads the cookie. **[V]**
- **Watch Together:** only the host may open links, eject, end, change the item, stop or change the
  rate. Guests have only their link's rights. A change of item is checked against the host's reach.
  Voice signalling is relayed only within a session. The WebSocket's Origin is checked
  (`websocket.Accept` with nil options), messages are capped at 64 KiB, and the hello must come
  within 10 s. **[V]**
- **TURN:** the HMAC credentials name a membership; the relay checks the expiry and liveness on
  every authenticated request; private, CGNAT, link-local, loopback and reserved peers are refused.
  **[V]**
- **rtcapi:** requests go through the same handler (auth, idempotency, huma's body limits). Paths
  must start with `/`, and an absolute URL is refused by `ParseRequestURI` plus the prefix check.
  **[V]**
- **Core's discovery client:** HTTPS only (loopback HTTP excepted). Answers are capped at 1 MiB and
  signalling messages at 64 KiB. Go drops `Authorization` on a cross-host redirect. Admin-only
  endpoints. **[V]**
- **Identity key:** written 0600 in a 0700 directory, and the seed is checked. Signatures are
  domain-separated by purpose (`proof.Message`). **[V]**
- **Single discovery's nonces:** expiry plus 16 random bytes, under a 128-bit truncated HMAC-SHA256
  with a 256-bit key made per process. Expiry is checked; one valid use, then kept spent until the
  expiry; a restart voids every outstanding nonce. Sound as designed. **[V]**
- **Web:** registration and Watch Together secrets ride in the fragment and are replaced in history
  after use. No open redirect: navigation is to named screens. **[V]**

## Errors in comments, invariants or docs

- `internal/core/auth/doc.go:25` says "Failed sign-ins are limited per account and per address". The
  address is a whole IPv6 address, and concurrent attempts pass (K2).
- `internal/core/auth/pairing.go:28-30` says the caps are "a bound on memory and on one network, not
  on who asks". Twenty networks deny everyone (K5).
- D288 §1 says the core "limits nothing here" since its server and VPS need no protection. The
  staging core is directly on the internet (K5).
- D288 §4 and D221 make resuming a remembered device the rule, but no proof is asked for (K1).
- `internal/core/rtcapi/doc.go`: "the offer endpoint answers only a signed-in client". True of
  `POST /rtc/offer`; the discovery's offers are answered with no sign-in (`discovery/signal.go:28`).
- `internal/core/api/together_socket.go` (`channelSocket`): "a message is at most
  rtcapi.MaxMessageBytes". Nothing in rtcapi enforces it on receive. Pion's SCTP receive buffer is
  the only bound. **[I]**
- `cmd/orpheum/serve.go:537`: "only on a loopback listener". It checks `server.listen` only, while
  the same mux is served on HTTPS and WebRTC (K7).
- `internal/core/turn/doc.go`: "never to the server's own machine". The VPS's public address is
  allowed (K9).
- `discovery/single/single.go:33-36`: "a stranger asking for challenges ... fills no table". True of
  challenges; `spent` and `tokens` still grow (K17). And `enrol`'s "While its core is claimed it
  refuses any other key" is defeated by stale tokens (K13).
- `discovery/single/record.go:43`: "mode 0600". Set only when the file is created (K18).
