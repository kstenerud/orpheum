> **ABOUTME:** Identity, sessions and signing in — including on a television, which is the
> first thing every user does on the device with the worst text entry ever shipped.

# Authentication and identity

- **Status:** PLANNED — pairing shape decided 2026-09-19 ([D93](../../decisions/working-notes.md), [audit-2026-09-19.md](../research/audit-2026-09-19.md) §15); ~~the token model remains [threat-model.md](threat-model.md)'s open question~~ **the token model settled by [D154](../../decisions/working-notes.md)** (one grant table, opaque and revocable). **In Cut A** ([D178](../../decisions/working-notes.md)): local accounts, sessions and the invite from outside; **no OIDC/LDAP, and no television pairing** — her television joins a Watch Together session as a device of her membership instead ([D191](../../decisions/working-notes.md), 2026-09-23).
- **Depends on:** —
- **Blocks:** [library-model.md](library-model.md) — the user and identity tables are schema

## Why this document exists

Because it did not, and it is a Phase 1 subsystem. Identity, sessions, token scope and
lifetime, revocation, device pairing and the sharing hand-off were spread across a bullet in
[library-model.md](library-model.md), two open questions in [threat-model.md](threat-model.md),
and a section in [remote-access.md](remote-access.md). None of those owns it.

[D20](../../decisions/working-notes.md) already promoted **OIDC/OAuth and LDAP into the core**
for Phase 1, on the evidence that a third-party Jellyfin SSO plugin has 1,458★ and Navidrome's
two most-requested issues are the same two things. Retrofitting external identity is painful;
so is retrofitting anything else on this page. **Both are out of Cut A**
([D178](../../decisions/working-notes.md) §4), which has two users and local accounts.

## What Cut A needs from this page

[D178](../../decisions/working-notes.md) (F6, F90) and [D191](../../decisions/working-notes.md):

- **Server-local accounts and sessions**, on D154's grant table below.
- **The invite from outside**: the remote person gets an invited account
  ([D174](../../decisions/working-notes.md)), and what it may see is asked when the invite is
  made ([D184](../../decisions/working-notes.md)), never defaulted.
- **No television pairing.** RFC 8628 below (D93, F30) waits until she wants to watch on that
  television alone. In a Watch Together session her television joins by the session link or a
  short code from her phone, as a device of her membership, and needs no sign-in of its own
  ([D191](../../decisions/working-notes.md)).
- **Settings and provider credentials live in the database.** Whatever changes at runtime is a
  row in the `setting` table, except that a credential — the TMDB key, the server's
  OpenSubtitles login ([subtitle-acquisition.md](subtitle-acquisition.md)) — is a sealed row in
  the `secret` table ([D194](../../decisions/working-notes.md)). Both live
  ([schema.md](schema.md), [D181](../../decisions/working-notes.md)). Every secret is the
  server's: an admin enters the OpenSubtitles login once and every signed-in person fetches with
  it ([D224](../../decisions/working-notes.md) §1, §9). *(Amended 2026-09-27 by D224: this made
  the OpenSubtitles login each user's, per user where the provider's terms are per user.)*
  Because the backup carries the whole database ([D185](../../decisions/working-notes.md)), it
  carries these too.

## Four identity sources, in the order they ship

From [remote-access.md](../research/remote-access.md) §6, unchanged:

1. **Server-local accounts.** Always available, no cross-server identity. Ships first.
2. **Capability URLs.** Perfect for "here is one film"; unattributable for ongoing access.
3. **Bring-your-own OIDC**, for the technical household and for anyone with a homelab SSO.
4. **An optional first-party directory** — the premium tier's identity half, and the open
   product question ([overview.md](../overview.md) §Open problems).

## Signing in on a television

*Not in Cut A ([D191](../../decisions/working-notes.md)); designed now because its endpoints
are part of the frozen API core below.*

**The standard is RFC 8628**, the OAuth 2.0 Device Authorization Grant: the TV shows a short
code and a verification URL, the user opens it on a phone, the TV polls until it is approved or
the code expires. It is what the large streaming services use, and it is the shape Jellyfin
arrived at independently as Quick Connect —
`QuickConnectResult { Secret, Code, DateAdded, DeviceId, DeviceName, AppName, AppVersion }`.

Use the standard rather than a bespoke twin. The reasons are not purity:

