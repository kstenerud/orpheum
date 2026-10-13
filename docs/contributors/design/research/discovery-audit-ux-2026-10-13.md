> **ABOUTME:** A user-friendliness audit, before development, of the discovery protocol
> ([discovery-protocol.md](../discovery-protocol.md)) and the paid service's spec (draft 4, the
> shared files' `discovery-service-spec.md`). It walks each journey as a person would, compares it
> with Plex, Jellyfin, Emby, Tailscale, Synology QuickConnect and Home Assistant Cloud, and checks
> it against published UX guidance. Done 2026-10-13, on the owner's request for an audit (spec §11.3).

# Discovery audit: user-friendliness (2026-10-13)

**Markers.** **[V]** means checked at a source listed under *Sources*. **[I]** means my inference.
`D<n>` cites a decision. "Protocol" is `discovery-protocol.md`. "Spec" is the paid service's spec,
draft 4. Severity: **high** means a common journey fails or frightens people; **medium** means
extra steps, confusion or a costly recovery; **low** means polish.

## Summary

The design gets the hard parts right for a household: no third-party account for family, a link
that does everything on a phone, LAN playback that survives the service, and plain warnings. Its
weak points are at the edges a family meets most often.

1. **Chrome's Local Network Access prompt will appear on almost every first visit**, at home and
   away, because the client races the LAN name at once. A "Block" sticks, and Plex's forum shows
   what follows.
2. **A device that forgets its server has no way back on its own.** Safari deletes site storage
   after seven days without a visit, in-app browsers keep their own storage, and the design has no
   lookup. The only fix today is a new link from the admin, which also resets a password.
3. **When nothing direct works, the person sees a dead end** addressed to someone else: the spec's
   message tells a guest in a hotel to ask "your ISP" for an IPv4 address. Every competitor has a
   relay here. Orpheum decided not to (D273 §2), so the message and the frequency matter more.
4. **The owner's first setup is about twelve steps on three surfaces**, in network jargon
   ("discovery", "claim", "WebRTC", a raw API address). Plex claims a server with a sign-in.
5. **Several recoveries quietly cost every household device its link**: a restored backup without
   `identity.key` followed by *Connect*, and resubscribing after `released`. The design can avoid
   both.

The rest is medium or low: lapse messages that contradict the spec, no support for a person on two
servers, a TV flow with two codes and no QR, magic-link pitfalls, and no word on what the person
sees while paths race.

## The journeys, counted

Steps are actions a person takes; decisions are points where they must choose or understand
something. Counts are for the design as written, the beta's gaps aside (U4). **[I]** throughout.

| Journey | Steps | Decisions | Surfaces | Worst point |
| --- | --- | --- | --- | --- |
| Owner's first setup (install → family on) | ~12, plus 3 per person invited | 4 (discovery address, port mapping, whether the reachability is good enough, how to send links) | the core's UI, e-mail, the dashboard | the setup code is in the server's log; the claim code's 30 minutes run while the magic link travels |
| Family member, phone, by link | 4 (tap link, password twice, submit) | 0–1 (the LNA prompt) | the phone | the LNA prompt; an in-app browser keeps the result |
| Family member, TV (TV browser) | ~8 (type the address with a remote, *Get a code*, open the phone, account page, type 8 characters, name the TV, compare, confirm) | 2 | TV and phone | finding *Activate a device*; two codes |
| Watch Together guest | 3–4 (tap link, name, Join, tap for sound) | 0–1 (the LNA prompt) | one device | a frightening prompt or a dead end |
| Returning user, new device, no link | no self-serve path | — | — | must find someone signed in, or the admin |
| Owner: lost e-mail | contact support, prove by billing or a code | 1 | e-mail to support | a claimed core shows no code to prove with |
| Owner: restored backup without `identity.key` | re-key: dashboard, code | 1 | core UI, dashboard | *Connect* looks like the fix and makes a new core id |
| Owner: ISP or router changed | 0 if mapping or IPv6 still work | 0 | — | nobody is told when a path is lost |
| Owner: subscription lapsed | resubscribe | 1 | dashboard | after `released`, every device needs a new link |
| Discovery down | 0 for returning devices on a remembered HTTPS path | 0 | — | new devices, guests, TV codes and WebRTC-only households fail with a browser error |

---

## Findings

### U1 — The Local Network Access prompt appears on nearly every first visit, and a "Block" sticks (high)

