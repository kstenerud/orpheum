> **ABOUTME:** Identity, sessions and signing in — including on a television, which is the
> first thing every user does on the device with the worst text entry ever shipped.

# Authentication and identity

- **Status:** PLANNED — pairing shape decided 2026-09-19 ([D93](../../decisions/working-notes.md), [audit-2026-09-19.md](../research/audit-2026-09-19.md) §15); the token model remains [threat-model.md](threat-model.md)'s open question.
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
so is retrofitting anything else on this page.

## Four identity sources, in the order they ship

From [remote-access.md](../research/remote-access.md) §6, unchanged:

1. **Server-local accounts.** Always available, no cross-server identity. Ships first.
2. **Capability URLs.** Perfect for "here is one film"; unattributable for ongoing access.
3. **Bring-your-own OIDC**, for the technical household and for anyone with a homelab SSO.
4. **An optional first-party directory** — the premium tier's identity half, and the open
   product question ([overview.md](../overview.md) §Open problems).

## Signing in on a television

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
- **Omission, not rejection** ([principles §10](../../principles/README.md)) for our own
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
| **Capabilities** | a closed set of verbs — `administer`, `delete_media`, `download`, `host_watch_together`, `edit_metadata`, `remote_access` | the enum, and it stays short |
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

**`host_watch_together` is the capability worth noticing.** A guest is authenticated by
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
grant(hash, kind, subject, scope, issued_at, expires_at, last_seen, revoked_at, label, device_id)
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

## Still open

The **idle-expiry window** is a number wanting a real household rather than a guess, and
whether a `node` grant is **mTLS or a shared enrolment secret** remains
[D107](../../decisions/working-notes.md)'s choice — constrained only by D154's requirement
that it be a row in the same table.

## Done when

A television is signed in without anyone typing a password with a remote; a grantee pairs their
own device without the owner present; a session can be listed and revoked by device; an old
client that cannot speak the current API still reaches the handshake and is told why; and no
credential appears in a URL anywhere.