- the flow is the same whether the identity source is a local account or an external OIDC
  provider, so (1) and (3) above do not need two implementations;
- it is the only sign-in that works for a **grantee**, whose television must be pairable
  without the owner typing a password into somebody else's living room;
- the approving screen is a natural place to name the device being approved, which is the only
  defence against a code phished out of a household member.

**It changes what a session is**, which is why it must be designed in rather than added: the
authorising act happens on a different device from the one being authorised, so a session is
created by one principal on behalf of another.

## Constraints from elsewhere

- **The pairing and handshake endpoints are the frozen core of the API**
  ([D85](../../decisions/working-notes.md)). They are the first thing a years-old television
  touches; if they break, the client cannot even report that it is too old.
- **Sign the session, not the segment** ([D70](../../decisions/working-notes.md),
  [threat-model.md](threat-model.md)). Per-segment per-user signatures give every viewer a
  unique URL for identical bytes and silently foreclose caching and fan-out.
- **A grantee is a different trust domain** ([D76](../../decisions/working-notes.md)), so
  authorisation composes with the share predicate at every access path, not only at listing.
- **Omission, not rejection** ([principles §10](../../principles/10-only-what-a-client-may-use.md)) for our own
  cooperating clients; routine validation for everything untrusted.

## The principals: two roles, and one non-principal

[D125](../../decisions/working-notes.md). **There is no `Profile` separate from a `User`.**
Netflix separates them because one paying account fronts several viewers under one licence; we
license nothing ([D76](../../decisions/working-notes.md)), so the thing that forces the split
does not exist here.