**Journeys:** family member's first sign-in, Watch Together guest, returning user on a new browser.
**Sections:** protocol §3.2, §3.3, §3.4; spec §11.1 ("IPv6 and WebRTC usually avoid the Local Network
Access prompt").

**What the design says.** The client "tries every path at once" (protocol §3.3, step 1), and the
paths include a LAN name such as `192-168-1-20.<coreId>.<zone>` (§3.2). §3.4 says the prompt is
something "racing IPv6 and WebRTC first mostly avoids".

**What a user would experience.** Chrome counts as a local network request "any request from the
public network to a local network" destination, where a local network destination is "any
destination that resolves to addresses reserved for local use", such as 192.168.0.0/16 **[V]**
(Chrome blog). The LAN name resolves to a private address wherever the person is. So on the first
visit, whether at home, at a friend's or in a café, Chrome asks: *"app.orpheum-app.com wants to look
for and connect to any device on your local network"* **[V]** (wording quoted by Okta, F5 and The
Windows Club). Racing at once does not avoid it: the fetch that triggers it starts at once.
§3.3 and §3.4 contradict each other here **[I]**. A guest sees a privacy prompt from a product whose
pitch is privacy. If they choose *Block*, the decision is remembered for the site and must be undone
in site settings **[V]** (The Windows Club, Plex). Because `app.orpheum-app.com` is one origin for
every household, a block made as a guest at a friend's house also blocks the LAN path for the
person's own household later **[I]**. While the prompt waits unanswered, the LAN fetch waits too,
and the "few seconds" window of §3.3 closes without it **[I]**.

**What comparable products do.** Plex's web app has the same shape and the same problem. Plex
posted an announcement: if a user denies the prompt, the web app cannot make a local connection,
and "for folks that do not subscribe, this will cause the web app to ask you to purchase/subscribe
to a Remote Watch Pass or Plex Pass" **[V]**. The post links some fifty forum threads with titles
such as *Plex thinks I'm off-network*, *Server Unavailable While Owner Works Fine* and *Can't Access
Plex from browser locally; remote access works fine* **[V]**. Chrome itself notes that WebRTC is
"not yet" gated and that they plan to gate it "soon" **[V]**.

**Recommendation.**
- **Never race the LAN name cold.** Return the `lan` path only when the discovery sees evidence the
  client is at home: the request's source address equals the core's public IPv4, or shares the
  core's IPv6 /56 or /64. Otherwise leave it out of `POST /paths`.
- **Prefer WebRTC for the LAN.** ICE finds the LAN path by itself (research table, protocol §3.2) and
  is not gated today. Use the LAN name only as an upgrade once WebRTC shows a host candidate on the
  same subnet.
- **Explain before Chrome asks.** When the LAN name is about to be tried, show one line first:
  "Your browser will ask to connect to devices on your network. Allow it so films play at full
  speed at home." Keep a help page for undoing a block, as Plex had to write after the fact.
- Fix §3.4's sentence so the protocol does not promise what the race cannot keep.

### U2 — A device that forgets its server has no self-serve way back (high)

**Journeys:** returning user on a new device, household member who forgot which server, family
member on an iPhone. **Sections:** protocol §3.1 ("A new device with no link cannot search");
spec §2; D218 §5; D214 §3.

**What the design says.** A device learns its core once, from a link, a QR or a code, and keeps a
locator. There is no search by name or e-mail, which would be a list of cores. A forgotten password
is answered by a new registration link from the admin, which sets a new password (D218 §5).

**What a user would experience.**
- **Safari deletes the locator.** WebKit deletes "all of a website's script-writable storage after
  seven days of Safari use without user interaction on the site": LocalStorage, IndexedDB and
  service worker registrations alike **[V]**. A grandmother who watches once a fortnight on her
  iPhone loses her locator, her sign-in and the cached client. Home-screen web apps "have their own
  counter of days of use" **[V]**, so they are spared. **[I]:** this will be the most common way a
  family member loses the server.
- **In-app browsers keep their own storage.** A registration link tapped in Gmail or another app on
  iOS opens a Safari View Controller, which since iOS 11 does not share cookies or website data with
  Safari **[V]**. The person sets a password there; later, in Safari, nothing knows their server.
