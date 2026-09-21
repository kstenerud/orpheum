> **ABOUTME:** What the field's credentials actually are, read from source — and the four
> measurements that decide opaque-versus-signed. The short version: Jellyfin's token is a
> plaintext GUID with no scope and no expiry that it accepts in a query string, and a revocable
> opaque token costs 4.2 µs to check.

# The token model, measured

Researched **2026-09-21**. Backs [D154](../../decisions/working-notes.md). Answers the question
[threat-model.md](../plans/threat-model.md) has carried since 2026-09-18 and that five
decisions and two plans say they are waiting on.

---

## 1. What Jellyfin actually does

Read from `src/Jellyfin.Database/.../Entities/Security/` and `Jellyfin.Api/.../AuthorizationContext.cs`.

**A device's access token:**

```csharp
AccessToken = Guid.NewGuid().ToString("N", CultureInfo.InvariantCulture);
```

A random UUIDv4 — **122 bits of entropy**, which is fine — and then four things that are not:

| Property | Jellyfin | Consequence |
| --- | --- | --- |
| **Stored** | **plaintext**, compared with `apiKey.AccessToken == token` | a database read is a full compromise of every session |
| **Expiry** | **no column exists.** `Device` has `DateCreated`, `DateModified`, `DateLastActivity` and nothing else | a token issued once is valid forever |
| **Scope** | none. `ApiKey` is `(Id, DateCreated, DateLastActivity, Name, AccessToken)` | **an API key is admin-equivalent by construction** |
| **Where accepted** | `queryString["ApiKey"]`, `queryString["api_key"]` — `AuthorizationContext.cs:105,110` | **this is the "Copy Stream URL leaks the session key" bug**, and it is not a slip; it is a documented input |

Those four compose into the worst case: a URL a user copies out of the player to paste into VLC
carries a **never-expiring, unscoped, admin-equivalent, plaintext-stored** credential.

## 2. What Plex does, which is better in the one place that matters

`python-plexapi`, `server.py:229`:

```python
def createToken(self, type='delegation', scope='all'):
    """ Create a temp access token for the server. """
    q = self.query(f'/security/token?type={type}&scope={scope}')
```

Called from `client.py` when **casting** — Plex mints a *separate temporary delegation token*
to hand to a player rather than handing over `X-Plex-Token`. The parameters are named `type`
and `scope`, so the model has both concepts even though the default is `scope=all`.

**That is the shape worth copying: the credential you hand to a device is not the credential
you hold.** Plex still puts `X-Plex-Token` in query strings everywhere (`includeToken=True`
appears throughout the library), so it solves the delegation case and not the leak case.

## 3. The measurement that decides opaque versus signed

The usual argument for a signed token (JWT-shaped) is that it verifies without touching the
database. The usual counter is that it cannot be revoked. Both are true, so the question is
what each actually costs.

Rig: Intel i3-1315U, Go 1.26.2, `modernc.org/sqlite` (the pure-Go driver
[D31](../../decisions/working-notes.md) picked, which is the slower one), a
`WITHOUT ROWID` grant table of **100,000 rows**, warm.

| | µs/op | ops/s |
| --- | ---: | ---: |
| **Opaque: SHA-256 of the presented secret + primary-key lookup** | **4.23** | **236,000** |
| …the lookup alone, without the hash | 4.17 | 240,000 |
| Signed: HMAC-SHA256 verify + base64 + parse, **no revocation check** | **0.57** | 1,763,000 |
| Signed: **Ed25519** verify | **32.6** | 31,000 |
| **Signed + a revocation lookup** — what a revocable signed token really costs | **4.61** | 217,000 |

Three conclusions, and the third is the decisive one:

- **An opaque token costs 4.2 µs to verify.** For scale, the authenticated request rate here is
  about **5 per second** — twenty concurrent streams pulling one HLS segment every four
  seconds — plus [D81](../../decisions/working-notes.md)'s ~2 progress writes/second. We have
  roughly **four orders of magnitude of headroom**. Verification cost is not a design input.
- **Asymmetric signing is the slowest option**, by 7.7×. The usual reason to reach for it —
  so a relay or a transcoder can verify without asking the core — is the most expensive thing
  on the list, and [D72](../../decisions/working-notes.md) does not want the relay verifying
  anything anyway.
- **A signed token that can be revoked is slower than an opaque one** (4.61 vs 4.23 µs),
  because it pays for the lookup *and* the signature. Revocation is not a nice-to-have here:
  [D117](../../decisions/working-notes.md) and [D119](../../decisions/working-notes.md) already
  make **revocation the control** for Watch Together. **The moment revocation is required, the
  signed token's only advantage disappears and it becomes strictly worse.**

## 4. Which clients genuinely cannot send a header

This decides whether a URL-borne credential is unavoidable, and mostly it is not:

| Surface | Can set a request header? |
| --- | --- |
| `fetch`/XHR, hls.js (`xhrSetup`) | yes |
| ExoPlayer, AVPlayer, libmpv | yes |
| **Our Cast receiver** — it is our own web app ([D124](../../decisions/working-notes.md)), so `LoadRequestData.credentials` plus a request filter | **yes** |
| `<video src>` with native HLS (Safari/iOS), `<img src>` | **no** — but a **cookie** is attached automatically |
| A URL deliberately copied out, to VLC or mpv | **no, and nothing can help** |

So the only case with no mechanism is **the one where the user is deliberately exporting a
URL** — which is a feature request in its own right (*"play via external player"*, **+63**,
[tv-client.md](../plans/tv-client.md)) and therefore wants a credential minted on purpose
rather than a session token leaking by accident.

## 5. Why refresh tokens buy nothing here

Refresh tokens exist because access tokens are *unrevocable*, so they must be short-lived,
so something must silently renew them. Ours are revocable in one row update and cost 4.2 µs to
check. **Removing the reason removes the mechanism**, and with it a second credential class,
a rotation protocol and a race condition.

## Sources

- `jellyfin/jellyfin` — `Entities/Security/{Device,ApiKey}.cs`, `AuthorizationContext.cs`
- `pkkid/python-plexapi` — `server.py` (`createToken`), `client.py`
- Benchmark harness: `/yoloai/cache/bench/tokens/main.go`
