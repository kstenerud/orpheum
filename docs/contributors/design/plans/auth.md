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

| | |
| --- | --- |
| **Roles** | `admin` and `user`. Two, closed, not a permission lattice. |
| **An admin is also a user** | Same row, same play state, same preferences — administration is a capability the row carries, not a second account to switch into. |
| **`admin` grants** | Library configuration, node enrolment, user creation, deletion policy, the metrics endpoint. The things that change the *server*. |
| **Every user gets** | Their own play state, history, favourites, ratings, preferences, downloads and rules. Independent, always, including the admin's. |
| **A user sees** | Whatever their predicate allows ([D76](../../decisions/working-notes.md), [D84](../../decisions/working-notes.md)). |

**A grantee is a user with a predicate, and so is a child.** Three things the tree modelled
separately — household member, shared-with relative, kids profile — are one row and one rule
each. That makes the rules engine's fifth binding the *only* access mechanism there is.

**A Watch Together guest is not a principal at all.** No user row, no password, no recovery,
and **nothing persists past the session** ([D125](../../decisions/working-notes.md),
[D117](../../decisions/working-notes.md)). They are authenticated by possession of the URL and
by nothing else — deliberately, because the motivating case is someone with no account — and
bounded by a cap, ejection and rotation rather than by a predicate
([D119](../../decisions/working-notes.md)). See [watch-together.md](watch-together.md).

## Still open

Everything [threat-model.md](threat-model.md) lists: session lifetime, API key scoping and
expiry, revocation, and what a shared link actually grants. This document now settles the
**principals**; their credentials are still that document's question. Two smaller ones from
[D125](../../decisions/working-notes.md): whether a second admin must exist before the first
can be removed (it should — a household with no admin is unrecoverable), and whether `admin` is
revocable by another admin.

## Done when

A television is signed in without anyone typing a password with a remote; a grantee pairs their
own device without the owner present; a session can be listed and revoked by device; an old
client that cannot speak the current API still reaches the handshake and is told why; and no
credential appears in a URL anywhere.