- **A new phone or a cleared browser** has the same result **[I]**.
- In each case the person has a name and a password and nowhere to type them. `app.orpheum-app.com`
  with no locator cannot use them, yet today's sign-in page shows that form first
  (`sign-in.tsx`). The only self-serve path is *Get a code*, which needs a second device already
  signed in. Otherwise they ask the admin, who can only send a registration link, whose page says
  "Choose your password", which reads as a reset to someone who knows theirs.

**What comparable products do.** Plex, Emby and Tailscale tie the server list to the person's
account: sign in anywhere and the servers appear. Emby's help says it plainly: "The user no longer
needs to remember the URL or IP/port of each Emby Server" **[V]**. Synology gives each NAS a
QuickConnect ID that the user types at `quickconnect.to`; one guide's whole section is *How to Find
Your QuickConnect ID* for people who forgot it **[V]**. Jellyfin has the user type the server's
address **[I]**. Orpheum rules out the account (D72) and the public ID (D214 §3), so it needs its
own answer.

**Recommendation.**
- **Give each person a "your server" link and QR**, on their account page and on the screen after
  registering: `https://app.orpheum-app.com/s#<coreId>.<secret>`. It **locates only**: it grants
  nothing, needs the password after, is reusable, and is revocable by the admin. It is not a list
  of cores (it names one, to someone who holds its secret), so D214 §3 holds **[I]**. A person can
  bookmark it, save it in their notes or e-mail it to themselves. Admins can resend it without
  touching the password.
- **After registering, say what keeps them signed in**: "Add Orpheum to your home screen"
  on iOS; "Open in Safari" when the page detects an in-app browser. Call
  `navigator.storage.persist()` where supported.
- **Design the no-locator page.** With no locator, `app.` should not show a name and password form.
  It should offer: "Open the link you were sent", "Sign in using a phone where you're already
  signed in" (the device code), and "Ask the person who runs your server for your server link".
- **A link for an existing person is a sign-in link, not a reset.** Let the admin's link for a
  known person say "Sign in on this device" and keep the password unless the person changes it.

### U3 — When nothing direct works, the person meets a dead end written for someone else (high)

**Journeys:** Watch Together guest, family member travelling, a household nothing reaches.
**Sections:** spec §1.3; protocol §5 (last paragraph), §3.2; D273 §2, D274 §2–§3.

**What the design says.** No relay in the paid tier. When symmetric NAT on both ends or an
HTTPS-only network blocks every path, the page says:

> "Your server could not be reached from this network. It can be reached from networks with IPv6,
> and from most others by a direct connection, but not from this one. To reach it from everywhere:
> ask your ISP for a public IPv4 address, or run a small VPS of your own (guide). Orpheum Discovery
> never carries your video."

The owner is warned ahead of time on the System page.

