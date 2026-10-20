> **ABOUTME:** An independent security audit of device codes through the paid service (D293 §2), as
> built on 2026-10-20: where the scheme is weak, what fixes each weakness, and how it compares with
> the device grant, a QR code, a code shown on the phone and a PAKE.

# Device codes through the paid service: a security audit

Done on 2026-10-20, at the owner's request, by an agent that had not built the scheme. It read
Orpheum at an earlier commit and orpheum-connect at an earlier commit and changed nothing. The building agent then
checked H1's and H2's evidence against the code; both hold. Line numbers are as of those commits.
This is research: what to build from it is the owner's to decide.

## 1. The scheme as built

1. **The TV asks for a code.** It makes a P-256 key whose private half cannot be exported
   (`web/packages/sdk/src/device-code.ts`). It posts the public half and its platform to the
   service's `POST /client/v1/device-codes`, and is given:
   - a code, `xxxx-xxxx`, 40 bits;
   - a 24-byte secret to poll with;
   - a life of ten minutes.

   Codes are held in memory: at most 1000 live, 20 not yet entered per /48 or IPv4 address, and 5
   per /64 (`orpheum-connect/internal/devicecodes/devicecodes.go`).
2. **The person enters the code on their core**, under Activate a device. This needs a session
   that manages the account, and five failures in fifteen minutes stop the person
   (`internal/core/auth/pairing.go`). A code the core did not issue is claimed from the service,
   which binds it to that core and returns the TV's key and platform.
3. **The check code** is 40 bits of SHA-256 over the TV's key (`discovery/proof/devicecode.go`).
   The core makes it from the key the service returned; the TV makes it from its own key once its
   code is entered. The key fixes it, so it is known from the moment the code is issued.
4. **A match.** The core issues a device-scoped session, keeps it as a hand-off for 60 s under
   `DeviceCodePerson(code)`, and seals `{coreId, fingerprint, realm, handOffCode}` with HPKE base
   mode (DHKEM(P-256), HKDF-SHA256, AES-128-GCM). The info is a fixed label; there is no AAD and no
   sender authentication. The core posts the result to the service's `/outcome`.
5. **The TV takes the session.** Its next poll gets the sealed outcome and a bootstrap token
   (five minutes, reusable). It opens the outcome, remembers the household, goes to the client
   origin with the hand-off fragment, and redeems the code over the path it reaches. The signed
   manifest is checked only after that.

**Where the code differs from the docs:**
- The page entering the code has no warning; D293 §2 puts one there. The warning comes only at the
  check-code step.
- Nothing logs an admin entering a code for another admin, which A11/K10 asks for.
- S13's "fairly across sources" is only a cap on codes live at once. Nothing rate-limits over
  time, or claims, or polling.
- A device a service code activates never resumes the device it was before (D221).
- The platform may be 64 runes through the service, against 40 through the core's own API.

## 2. Findings

### H1 (high): the hand-off code goes to any direct path the service names, before the core's identity is checked

**Evidence:**
- `web/packages/sdk/src/discovery.ts` `directUrlsOf` takes any `https://` URL as a direct path,
  without checking that its host is under `<coreId>.<zone>`.
- An answer on `/health` is proof enough (`direct-path.ts`).
- `loader-at-client-origin.tsx` redeems the hand-off before it checks the signed client manifest.
- The outcome's fingerprint is checked only on signed WebRTC answers and on the manifest.
- The page's CSP admits every core's names.

**Attack.** It needs control of the service's API only, which is the case the seal and the check
code are meant to stop.
1. The attacker runs a claimed core of their own, with a valid certificate.
2. The service's `/paths` names that core's host for the victim's `coreId`.
3. The attacker's core receives `POST /session/redeem` with the code, redeems it at the real core
   and relays everything.
4. The manifest the real core signed passes, so the TV works while the attacker holds its session.

The same path serves remembered sessions and the realm and link hand-offs, so this predates device
codes. A service that controls the zone's DNS and CAA can also get a certificate for the victim's
own label; watching Certificate Transparency is deferred.

**Fix:**
- Accept a direct URL only when its host is `<label>.<coreId>.<zone>`.
- Release no bearer secret over a path the identity key has not vouched for. For example, the core
  could answer the redemption with the session sealed to the TV's HPKE key, so a relay holding the
  code gets ciphertext alone.
- Bring CT watching forward.
- Say in D293 §2 that the scheme holds against an honest-but-curious service only.

### H2 (high once public, medium in the closed beta): claims are unlimited, and claimed codes stop counting against their source

**Evidence:**
- `orpheum-connect/internal/cores/devicecodes.go` counts no failure and limits no rate.
- `hasRoomLocked` skips claimed codes in the per-site and per-network counts, but counts them in
  the 1000 cap.