**A role is a named policy object and a user is assigned to one**
([D130](../../decisions/working-notes.md), correcting D125's two-closed-roles). A predicate
attached to a *user* means three children are three copies of one policy, and copies drift —
the household raises the ceiling in two places and forgets the third, which fails **open**.

| Part of a role | What | Vocabulary |
| --- | --- | --- |
| **Capabilities** | a closed set of verbs — `administer`, `deleteMedia`, `download`, `hostWatchTogether`, `editMetadata`, `remoteAccess` | the enum, and it stays short |
| **Visibility** | **one** predicate | [D75](../../decisions/working-notes.md)'s language over its field registry |
| **Defaults** | preference seeds for a new member | [D24](../../decisions/working-notes.md)'s axes |

- **A user has exactly one role.** Two would mean combining two predicates, and neither `AND`
  nor `OR` is a defensible default — one silently narrows, the other silently widens and breaks
  [D76](../../decisions/working-notes.md)'s fail-closed rule.
- **Two built-ins, undeletable**: `Administrator` and `Household`. A server whose last admin
  role was deleted is unrecoverable.
- **Everything else the household makes**: `Kids`, `Teens`, `Guests`. Define once, assign three
  children, done — which is the whole requirement, and it is one join column.
- **An admin is also a user.** Same row, same play state, same preferences. Administration is a
  capability the role carries, not a second account to switch into.
- **Every user gets their own** play state, history, favourites, ratings, preferences,
  downloads and rules. Independent, always, including the admin's.

**A share is a role with one member**, which unifies [D76](../../decisions/working-notes.md)
and [D84](../../decisions/working-notes.md): both were *"a grantee plus a predicate"* and both
become *"a role's predicate"*, evaluated once for every member. The UI need not say "role" for
a one-off share.

**`hostWatchTogether` is the capability worth noticing.** A guest is authenticated by
possession of a URL and nothing else; that is acceptable when an adult chose to share it, and
is a child handing strangers a route into the library otherwise. The hole
[D125](../../decisions/working-notes.md) accepts deliberately is only safe if **who may open
it** is itself a permission.

**Per-user exceptions narrow and never widen.** If a child needs different rules that is a
different role — which is why roles must be cheap to make. The one exception is a per-user
**deny** list, because it cannot grant anything.

**A Watch Together guest is not a principal at all.** No user row, no password, no recovery,
and **nothing persists past the session** ([D125](../../decisions/working-notes.md),
[D117](../../decisions/working-notes.md)). They are authenticated by possession of the URL and
by nothing else — deliberately, because the motivating case is someone with no account — and
bounded by a cap, ejection and rotation rather than by a predicate
([D119](../../decisions/working-notes.md)). See [watch-together.md](watch-together.md).
**A participant who is signed in is still a user**, and their viewing in the session is recorded
against their own account; the session itself grants them nothing beyond the stream
([D191](../../decisions/working-notes.md)).

## Two attributes the role needs that nothing else names

From the 2026-09-21 source sweep ([D149](../../decisions/working-notes.md),
[D150](../../decisions/working-notes.md)).

- **A bandwidth and concurrent-stream ceiling is a role attribute.** Jellyfin has
  `RemoteClientBitrateLimit`; Plex has per-user limits; nothing in this tree had either. It
  belongs on [D130](../../decisions/working-notes.md)'s role object rather than in the stream
  service, because *"how much of the shared pipe may this person take"* is a policy attribute
  exactly like the ones already there — and because
  [remote-access.md](remote-access.md)'s **paid relay** makes it a cost control rather than a
  courtesy. Two numbers: a maximum bitrate and a maximum simultaneous stream count, both
  nullable meaning unlimited, both enforced at session admission so a refusal is a stated
  reason ([D92](../../decisions/working-notes.md)) and not a stall.
- **A PIN may switch profiles; it may never authenticate.** *PIN protection for users* is the
  **highest-scoring single issue in the whole 932-issue sweep corpus (+56)**, and Jellyfin
  **removed** its PIN in 10.9 as *"a big security risk especially for administrator
  accounts"*. Both are right, because they are different things. What people want is a
  **profile selector on a television that is already authenticated** — a switch between
  identities already admitted on an already-trusted device, which is
  [D120](../../decisions/working-notes.md)'s person-plus-device. What Jellyfin removed was a
  four-digit *password* that could obtain an administrator session. So: **a PIN may gate a
  switch to a more-privileged profile on an authenticated device, and may never be a
  credential that authenticates a session, issues a token, or admits an administrator.**
  [threat-model.md](threat-model.md) carries the prohibition and
  [testing-strategy.md](testing-strategy.md) asserts it.

## The credential: one grant row, opaque and revocable

[D154](../../decisions/working-notes.md), 2026-09-21, on the measurements in
[token-model.md](../research/token-model.md). This document settles the **principals**; that
decision settles their **credentials**, and the two together close
[threat-model.md](threat-model.md)'s oldest question.

```
grant(secret_sha256, kind, user_id, scope, issued_at, expires_at, last_used_at, revoked_at, device_name, device_id)
```

**Opaque 256-bit random, stored as a SHA-256 hash, never as the secret.** Not a JWT — measured,
an opaque token verifies in **4.23 µs** at 100,000 rows against an authenticated request rate
of about **5/second**, while a *revocable* signed token costs **4.61 µs** and Ed25519 costs
**32.6 µs**. Revocation is already the control ([D117](../../decisions/working-notes.md),
[D119](../../decisions/working-notes.md)), so the signed token's only advantage was never
available. It also keeps the format ours: a JWT's claims become a public contract the moment a
client parses them ([D108](../../decisions/working-notes.md)).

Eight kinds — `session`, `link`, `cast`, `join`, `api_key`, `adapter`, `node`, `relay` — each
with its scope and lifetime in D154's table. Two rules govern all of them: **the broader the
scope, the more deliberate the act that mints it**, and **no credential ever appears in a
URL.** Bytes carry a path-scoped `Secure; HttpOnly; SameSite` cookie; the API carries an
`Authorization` header and refuses anything else. A URL a user deliberately exports to VLC gets
a `link` grant — one item, one rendition, minutes — which turns Jellyfin's *Copy Stream URL*
vulnerability into this project's *"play via external player"* feature (**+63**).

**Sessions expire on idle, not absolutely.** A television that signs itself out every thirty
days is the behaviour users call broken; `last_seen` plus a long idle window retires abandoned
devices without that cost, and revocation remains the real control. **There are no refresh
tokens** — they exist to renew *unrevocable* tokens, and ours are revocable.

**The last administrator cannot be removed or demoted**, and `administer` is otherwise
revocable by another admin. Recovery is the filesystem
([D35](../../decisions/working-notes.md)'s rule applied to people): a single-use,
minutes-long admin grant minted by a command on the server host into a file only the server
user can read. Anyone able to run it already owns the machine.

## How entitlement ends, and why nothing is told

[D168](../../decisions/working-notes.md), 2026-09-23. Three things get called entitlement and
they end differently:

| Question | Governed by | Ends when |
| --- | --- | --- |
| am I still a principal? | the grant row | `revoked_at` is set, or idle expiry passes |
| may I still see *this*? | the role or share predicate ([D164](../../decisions/working-notes.md)) | the predicate changes, or the item does |
| may I keep bytes I already hold? | the lease ([D167](../../decisions/working-notes.md)) | the lease is not renewed |

**The mechanism is that nothing is ever pre-authorised.** Every request resolves entitlement
at the point of use against current state, so a client never holds a decision — only content
it was already given. **Revocation therefore takes effect on the very next request, with no
message sent to anybody.** Nothing is in flight; nothing can be missed.

Measured, because the objection is always cost (3,500 items, 200 live grants):

| | |
| --- | --- |
| grant is live — hash lookup, `revoked_at` and expiry checked | **3.26 µs** |
| may this principal see this one item — the role predicate | **3.78 µs** |
| **both, as one byte request does them** | **6.64 µs** |
| revoke every grant one user holds | **145 µs**, one `UPDATE` |
| **100 concurrent streams, one check per 6 s segment** | **0.011% of one core** |

There is no performance argument for caching an authorisation decision, and
[D164](../../decisions/working-notes.md) already declined to cache the role's visible set —
which read as a performance non-event and is in fact the security property: **no materialised
entitlement means nothing stale to invalidate.**

**The notification is a latency optimisation and may always be lost.** A client sitting on a
grid does not re-request for minutes, so *"your access changed, re-sync"* closes that window —
as an invalidation hint in [principles §14](../../principles/14-a-cache-may-always-be-wrong.md)'s sense, with **no
delivery guarantee, ordering, acknowledgement or retry**, because the next request catches it
anyway. It carries **no detail**: naming what was withdrawn tells a child the item exists,
which is the disclosure [D134](../../decisions/working-notes.md) closed everywhere else. It is
the same event that invalidates the sync cursor ([D166](../../decisions/working-notes.md)).

**Offline, time is the only mechanism.** You cannot inform a client that is not listening, so
for a device on a plane revocation latency is bounded by the **session idle window** and the
**download lease**. And since a lease is indefinite by default
([D169](../../decisions/working-notes.md)), the guarantee has to be stated precisely rather
than reassuringly: *for your own devices there is no offline revocation guarantee, and there
should not be one; for a grantee there is, and it is exactly the lease the owner set on the
share.* An **idled-out session does not orphan a download** — a phone in a drawer for three
months signs in again with its films untouched. Anything stronger would mean enforcing against bytes on hardware we do not control,
which is [D3](../../decisions/working-notes.md)'s declined DRM in a different hat.

**And one misreading to head off:** [stream-service.md](stream-service.md)'s *"authorise the
session, not the segment"* is about **cache keys**, not about skipping checks — it keeps
identical bytes on one URL. The grant still rides every segment request and is still checked,
so a revoked viewer stops **within one segment**, mid-film.

## Still open

The **idle-expiry window** is a number wanting a real household rather than a guess, and
whether a `node` grant is **mTLS or a shared enrolment secret** remains
[D107](../../decisions/working-notes.md)'s choice — constrained only by D154's requirement
that it be a row in the same table.

~~**How provider credentials in the `setting` table are protected at rest** is not decided.~~
**Settled by [D194](../../decisions/working-notes.md):**
- **Our users' passwords** are argon2id hashes with RFC 9106's parameters, re-hashed on login
  when the parameters change.
- **Third-party secrets** (the TMDB key, OpenSubtitles logins, the DNS token, the TURN secret)
  are sealed in their own `secret` table with a key kept in `secret.key`, outside the database.
  A database backup therefore carries only ciphertext.
- **Secrets are write-only over the API**, and a Go type refuses to print them.
- **Restoring without the key** asks for the secrets again and loses nothing else.

## Done when

**Cut A:** the owner invites the remote person, who signs in from outside the house over HTTPS
with a local account; a session can be listed and revoked by device; she fetches subtitles with
the server's OpenSubtitles login, a sealed secret she never sees
([D224](../../decisions/working-notes.md) §1); and no credential appears in a URL anywhere.

**Later:** a television is signed in without anyone typing a password with a remote; a grantee pairs their
own device without the owner present; a session can be listed and revoked by device; an old
client that cannot speak the current API still reaches the handshake and is told why; and no
credential appears in a URL anywhere.