**What a user would experience.** The person who sees this is rarely the owner. A guest is told
about "your server" and "your ISP". A family member in a hotel cannot act on IPv6, an ISP or a VPS.
The last sentence is a product claim at a moment of failure. Nothing tells them what *they* can try
now. How often this happens is unknown. Hole punching succeeded in about 70% of attempts in the
measurement the research cites **[V, research]**; IPv6 and mapped ports raise the household's odds.
For a DS-Lite household (the owner's own kind, D274 §4), a client on an IPv4-only network has
only WebRTC **[I]**.

**What comparable products do.** Every comparable product keeps a last-resort relay. Plex relays
when direct fails, capped at 2 Mbps **[V]**. Synology QuickConnect relays when hole punching fails,
"the slow one" **[V]**. Tailscale falls back to its DERP relays **[V]**. A slow picture is a
different experience from no picture. GOV.UK says not to use error messages for "a problem the user
cannot fix" and that a message must "describe what has happened and tell them how to fix it" in
plain English with no jargon **[V]**. NN/g asks for "constructive advice" and language "familiar to
your users" **[V]**.

**Recommendation.**
- **Write the message per audience.** For anyone but the admin: "Can't reach *Stenerud home* from
  this network. Try your phone's mobile data, or another Wi-Fi. If it keeps happening, tell
  *Karl*." For the admin, the System page keeps the ISP/VPS advice, without "Orpheum Discovery never
  carries your video".
- **Suggest the hotspot.** Mobile networks mostly give phones IPv6 (D274 §4), so a phone's hotspot
  is the most likely fix in a hotel **[I]**.
- **Measure in the beta** (D274 §3 already asks this) how often a client ends here, by client
  network kind, and put the number in front of the owner before the paid launch. If it is more than
  a few percent, D274 §2's relay of failed connections is the user-friendly answer.
- **Make the owner's own TURN one switch** where they already run a VPS (spec §1.3), and say on the
  System page how many clients failed to connect last week, not only that the probe failed.

### U4 — The beta's scope leaves TVs and IPv4-only clients without a way in (medium)

**Journeys:** TV, family member on IPv4-only networks. **Sections:** spec §10 (closed beta).

**What the design says.** The beta has "no QR pairing, no TV codes, no WebRTC through the service
yet".

**What a user would experience.** A TV has no way to receive a registration link, and without TV
codes through the service it cannot learn its locator **[I]**. So no TV can join through `app.` in
the beta. Without WebRTC through the service, a DS-Lite or carrier-NAT core is reachable only by
IPv6, so a client on IPv4-only Wi-Fi hits U3's dead end every time **[I]**. Beta testers will
likely judge the product on exactly these two cases: the living-room TV and the phone on someone
else's Wi-Fi.

**Recommendation.** Either bring device codes through the service into the beta (the core's side is
built, D288), or state on the beta invite which households and devices it supports. Recruit the
first households with IPv6 or a mapped IPv4, and tell them a TV must wait.

### U5 — The owner's first setup is long, jargon-heavy and spread over three places (medium)

**Journeys:** owner's first setup. **Sections:** protocol §2.2; spec §2 (sign-in), §8 (pages),
§11.3 Q19; `system-discovery.tsx` and its strings; `sign-in.tsx` ("setup code from the server's
log").

**What the design says.** The admin opens System → Discovery and chooses *Connect*; the core shows
an 8-character claim code for 30 minutes; the customer signs in to `account.orpheum-app.com` by
magic link and types it. A QR of the claim link is planned, not built. A check code at claiming is
proposed (Q19).

**What a user would experience.** Roughly: find the setup code in the server's log; create the
admin; open System; read that "A discovery tells your clients where to find this server"; accept
a pre-filled field showing `https://api.orpheum-app.com/core/v1`; press *Connect*; read "To claim this
server, sign in at the discovery and enter the code"; open another site with no link given; ask
for a magic link; switch to e-mail; return; type the code; (with Q19) compare a check code; return
to System; wait for "Claimed". If the e-mail is slow or in spam, the 30 minutes run out. The design
does not say what the core shows then **[I]**. The words "discovery", "claim", "clients",
"WebRTC", "IPv6" and "NAT-PMP" all face the person the spec calls the likely payer, "the less
technical owner" (spec §5.2).

**What comparable products do.** Plex claims a server when the signed-in owner opens it on the LAN:
"this will initiate the process to claim the server" **[V]**. Its Docker claim token comes from a
page and lasts minutes, not half an hour **[V]**. Synology names the feature *QuickConnect* and
says setup "takes about a minute" **[V]**. Home Assistant calls it *remote access*. GOV.UK warns
that e-mail loops "can be disruptive because they force users to switch from your service to their
email account and back again" **[V]**.

**Recommendation.**
- **Make the claim one click.** System → *Turn on access from outside the home* → opens
  `account.orpheum-app.com/claim#<code>` in a new tab; sign in there if needed; the page shows the
  server's name and asks *Add this server to your account?* Typing the code stays as the fallback.
- **Re-issue the claim code by itself** when it expires, and say so: "The code expired, here is a
  new one."
- **Q19: confirm on the core, not by comparing.** Rather than a check code on both screens, have the
  System page show "*kstenerud@…* wants to add this server to their account. Allow?" The admin is
  already there, the core is the authority, and a request from a stranger is plain to see **[I]**.
- **Plain names.** Call it *Remote access* (or a brand like *Orpheum Connect*) in the core's UI.
  Hide the address field behind "Use my own discovery server instead…". Keep paths and port
  mappings under *Details* for the curious.
- **First-run setup in the browser.** The setup code in the log is the first thing the owner meets.
  Show it on the console and accept it on the first page the LAN browser opens, as Jellyfin's and
  Plex's first-run pages do **[I]**.

### U6 — Two recoveries quietly cost every household device its link (medium)

**Journeys:** lost identity key or restored backup; subscription lapsed. **Sections:** protocol
§2.1, §2.8; spec §3.10, Q8, Q12.

**What the design says.** Scheduled backups never hold `identity.key` (Q12). Restoring without it
means re-keying: a claim code from the new key, typed where the core was claimed, keeps the
`coreId` and every locator. After `released`, the `coreId` is retired; "users must be sent new
links".

**What a user would experience.** The common disaster, a dead disk restored from a scheduled backup,
always loses the key **[I]**. The core starts with a new key. Its old session fails. The owner sees
System → Discovery and the obvious button is *Connect*, which enrols a **new** core: every device's
locator, every link and every remembered path now point at a retired id **[I]**. The design does
not say what the System page shows in this state. After a lapse into `released`, resubscribing
leaves every family member, TV and guest link dead, though the same server with the same key comes
back.

**What comparable products do.** Plex's guide to moving a server warns that "it may not be possible
to retain any existing library access that may have been granted to other Plex accounts" **[V]**:
losing the server's identity is a known, resented cost there. For Home Assistant Cloud, local access
continues when the subscription lapses and renewing restores remote access **[V, secondary]**.

**Recommendation.**
- **Detect the case on the core.** If the core remembers a `coreId` but its key no longer opens a
  session, System says: "This server's identity key changed (restored from a backup?). Re-connect it
  to keep your household's links working," with one button for the re-key. Hide *Connect* there.
- **On the dashboard**, when a new core is claimed by an account that has an offline core, ask: "Is
  this a replacement for *Stenerud home*?" and re-key if yes.
- **Let a released core come back with the same key.** Retire the id for others, never for the key
  that held it. A resubscribing household then keeps every link. The "never reused" guarantee is
  about reaching someone else's server, which the same key cannot do **[I]**.
- **Say on the backup page** that scheduled backups leave out the server's identity and what that
  costs, so the owner chooses knowingly.

### U7 — Lapse and outage messages contradict the spec and leave household members guessing (medium)

**Journeys:** subscription lapsed, discovery down. **Sections:** spec §3.10, §7.6;
`locales/en.ts` (`system.discovery.state`).

**What the design says.** In `frozen`, the spec says "`app.` still finds the core for remembered
devices", and the core shows "Discovery has ended. Your users can reach you until your address changes
or your certificate expires. Switch to your own discovery: <steps>". During an outage, returning
devices on a remembered HTTPS path keep working; new devices, WebRTC-only cores, claims and codes
break.

**What a user would experience.**
- The built strings disagree with the spec. `grace` says "for a few more days" where the spec says
  "Discovery ends on <date>". `frozen` says "clients cannot find this server through the discovery
  until it is renewed", where the spec says remembered devices still find it.
- "Your users can reach you" is unclear: reach whom? "Until your address changes" asks the owner to
  predict their ISP. The first action offered is to switch to a VPS, not to renew.
- Household members get no message at all. When a frozen core's address changes, or the service is
  down for a guest or a WebRTC-only household, the person sees a generic failure or a browser error
  page.

**What comparable products do.** Plex's outages left people unable to reach servers on their own
network, and How-To Geek wrote it up as news **[V]**: being on the path is visible and blamed.
Orpheum keeps the LAN working, which is better, but family members only know `app.`, not the LAN
address.

**Recommendation.**
- Align the strings with the spec, and give dates: "Remote access ends on 2 November unless the
  subscription is renewed. [Renew]". In `frozen`: "Remote access is paused. Devices that already
  know this server keep working until its address changes. [Renew] · Other options".
- **Tell household members the cause** without billing detail: "Remote access to *Stenerud home* is
  paused. At home it still works. Ask *Karl*." The service knows the state for the locator.
- **Serve the client's files from somewhere that outlives the API** (spec §7.1 already allows a CDN)
  and show "Orpheum's connection service is down. Your server is fine. Status: <link>" instead of a
  browser error.
- Keep the last good LAN address with the device so the cached client can still reach the server at
  home when `app.` cannot help.

### U8 — A person on two servers has nowhere to switch (medium)

**Journeys:** invited outsiders (D243 §4), household member who forgot which server. **Sections:**
protocol §3.1 ("a device locator ... which the page keeps"); spec §2, Q5.

**What the design says.** The page keeps *a* locator. Q5, how a household user identifies which core
they have an account on, is open.

**What a user would experience.** A person in their own household and invited to a friend's opens
the friend's invite on the same phone. The design does not say whether the second locator replaces
the first, or how they get back **[I]**. Watch Together invites from several households compound it.

**What comparable products do.** Emby Connect's help uses this very case: "You have 2 brothers and 3
cousins running Emby Servers" **[V]**. Plex shows a server picker. 

**Recommendation.** Keep a list of locators, each with the server's friendly name (from the core)
and who is signed in there. Show a switcher when there is more than one. Never let a guest join
replace a member's locator. This also answers Q5: the device remembers, by name.

### U9 — Signing in a TV takes two codes, a typed name and no QR (medium)

**Journeys:** family member's TV, returning user. **Sections:** D288; protocol §3.1;
`code-sign-in.tsx`, `device-activation.tsx` and their strings.

**What the design says.** The TV shows an 8-character code. A signed-in person opens *Your account*,
types the code and a name under *Activate a device*. Both screens then show a check code with a large
warning; only "Yes, they match" signs the TV in.

**What a user would experience.** The direction is right: no typing on the TV. But the person must
find *Activate a device*, type 8 characters, invent a name, then compare a second code. The TV page's
button reads *Get a code*, which many people read as "text or e-mail me a code" **[I]**. The warning
is 45 words long.

**What comparable products do.** Apple's HIG for tvOS: "Prefer letting people use another device to
sign up or authenticate" **[V]**. Tailscale's Apple TV: "(Recommended) Scan the QR code" **[V]**.
Plex shows a 4-character code at `plex.tv/link` and, on most TVs, a QR **[V, secondary]**. Jellyfin's
Quick Connect is the same direction as D288 with a 6-character code and no second code **[V]**.
RFC 8628 §3.3.1 lets the device show a QR of a URI that carries the code, and then the server
"SHOULD display the user_code ... and ask them to verify that it matches" the device: the code
itself becomes the check **[V]**. Research on device pairing found compare-and-confirm "usable and
preferred" but "susceptible to security failures", since people learn that pressing *match*
finishes the task **[V]** (Kainda, Flechais, Roscoe). Microsoft moved its push approvals to number
matching, where the user types the number shown, for the same reason **[V]**.

**Recommendation.**
- **Show a QR beside the code** on the TV: `app.orpheum-app.com/activate#<code>`. Scanning it with a
  signed-in phone opens *Activate a device* with the code filled in. Typing stays possible.
- **Default the name** from the device ("TV browser", "Chrome on Windows") and let the person change
  it later.
- **For the check, make the person pick, not just agree:** show three codes on the phone and ask
  "Which one does your TV show?" One tap, and it cannot be passed without looking **[I]**.
- **Rename *Get a code*** to "Sign in with your phone" (or "with another device").
- **Shorten the warning** to two sentences: "Only continue if this TV is in front of you. Never enter
  a code someone sent you."

### U10 — Magic links: cross-device, scanners and superseded links (medium)

**Journeys:** owner's first setup, owner's recovery. **Sections:** spec §2 (*Sign-in for customers*).

**What the design says.** Single use, at least 24 hours, "invalidated by the next link asked for",
and "confirming on the page that opened it which address asked".

**What a user would experience.**
- If "the page that opened it" means only the browser that asked can use the link, the owner who asks
  on a laptop and reads mail on a phone fails **[I]**. The spec's wording is ambiguous.
- Corporate and some consumer mail scanners open every link in a message. Supabase's issue tracker
  records magic links "invalidated by corporate link scanning software" **[V]**. Putting the token
  after `#` defeats a plain fetch; a scanner that runs the page's script could still spend it **[I]**.
- An owner who presses *Send again* because the first mail is slow then clicks the first one and is
  told it no longer works.

**What comparable products do.** GOV.UK: expire a link when it is "superseded by a new link", and
when someone uses one, "explain that it's expired, and why"; "let users resend" **[V]**. The common
answer to cross-device sign-in is a short code in the same e-mail, which the person types on the
device that asked **[V]** (Scalekit).

**Recommendation.** Let the link work on any device. Make its page ask for one click, "Sign in as
k…@gmail.com", before spending the token, so a scanner never does. Put a 6-character code in the same
mail for typing on another device. When a superseded link is used, say "A newer link was sent at
14:02. Use that one, or send another."

### U11 — The design does not say what the person sees while paths race (low)

**Journeys:** every first visit. **Sections:** protocol §3.3.

**What the design says.** The first path to answer carries the page; the others get about 3 s; the
client then moves to the best.

**What a user would experience.** Usually nothing, which is right. But first visits that wait on
WebRTC (0.45 s set-up at 50 ms, more on a poor link) or on an unanswered LNA prompt can pass a second
with a blank page **[I]**. A move between paths during sign-in or the first play could stall a
request **[I]**.

**What comparable products do.** NN/g: 1 second is "the limit for the user's flow of thought to stay
uninterrupted", and past 10 seconds people need "feedback indicating when the computer expects to be
done" **[V]**.

**Recommendation.** After 1 s, show "Connecting to *Stenerud home*…"; after 10 s, say what is being
tried in plain words and offer the U3 advice. Never name paths. Move traffic only between requests,
never mid-sign-in or mid-segment. Give the admin a "how this device is connected" line (at home /
direct / through the router / peer-to-peer) in the status tray for diagnosis.

### U12 — The owner is not told when remote access gets worse (low)

**Journeys:** ISP changed, router replaced. **Sections:** protocol §2.4, §2.6; spec §1.3.

**What the design says.** The probe's result goes to the System page, and the dashboard says ahead of
time when a household cannot be reached.

**What a user would experience.** A new router with UPnP off or a closed IPv6 firewall silently moves
the household from `everywhere` to `ipv6OrWebrtc` or `nowhere`. The owner learns it from a family
member's failure **[I]**.

**Recommendation.** The service has the customer's e-mail: send one message when `reachableFrom`
drops and stays down for an hour, with the one most likely fix ("Your new router may have UPnP
turned off"). Let the owner turn these off.

### U13 — Smaller wording and recovery points (low)

- **Lost e-mail:** the spec offers proof by "a claim code shown on their core's admin page", but a
  claimed core shows no claim code. Add *Show an ownership code* to System → Remote access.
- **"a television say"** (`activate.explain`) reads oddly; "for example, a TV" is plainer.
- **"Clients"** appears in admin strings ("Clients can reach it from anywhere"). Say "devices" or
  "your household".
- **`reachable.ipv6OrWebrtc`** ("Clients with IPv6 reach it directly; the rest connect over WebRTC")
  should say what it means for people: "Reachable from most places. Some hotel or office networks may
  not reach it."
- **`reachable.nowhere`** ("The discovery could not reach it from outside. Clients at home still
  can.") needs the next step: what to check, linked to a help page.
- **The claim string** says "sign in at the discovery". Name the site and link it (U5).

---

## What is good

- **No third-party account for family.** A household member needs a link and a password; Plex makes
  every viewer a plex.tv account. This is the product's strongest UX advantage and the spec says so
  (§5.2).
- **The phone journey by link is four steps** with no app to install. Competitors need an app, an
  account or a tailnet.
- **D288's direction matches platform guidance:** the device that lacks a keyboard shows the code,
  and the device with one enters it (Apple HIG **[V]**, Jellyfin Quick Connect **[V]**).
- **Codes are forgiving:** safe32 accepts either case, `o` for `0`, `l`/`i` for `1`, and ignores
  dashes and spaces (D218 §3), as RFC 8628 §6.1 recommends **[V]**.
- **The LAN survives the service.** Plex lost local playback in its outages **[V]**; Orpheum's core
  keeps its own client (spec §7.6).
- **Warnings ahead of time:** the owner learns from the probe that a household is unreachable before
  a family member does (spec §1.3), and deprecation dates reach the System page (protocol §5).
- **The exit path is a requirement with a test** (D25, spec §3.10), not an afterthought.
- **Port mapping is on by default and shown**, with one switch (D273 §7).
- **The device-activation warning exists and is plain**; it needs shortening, not rethinking.

## Sources

Fetched copies are under `/yoloai/cache/discovery-audit/` (files starting `ux-`).

- Chrome for Developers, *New permission prompt for Local Network Access*:
  <https://developer.chrome.com/blog/local-network-access>
- Plex Forum, *Important note about the Plex Web app – Local Network Access* (6 Nov 2025):
  <https://forums.plex.tv/t/important-note-about-the-plex-web-app-local-network-access/933264/1>
- The Windows Club, *Look for and connect to any device on your local network*:
  <https://www.thewindowsclub.com/website-wants-to-connect-device-on-local-network>
- Okta, *Chrome device permissions*: <https://help.okta.com/eu/en-us/content/topics/end-user/ov-device-lna-prompt.htm>
- WebKit, *Full Third-Party Cookie Blocking and More* (7-day cap on script-writable storage):
  <https://webkit.org/blog/10218/full-third-party-cookie-blocking-and-more/>
- Microsoft Learn, *WebKit and Safari changes in iOS 11* (SFSafariViewController storage):
  <https://learn.microsoft.com/en-us/xamarin/ios/platform/introduction-to-ios11/web>; Okta, *A
  History of the Mobile SSO Experience in iOS*: <https://developer.okta.com/blog/2022/01/13/mobile-sso>
- RFC 8628, *OAuth 2.0 Device Authorization Grant* (§3.3.1, §5.4, §6.1):
  <https://www.rfc-editor.org/rfc/rfc8628>
- Apple Human Interface Guidelines, *Managing accounts*:
  <https://developer.apple.com/design/human-interface-guidelines/managing-accounts>
- Tailscale, *Apple TV*: <https://tailscale.com/kb/1280/appletv>; *Key expiry*:
  <https://tailscale.com/kb/1028/key-expiry>; *How NAT traversal works*:
  <https://tailscale.com/blog/how-nat-traversal-works>
- Jellyfin, *Quick Connect*: <https://jellyfin.org/docs/general/server/quick-connect/>
- Emby, *Emby Connect*: <https://emby.media/support/articles/Emby-Connect.html>
- Plex support, *Accessing a server through Relay*:
  <https://support.plex.tv/articles/216766168-accessing-a-server-through-relay/>; *Why can't the
  Plex app find or connect to my Plex Media Server?*:
  <https://support.plex.tv/articles/204604227-why-can-t-the-plex-app-find-or-connect-to-my-plex-media-server/>;
  *Move an install to another system*:
  <https://support.plex.tv/articles/201370363-move-an-install-to-another-system/>
- plexinc/pms-docker README (`PLEX_CLAIM`): <https://github.com/plexinc/pms-docker>; Plexopedia,
  *How to Claim Your Plex Server*: <https://www.plexopedia.com/plex-media-server/general/claim-server/>
- Plex link codes and QR (secondary): <https://macmyths.com/plex-tv-link-where-to-enter-the-tv-code-and-connect-your-account/>
- How-To Geek, *Your Plex server is inaccessible right now, even on your home network*:
  <https://www.howtogeek.com/your-plex-server-is-inaccessible-right-now-even-on-your-home-network/>
- WunderTech, *Synology QuickConnect: How to Set it Up, Log In, and Find Your ID*:
  <https://www.wundertech.net/synology-quickconnect-ultimate-setup-guide/>; Synology, *QuickConnect
  White Paper*: <https://kb.synology.com/en-sg/WP/Synology_QuickConnect_White_Paper/4>
- Home Assistant Cloud lapse (secondary): <https://evezone.evetech.co.za/quick-bytes/how-to-fix-not-being-able-to-access-home-assistant-remotely>
- Nielsen Norman Group, *Response Times: The 3 Important Limits*:
  <https://www.nngroup.com/articles/response-times-3-important-limits/>; *Error-Message
  Guidelines*: <https://www.nngroup.com/articles/error-message-guidelines/>
- GOV.UK Design System, *Error message*:
  <https://design-system.service.gov.uk/components/error-message/>; *Confirm an email address*:
  <https://design-system.service.gov.uk/patterns/confirm-an-email-address/>
- Kainda, Flechais, Roscoe, *Secure and Usable Out-Of-Band Channels for Ad hoc Mobile Device
  Interactions*: <https://www.cs.ox.ac.uk/files/2858/main%20(6).pdf>
- Microsoft Learn, *Number matching in MFA push notifications*:
  <https://learn.microsoft.com/en-us/entra/identity/authentication/how-to-mfa-number-match>
- Supabase auth, *Magic Links are invalidated by corporate link scanning software* (#1214):
  <https://github.com/supabase/auth/issues/1214>
- Scalekit, *OTP vs Magic link*: <https://www.scalekit.com/blog/otp-vs-magic-links-passwordless-authentication>
- In this repository: [discovery-protocol.md](../discovery-protocol.md);
  [reachability-approaches-2026-10-09.md](reachability-approaches-2026-10-09.md) (the 70% hole-punching
  figure, rebinding); decisions D72, D214, D218, D221, D243, D273, D274, D288 in
  [working-notes.md](../../decisions/working-notes.md); `web/apps/web/src/` (`sign-in.tsx`,
  `code-sign-in.tsx`, `device-activation.tsx`, `register.tsx`, `system-discovery.tsx`,
  `locales/en.ts`).