- A claimed code lives out its ten minutes.
- The core's per-person limit is skipped by calling the service directly with a core's session.

**Attacks:**
- **Exhaustion.** One address with one claimed core asks for 20 codes, claims them, and repeats;
  about 50 rounds fill the cap, and no TV anywhere gets a code. Without a core, about fifty /48s
  or IPv4 addresses do the same.
- **Guessing.** With the pool full, about 1.1×10⁹ guesses hit a live code: days, unthrottled.
  Each hit is M1's hijack.
- Each call scans every code under one global mutex.

**Fix:**
- Limit failed claims per core (about 5 in 15 minutes), and claimed-but-unfinished codes per core.
- Keep counting a claimed code against its source, or expire an unfinished claim within about two
  minutes.
- Limit asking and polling per source over time.

### M1 (medium): a code seen by someone else can be taken, and the TV joins whoever activates it, unasked

The TV shows the check code, then "Activated. Opening your server…": nothing names the household
or the person, and nothing asks the TV to confirm. The household is then remembered, and a
browser with one household goes straight to it on every visit.

**Attack.** Someone sees the code: a TV in a shared space, a screenshot, a stream, or H2's
guessing. They enter it on their own core first; their core makes the same check code. The real
person's entry then fails. The TV opens the attacker's household on an official-looking origin,
whose client can show a fake "sign in again" form for the realm password.

**Fix.** The TV shows "Join ‹household› as ‹person›?" and needs an OK on the remote, defaulting to
No. The core signs the outcome with its identity key over (the TV's key, the code, `coreId`).

### M2 (medium): the check code does not stop a malicious service, and only partly stops one that holds the API alone

- **Malicious JavaScript.** The service serves the TV's script (protocol §3.4 admits this), so it
  can show any check code, read the key, or send the opened outcome anywhere.
- **Forged outcomes.** HPKE's base mode authenticates no sender, so anyone who knows the TV's
  public key can seal an outcome. That includes the service and any core that claimed the code. A
  service holding the API alone can send any TV to any household, with no core or person in it.
- **Grinding the check code.** Swapping in a key of its own needs a key whose 40-bit check code
  matches the TV's: about 2⁴⁰ point additions. The audit's estimate, not measured: tens of minutes
  on one GPU, minutes on a rented cluster. That fits the window the service controls by delaying
  its answer to the claim. People also often compare part of a code.
- **What the service learns without attacking.** The device's key, platform, address and timing,
  and which core claimed which code; the address stays linked to a household through the
  bootstrap's five minutes and every later `/paths`.

**Fix:**
- Make the check code a committed short authentication string, as Bluetooth's numeric comparison
  and ZRTP do:
  1. The TV commits to `H(K_t ‖ n_t)` when it asks.
  2. The core sends a random `n_c` once it has claimed the code.
  3. The TV reveals `n_t`.
  4. The check code is a truncation of `H(K_t ‖ n_t ‖ n_c)`.

  Or use a PAKE (§3d).
- Sign the outcome (M1).
- Say in the docs that the service's JavaScript is trusted.

### M3 (medium): a check code known in advance lets a fixed message phish

The core's own codes pick the check code at random when the code is entered. A service code's is
fixed by the key, so the attacker knows it the moment the code is issued. A mass message ("enter
ABCD-EFGH under Activate a device, you'll see WXYZ-1234, press Yes") then works with no live call,
for ten minutes. This is the phishing pattern used against RFC 8628's device grant. The victim
gets a device-scoped session (M4). Nothing on the phone describes the device or says how old its
code is.

**Fix:**
- Use the core's committed randomness in the check code (M2).
- Put the warning where the code is entered.
- Show "a TV asking from ‹network or country›, N seconds ago". The network alone needs no location
  database.
- Show each code-activated device prominently, with a one-tap remove.

### M4 (medium): a phished device lasts, and nobody is told

**Lifetime.** A session ends after about 90 days idle, renewed by each use; there is no absolute
lifetime.

**No notice.** Nothing logs or tells anyone that a device was activated.

**What the device scope still allows.** It removes only `administer`, so the device keeps:
- hosting Watch Together and making its links, which hands the library to guests;
- `edit_metadata` where the role has it;
- reading the person's own link, which outlives removing the device.

It cannot change the password, enter codes, remove devices or end other sessions, and no route
was found by which it gains more.

**Fix:**
- Log each activation, and tell the person's other sessions and the admin.
- Give code-activated devices an absolute life, or confirm them again.
- Take `edit_metadata` and reading the own link out of the device scope.
- Build the admin-for-admin confirmation and log.

### L1 (low): the bootstrap token is reusable for five minutes and bound to nothing

It reveals the core's paths, its addresses among them. **Fix:** bind it to the TV's key, or make
it single-use.

### L2 (low): the outcome is delivered once, and a lost answer loses it

The service forgets the code before it makes the bootstrap token and before the answer reaches the
TV. If that answer is lost, the TV sees "ended" while the phone says activated. The 60 s hand-off
then expires, and a device row is left dangling. The minute is also tight for an old TV engine
over WebRTC.

**Fix:** keep "activated" until the hand-off is redeemed, or for about a minute, and give a
device code's hand-off about three minutes.

### L3 (low): a code confirmed near its ten minutes leaves debris

The core issues the session and device before `/outcome` fails. **Fix:** extend the code's life
on its claim, or check that it is live before issuing.

### Sound as built

- CORS is an exact allowlist without credentials, and the endpoints are bearer-authenticated, so
  CSRF does not apply.
- The platform string is cleaned where it enters and escaped by React.
- The HPKE suite is standard, the info separates the use, and the recipient key is bound through
  the KEM context.
- The polling secret has 24 random bytes.
- An outcome blob may be 64 KiB, unchecked: 64 MB at the cap.

## 3. Other schemes

| Scheme | Security | UX |
| --- | --- | --- |
| **(a) RFC 8628 device grant** | The core's own flow without the check code. Remote phishing is its known weakness, met only by entropy, limits and warnings; it trusts the authorization server, here the service. | Familiar: go to a URL and type a code. |
| **(b) A QR on the TV carrying its key's hash, with a typed code kept as the fallback** | The key travels by camera, so the service is cut out of the exchange: it can drop or delay, not substitute. Phishing by forwarding the image remains, but a fresh QR per attempt and the device's details on confirming help. The service still serves the TV's script. | Best when it works: one scan and a confirm. On iOS the camera opens Safari, which shares no storage with a home-screen app (U2-2), so the person may not be signed in there; the typed code must stay. |
| **(c) The phone shows a code the TV types** | Phishing is harder: the victim must read their own code out. But the code is a bearer credential, open to distributed guessing (D288's objection), and key substitution still needs a check code. | Typing on a remote is poor; D288 dropped it for this. |
| **(d) A PAKE (CPace, SPAKE2) on a split code**, as Magic Wormhole does: part of the code routes, part is a password never sent | A service in the middle gets one online guess per run, at 2⁻ᵏ. There is no check code and nothing to grind; key confirmation replaces the comparison. It does nothing for phishing or hijack. Web Crypto cannot do the curve arithmetic, so it needs a library (a dependency, rule 3), and old TV engines need BigInt. | One screen fewer; codes of 10 to 12 characters for k ≈ 30. |
| **(e) This scheme, fixed** | With H1, H2, M1, M3 and M4 fixed: sound against third parties and an honest-but-curious service. With M2's committed check code: sound against a service holding the API alone. Nothing fixes a service serving malicious script, short of a native app or a client the core serves. | Unchanged, but for one OK on the TV. |

**The audit's recommendation.**
- **For the closed beta:** (e). Two households and known testers make H2's guessing and M3's mass
  phishing unlikely, but H1 and M1 are cheap and matter now.
- **Later:** (b) and (d). A QR on the TV carrying its key's hash and a routing code, with a typed
  fallback that is a committed check code or a split-code PAKE. Add the TV's confirmation and the
  device's whereabouts. Treat the service's script as a trusted dependency, and say so, until a
  native app or a core-served client exists.

## 4. The fixes, in order

1. **H1:** take a direct URL only under `<coreId>.<zone>`; release no bearer secret over a path the
   identity key has not vouched for (seal the redeemed session to the TV's key); bring CT watching
   forward.
2. **H2:** limit claims per core; count claimed codes against their source, or cap and expire them;
   limit asking, claiming and polling over time.
3. **M1:** the TV names the household and the person and waits for OK; the core signs the outcome.
4. **M3, M4:**
   - Put the warning where the code is entered.
   - Show the device's details and how old its code is.
   - Log and notify each activation.
   - Build the admin-for-admin confirmation.
   - Take `edit_metadata` and reading the own link out of the device scope.
5. **M2:** a committed check code, or a split-code PAKE; correct D293 §2's claim about a service
   that only relays.
6. **M4's lifetime:** an absolute life for code-activated devices, or confirmation again.
7. **L2, L3:**
   - Keep "activated" until the hand-off is redeemed.
   - Give a device code's hand-off three minutes.
   - Extend a code's life on its claim.
8. **L1:** bind the bootstrap token to the TV's key, or make it single-use; bound the outcome's
   size.
