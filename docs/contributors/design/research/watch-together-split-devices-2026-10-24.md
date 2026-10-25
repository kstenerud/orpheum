> **ABOUTME:** Research and a design sketch, for the owner to decide, of Watch Together split across
> one person's devices: the film on a TV that joins for one session by a code or QR code it shows,
> and voice and the remote on the person's phone. Read before designing the remote TV's join, a
> voice-only page, or anything that keys pacing, rights or volume to a device rather than a person.

# Watch Together Split Across a Person's Devices

- **Date:** 2026-10-24.
- **Requested by** the owner, 2026-10-24: *"a way in Watch Together for the movie to play on one
  device while the rest goes on another. So for example I join a session on my phone and my TV. The
  TV plays the movie only, and my phone does the voice chat and other controls that aren't directly
  related to the movie playback."* Then: *"It might be helpful to have a way for the TV to produce a
  QR code that you scan on your phone. Phone for voice + TV is going to be the way it's used, not
  laptop + TV. So you could go to a special Orpheum link (I own orpheum.cc), ask for a link code,
  redeem with a phone to link. It could even be a temporary commandeer of the TV, just for that
  session."*
- **Builds on:** [watch-together.md](../plans/watch-together.md) "Prepared for, not built" and its
  Open item "Voice on a second device"; [D120](../../decisions/working-notes.md#d120--a-participant-is-a-person-devices-attach-to-them-with-roles-and-the-cap-counts-people),
  [D124](../../decisions/working-notes.md#d124--the-playback-role-may-be-a-cast-receiver-and-for-a-guest-that-is-the-best-path-the-phone-controls-it-does-not-relay),
  [D188](../../decisions/working-notes.md#d188--the-interface-is-designed-against-the-fields-recorded-failures-accessibility-shapes-the-design-but-does-not-gate-cut-a-her-television-plays-and-her-phone-talks)
  §3, [D262](../../decisions/working-notes.md#d262--voice-keeps-working-on-a-bad-link-measured-honestly-sent-to-survive-loss-and-the-film-gives-way-to-it)
  §2, [D267](../../decisions/working-notes.md#d267--a-watch-together-membership-is-kept-by-the-browser-that-joined-or-by-the-account-of-whoever-is-signed-in-and-one-member-may-be-on-several-devices)
  §3, and the device codes of [D288](../../decisions/working-notes.md#d288--a-device-is-activated-by-the-code-it-shows-entered-on-a-device-already-signed-in-and-a-check-code-both-show),
  [D293](../../decisions/working-notes.md#d293--the-discoverys-open-questions-answered-a-live-instance-is-pinged-before-it-is-replaced-device-codes-through-the-service-sealed-by-hpke-on-p-256-in-the-beta-stagings-dns-token-in-the-sandbox-until-production)
  §2 and [D321](../../decisions/working-notes.md#d321--a-tv-shows-a-qr-code-carrying-its-keys-hash-which-activates-it-with-one-tap-the-typed-codes-check-code-is-committed).
- **Not a decision.** Nothing here is built or decided. §5 lists what the owner needs to answer.
  Web pages are cached under `/yoloai/cache/http/split-devices/`, read 2026-10-24.

## 1. What the design already says

Most of the model was decided a month ago and never built. This research adds how the second
device gets in, and what it may do once it is.

- **A person is a participant; devices attach to them with roles** (D120): `playback`, `voice`,
  `control`. "Voice companion" is a phone attached with `voice` while the TV holds `playback`. D120
  also said how the second device attaches: *"The first device shows a short code; the second enters
  it and joins the same membership rather than creating a new one"*, the device-authorisation flow
  ([D93](../../decisions/working-notes.md#d93--device-pairing-is-rfc-8628-shaped)) *"pointed at a
  session instead of an account"*.
- **The roster shows a person, not their devices**: *"'Karl (TV + phone)' is one row, not two"*
  (D120's status note).
- **The phone controls; it does not relay** (D124). The playback device fetches the film itself,
  so the film never passes through the phone.
- **Her TV's first path is its own browser**, *"joining the session as a device of her membership,
  by link or short code"* (D188 §3, as struck through and replaced there). Subtitles, the audio
  track and the film's volume are each person's own, never the group's (D188 §4).
- **The session never depends on an account**
  ([D191](../../decisions/working-notes.md#d191--watch-together-behaves-as-if-nobody-has-an-account-a-signed-in-participants-viewing-is-also-recorded-as-their-own)).
  So the TV's join cannot require the guest to be signed in.
- **One member may be on several devices**, each with a credential of its own (D267 §3). The owner
  asked for *"whatever makes this easier in future"* for exactly this case.
- **The plan's open item**: a link admits one person once, the credential stays in the browser that
  joined, and *"a second physical device has no way to join the same membership; opened on her
  phone, a second link makes a second member"* (watch-together.md, Open).

## 2. Prior art

### 2.1 Apple SharePlay: the call on the phone, the film on the TV

From Apple's guides ([Apple TV](https://support.apple.com/guide/tv/atvb65186d08/tvos),
[iPhone](https://support.apple.com/guide/iphone/shareplay-watch-listen-play-iphb657eb791/ios)):

- **The phone hands the film to the TV.** With a SharePlay session playing on the iPhone, the
  person picks the Apple TV as the playback destination, *"Enter the verification code if
  prompted"*, and *"The video plays in sync on Apple TV 4K, and you can keep the conversation going
  on your iPhone."* The FaceTime call never moves.
- **The TV need not be the person's.** The Apple TV guide recommends this route *"if you're not
  signed in as the current profile on Apple TV"*. The TV then plays for the phone's person without
  their sign-in. This is the owner's *temporary commandeer*. It requires the TV on the same network
  (*"an Apple TV 4K that's connected to the same network"*).
- **Everyone controls the shared position; each person controls their own presentation.**
  *"Each person watching the content can use the playback controls to play, pause, rewind, or
  fast-forward. (Settings like closed captioning and volume are controlled separately by each
  person.)"*
- **The film's audio gives way to voice**: *"smart volume"* turns the film down while people talk
  (search summaries of Apple's material; the wording on Apple's own page was not found, so
  UNVERIFIED in detail).
- **The video can move back** from the TV to the device on the call ("Move to ‹device›").

### 2.2 YouTube: "Link with TV code" and the phone as a remote

- **The TV shows a numeric code; the phone enters it** under Settings, Watch on TV. That works
  *"when you're not on the same Wi-Fi"* ([YouTube Help](https://support.google.com/youtube/answer/7640706)).
  The link lasts: *"Once you've linked your devices with a TV code, you can disconnect and
  reconnect them"*. Unlinking is all or nothing: *"Unlinking a single device will remove all linked
  devices."*
- **The full companion experience needs the same account on both**: *"You need to be logged into
  YouTube with the same Google Account on both your TV and your mobile device"*. A linked phone
  that is not the TV's account can still play to it and control it.
- **The protocol** ("Lounge API"), from a security researcher's write-up of a bug he reported
  ([xdavidhu, 2021-04-05](https://bugs.xdavidhu.me/google/2021/04/05/i-built-a-tv-that-plays-all-of-your-private-youtube-videos/)):
  the TV gets a `screen_id`, then a `lounge_token`, then a pairing PIN, which it shows. The phone
  sends the PIN to `/pairing/get_screen` and gets a `lounge_token` of its own. Commands then reach
  the TV through a long-polled `/bind` channel. The phone sends the TV commands; it does not carry
  the video.
- **What the TV may play is scoped per video.** To play the phone person's private video, the TV
  gets *"an extra video-specific token, called ctt"*, which *"only gives permission to watch that
  specific video"*. Orpheum's equivalent already exists: a viewer's cookie reaches only the film
  the session plays now ([D243](../../decisions/working-notes.md#d243--m4s-nine-questions-answered-a-guests-cookie-covers-one-film-invites-are-registration-links-under-roles-that-choose-libraries-the-names-certificate-comes-by-dns-01-through-cloudflare-and-a-vps-carries-every-route-in)
  §1).
- **The bug is the lesson**: the request that told the TV to play carried no CSRF token, so any
  page the phone person visited could make their TV play their private videos. Any request that
  drives another device needs the same authentication as any other.

### 2.3 Netflix: DIAL, casting, and its withdrawal

- **DIAL** (Netflix and YouTube, 2012) discovers TVs on the LAN by SSDP and launches an app there
  over REST ([Wikipedia](https://en.wikipedia.org/wiki/Discovery_and_Launch)). It launches the
  app; it carries no control. Both companies' phone remotes then spoke their own protocols. It is
  LAN-only.
- **Netflix withdrew phone casting** to most TVs in late 2025. Users are *"directed to use the
  remote that came with their TV hardware and use its native Netflix app"*
  ([MacRumors, 2025-12-01](https://www.macrumors.com/2025/12/01/netflix-kills-casting-from-mobile-app-to-tvs/);
  [Engadget](https://www.engadget.com/entertainment/streaming/netflix-ends-casting-from-mobile-devices-for-users-of-newer-tvs-151921578.html)).
  Netflix's stated reason was low adoption; reports also connect it with account sharing. One more
  withdrawn feature for [D150](../../decisions/working-notes.md#d150--read-the-withdrawals-not-just-the-requests-five-features-the-field-built-and-took-away)'s
  list. For Orpheum the warning is about what a phone remote costs to maintain, not about whether
  people want one.

### 2.4 Google Cast: sender and receiver

- **The sender controls; the receiver plays.** *"A Sender is an app or device that initiates and
  controls a Cast session, while a Receiver ... responds to Sender commands and handles content
  streaming"* ([Cast overview](https://developers.google.com/cast/docs/overview)). The receiver
  loads the media itself. A load request can carry *"Optional user credentials"*
  ([`LoadRequestData`](https://developers.google.com/cast/docs/reference/web_receiver/cast.framework.messages.LoadRequestData)),
  which is how a receiver plays as the sender's person without being signed in.
- **Several senders, one session**: *"one Sender could start a session on a Receiver and another
  Sender could join the same session to control playback"*.
- **The off-network path was withdrawn.** Guest mode let a visitor's phone cast with *"a 4-digit
  PIN shown on the screen saver"*, offered first by ultrasonic tones, the PIN *"reset every 24
  hours"*. Google removed it in 2023: *"the only way for guests to cast ... [is] by connecting to the
  host's Wi-Fi network"* ([9to5Google, 2023-08-23](https://9to5google.com/2023/08/23/chromecast-guest-mode/)).
  For Orpheum this fits D124's cast path: Cast needs the phone and TV on one network, which a
  guest's own phone and own TV are.
- **Casting, mirroring and extending as a first step**, and a custom receiver joined by the Cast
  session itself rather than a code, are weighed in [casting-2026-10-25.md](casting-2026-10-25.md).

### 2.5 Spotify Connect and Jam

- **Connect transfers playback to the speaker, and the phone stays the remote**: *"The phone
  continues to work as a remote control, so the user can pause playback, skip tracks, or change the
  volume"* ([Connect basics](https://developer.spotify.com/documentation/commercial-hardware/implementation/guides/connect-basics)).
- **A speaker belongs to whoever last took it.** ZeroConf logs the phone's person into the device
  with an encrypted credentials blob, and *"If a user's credentials are already stored, the existing
  user must be logged out"* ([ZeroConf API](https://developer.spotify.com/documentation/commercial-hardware/implementation/guides/zeroconf)).
  It is LAN discovery.
- **Jam on a TV is the closest match to the owner's sketch.** The TV shows a QR code, and phones
  that scan it join and control the queue: *"Spotify on TV supports in-person Jams which means that
  playback happens on the TV speakers"*, *"You don't need to be on the same WiFi for Jam to work"*
  ([Spotify support](https://support.spotify.com/us/article/jam/)). The host can review and remove
  the people who joined.

### 2.6 Disney+ GroupWatch, Plex Watch Together, Teleparty

- **GroupWatch** (withdrawn 2023-09-18) joined a TV through the account, not by pairing. You
  *"Accept the invite via web browser or a mobile device. On your TV, navigate to the chosen title.
  Click on the GroupWatch icon and select 'Join Stream'"* ([TechRadar](https://www.techradar.com/news/disney-plus-groupwatch);
  removal: [GamesRadar](https://www.gamesradar.com/Disney-plus-removed-groupwatch-feature/)). It had
  no voice, only reactions.
- **Plex Watch Together** (withdrawn from the new apps, 2025; web only): *"You can join the same
  session from multiple devices on the same account. Remember that each one is an individual
  stream"* ([Plex FAQ](https://support.plex.tv/articles/frequently-asked-questions-watch-together/)).
  Several devices were possible, but by account and with no roles: each device was a full viewer.
- **Teleparty** syncs browsers and mobile apps and adds chat, voice and video in the same app. No
  split between devices was found.

### 2.7 Discord: voice moved to another device

Discord's mobile app *"Transfer[s] Your Voice Call ... to Console"*: Xbox through the Xbox app,
PS5 through a linked PlayStation Network account
([Discord blog](https://discord.com/blog/how-to-talk-with-friends-in-crossplay-games-using-discord)).
That is the reverse of the owner's case: voice goes to the big device, and the phone sets it up. It
too is bound by an account link, not a code. Discord's Watch Together activity keeps voice and
video in one client.

### 2.8 Device codes and QR pairing: the security record

- **RFC 8628 names the attacks** ([RFC 8628](https://www.rfc-editor.org/rfc/rfc8628)):
  - **Brute force (§5.1)**: rate-limit the user code; an 8-character base-20 code is ~34.5 bits.
    Guessing a code binds the *victim's device to the attacker's account*, the reverse of a stolen
    token.
  - **Remote phishing (§5.4)**: the attacker starts the flow on their own device and sends the
    victim the code. The authorization server should *"inform the user that they are authorizing a
    device ... and to confirm that the device is in their possession"*, and *"SHOULD display
    information about the device"*. A QR shortcut (`verification_uri_complete`, §3.3.1) makes this
    *"particularly important ... as the user no longer has to type in the code"*. Lifetime is the
    other control: short enough *"to limit the viability of codes sent over email or text
    message"*.
  - **Session spying (§5.5)**: someone who sees the screen completes the flow first.
- **Device-code phishing is live and large.** Campaigns against Microsoft 365 use the real login
  page with attacker-started codes ([Sophos](https://www.sophos.com/en-us/blog/oauths-device-code-flow-abused-in-phishing-attacks);
  [CSA research note, 2026-03-25](https://labs.cloudsecurityalliance.org/research/csa-research-note-oauth-device-code-phishing-m365-20260325-c/)).
  The lure is always a code to enter on a page the victim trusts.
- **Orpheum already answered most of this for activating a TV**
  ([device-code-audit-2026-10-20.md](device-code-audit-2026-10-20.md)):
  - the code is entered only on a trusted device (D288 §1–§2);
  - the QR code carries the TV's key hash, so the key comes by camera past the service (D321 §1–§2);
  - the typed path has a committed check code that no one knows in advance (D321 §3);
  - the outcome is sealed to the TV's key (D293 §2);
  - the phone shows where and when the TV asked ([D322](../../decisions/working-notes.md#d322--activating-a-device-names-the-country-it-asks-from-as-cloudflare-names-it-the-networks-name-waits)).

  The audit's M4 applies here with more force: a phished device lasts. A grant that ends with the
  session bounds that harm by design.

### 2.9 What the prior art teaches

1. **The playback device fetches the film itself; the phone sends commands.** SharePlay, YouTube,
   Cast and Connect all work this way, and D124 decided the same.
2. **Shared position, personal presentation.** SharePlay keeps captions and volume per person, as
   D188 §4 does.
3. **Every product that let a guest drive someone else's screen off the network withdrew it**
   (Chromecast guest mode), or kept it bound to an account (GroupWatch, Discord, YouTube's full
   companion). The exceptions are YouTube's TV code and Spotify Jam's QR code, both pairings scoped
   to that TV. Orpheum's case is a person's own TV in their own house, joining a session elsewhere.
   That is YouTube's TV code pointed at a session, which D120 already proposed.
4. **A grant limited to one item** (YouTube's `ctt`) is the right size for a borrowed TV; Orpheum's
   per-film cookie already is one.
5. **No product found joins a borrowed TV to a group session for that session only.** SharePlay's
   unsigned-in Apple TV comes closest, but it is LAN-only and lasts only as long as the phone keeps
   handing it the film.

## 3. Where today's code assumes voice and film share a tab

Read at an earlier commit. Each of these must change, or be shown harmless, before a phone can carry voice
while a TV plays.

- **Voice-live is per tab.** `web/packages/sdk/src/voice-live.ts` keeps one module-level
  `isTabVoiceLive` (*"One tab carries one person's voice, so it is the tab's"*), and
  `web/packages/player/src/chunked.ts` reads `isVoiceLive` to cap quality and send
  `Orpheum-Pace-Kbps` (D262 §2). On a TV whose person talks on a phone, it is always false.
- **The voice device is attached from the player's tab.** `web/packages/sdk/src/together-voice.ts`
  attaches *"a second device of the membership with the voice role"* using the tab's own credential
  ([D281](../../decisions/working-notes.md#d281--everyone-in-a-session-hears-voice-from-the-start-talk-only-opens-the-microphone-and-mute-only-stops-ones-own-sending-the-hosts-pop-up-runs-voice-each-persons-volume-and-the-films-with-invites-and-people-in-dialogs-of-their-own)).
  No page attaches voice without also playing.
- **The viewer page is one screen**: `web/apps/web/src/together-viewer.tsx` holds the player, the
  speaker question, voice, the volumes and Leave together. There is no voice-and-remote page and no
  playback-only page.
- **A credential admits any role.** `Hub.Attach` (`internal/core/together/hub.go`) takes the role
  the device names: `playback`, `voice` or `both`. Nothing ties a credential to a role, so a
  credential handed to a TV could also carry voice, and one handed to a phone could also play.
- **There is no `control` role.** `DeviceRole` has `playback`, `voice` and `both`, against D120's
  three-row table. Commands are already per member, not per device (`member.may` in `group.go`),
  so a voice device may already send play, pause and seek as its person's rights allow. That half
  of "the phone is the remote" is in the protocol.
- **The host's leading player is "whichever device the host plays on"** (D267 §3). A host whose
  TV plays and whose phone talks needs the TV to lead and the phone's commands to be the host's.
  That is consistent with today's model, but untested.
- **Presentation lives in the playing tab.** Each person's film volume, the others' volumes
  (D281), and, once built, subtitles and the audio track (D188 §4) are set on the page that plays.
  No message carries them from one device of a person to another.
- **The speaker question** ([D184](../../decisions/working-notes.md#d184--the-defaults-audit-four-become-questions-seven-must-be-made-visible-the-rest-pass),
  D246 §6) is asked by the page that plays. With voice on the phone it belongs on the phone
  (earbuds or not), and the TV needs none.
- **A browser keeps one viewer membership**: `orpheum.together.viewer` in
  `web/apps/web/src/together-storage.ts`, in `localStorage`. A temporary grant kept there would
  outlive the session, and would collide with a TV that is also some other household's guest.
- **Upload share counts members with any device as players** (`PlayersHostedBy`,
  `internal/core/together/upload.go`; [D286](../../decisions/working-notes.md#d286--a-jump-cuts-every-run-of-a-quality-it-does-not-land-in-a-groups-player-keeps-the-films-own-picture-while-its-link-carries-it-encoded-devices-start-a-jump-on-a-common-quality-and-an-overloaded-server-moves-a-player-onto-a-quality-it-already-makes)
  §5). That is right for phone plus TV: one person, one player. But a phone that went on playing
  beside its TV would take two shares. `SendAwayNewest` picks by member, newest membership first,
  and sends `ended` to all of that member's devices, so the phone loses voice too when only the TV
  was sent away.
- **A member keeps their newest 16 credentials** (`credentialsKeptPerMember`,
  `memberships.go`). Linking many TVs, or one TV linked again and again, could push out the phone's
  credential.

## 4. The design sketch

### 4.1 The flow, as the person sees it

The guest is already in the session on their phone, by their link, in the phone's browser.

1. **On the TV's browser they type `orpheum.cc`.** Ten characters, against
   `app.orpheum-app.com/activate`'s thirty on a remote. The page asks the service for a code, as a
   TV activating does today (D293 §2: `POST /client/v1/device-codes`, with the TV's ephemeral P-256
   key and its commitment). It shows the code (`xxxx-xxxx`) and a QR code, and says: *"On your
   phone, in Watch Together, tap Watch on a TV."* The same screen also serves "sign this TV in to
   your household" (D321): the code is the same, and what the phone does with it decides which.
2. **On the phone, in the session, the person taps Watch on a TV.** It offers two ways:
   - **Scan the TV's code**: the phone's camera opens the QR link. The trouble is where it lands
     (§4.8).
   - **Type the code**: the committed check code then shows on both screens, with D288 §3's
     warning.

   Either way the phone names the TV as D321 and D322 do (*"A TV in Germany asked 12 s ago"*) and
   asks: *"Play this session on that TV, until the session ends?"*
3. **The TV asks too** (the device code audit's M1): *"Play ‹host›'s Watch Together on this TV for
   ‹guest name›, for this session only?"*, defaulting to No. This stops a stranger's session being
   pushed onto a TV whose code they saw.
4. **The TV plays.** It opens the host's core's client in a playback-only mode, reaching the core
   through the discovery by the locator in its sealed outcome, as a guest page does. It plays
   full-screen and shows nothing else but a small "‹guest› · phone connected" line and Leave.
5. **The phone stops playing and becomes the remote.** Its player detaches (one share of the
   upload, §3). It shows voice (Talk, Mute, each person's volume), the timeline with play, pause and
   seek as the person's rights allow (D271 §4), and the person's subtitles, audio track and film
   volume, which go to the TV. It also offers **Stop on TV**, which ends the TV's grant and resumes
   playing on the phone, as SharePlay's "Move to ‹device›" does.

The host can do the same: their phone hosts and talks, and their TV plays and leads.

### 4.2 The TV's grant

- **What it is**: a membership credential of the phone's member with a **fixed `playback` role**.
  The credential carries its role, so `Hub.Attach` refuses any other (§3). It is minted only by a
  confirmed pairing and sealed to the TV's key (D293 §2's HPKE), so the service never reads it.
- **What it reaches**: the film the session plays now, by the viewer cookie it already gets
  (`POST /together/playback`, D243 §1). It cannot browse, start voice, link another device, make
  links, or see what the phone sees but the roster. Whether the TV's own remote may pause is the
  person's rights (§5, Q4).
- **How long it lasts**: until the first of:
  - the session ends;
  - the member leaves or is removed (removal already cuts off every device of the member);
  - the phone taps Stop on TV, or the TV's Leave;
  - the member's place is taken by a new link (D267 §4 amended);
  - optionally, no voice device of that member has been connected for a set time (Q3).

  It is never written to the TV's `localStorage`. It goes in `sessionStorage`, under a key of its
  own, so closing the TV's tab ends it and nothing collides with a membership the TV keeps for
  itself (Q2).
- **The member keeps its credentials apart**: a playback-only grant does not count toward the
  newest 16 the phone's credentials are kept among, or the bound applies per role (§3).
- **Not a sign-in.** The TV is not listed among the person's devices and gains no account. A
  signed-in person who wants the TV for good activates it under D321 instead, from their account
  page.

### 4.3 The phone as voice and remote

- **Role**: the phone attaches as `voice` and sends commands as it may today. D120's `control` is
  then a page, not a role: the protocol needs none, since commands are per member (§3). Q5 asks
  whether to add it anyway, for a phone that only controls.
- **Shared commands** (play, pause, seek; what plays and stop for the host) go through the group as
  now. The TV is a follower like any other, or the leader when it is the host's.
- **Personal commands are new**: subtitles, audio track, film volume, and later levelling and the
  sound offset. They are relayed by the core *only to the same member's playback devices*. That is
  the same shape as `relay` for voice signalling, which already goes only to the device it names.
  They are never group state (D188 §4; SharePlay keeps the same split). The TV reports what it
  shows, and the phone displays that, so the two never disagree.
- **The phone needs no film**: the timeline is drawn from the group messages a voice device
  already receives (`broadcast` sends to every device), and posters come from the session's item.
  No player, no rendition, no part in sync: D120's *"implements none of the six"* holds.

### 4.4 Pacing per person (D262 §2)

The plan already says the core *"will pace that person's playback devices while any of their voice
devices is live"*. The sketch:

- **The voice device reports live or not** in its state, as the SDK already knows for its own tab.
- **The core tells the member's playback devices** whether that member's voice is live: a field on
  the welcome and a short message when it changes. The player's ABR then reads that instead of the
  tab-level flag. `voice-live.ts` becomes "this tab's, or what the core says of my person".
- **The phone may not share the TV's link.** On mobile data, pacing the TV costs it half its
  quality for nothing. Nothing detects home or away ([D247](../../decisions/working-notes.md#d247--a-slow-link-gets-a-lower-version-made-at-its-first-stall-and-played-from-its-second-and-a-group-waits-for-a-stalled-viewer-once-then-lets-them-catch-up-alone)
  §11). The core does see both devices' public source addresses, and the same address is a strong
  hint of one link (Q6).

### 4.5 Counting

- **The roster is one row per person**, with their devices beneath: "Karl · TV, phone" (D120).
  `DeviceView` already lists a member's devices with roles.
- **D120's cap counts people.** There is no cap now (D246 revised D119). Nothing changes: a TV adds
  no member.
- **Upload is shared per player.** The TV is that person's one player once the phone stops playing
  (§4.1 step 5). `SendAwayNewest` should send away the newest member's playback devices and leave
  their voice (Q7).
- **Bound the devices per member**, generously (D120: *"Device attachments are bounded separately
  and generously"*). For example, at most two linked TVs at once, a new one ending the oldest. The
  number is the owner's.

### 4.6 Drops and reconnects

- **The TV drops**: it is that member's player, so the group plays on and the TV catches up alone
  when it is back ([D250](../../decisions/working-notes.md#d250--a-watch-together-group-stops-only-at-its-own-sync-points-a-members-trouble-is-theirs-to-catch-up-on-from-what-is-already-being-made)
  §2). It reconnects by the credential in its tab's `sessionStorage`. A TV whose browser was killed
  (a TV browser closing on input switch is common) has lost it, and is linked again by a new code.
  The phone shows "TV reconnecting" and, past a while, "TV gone, link again".
- **The phone drops** (locked screen, call, lost signal): voice stops and the film plays on, since
  the TV needs nothing from the phone. The TV shows "phone disconnected". Whether the grant should
  then end after a while is Q3: ending it guards against a TV left playing in an empty room, but a
  phone whose browser is suspended on lock would lose the TV every time.
- **iOS Safari may stop the microphone when the screen locks** or Safari goes to the background.
  WebKit has a report of exactly that in a web view ([WebKit 226620](https://bugs.webkit.org/show_bug.cgi?id=226620)),
  and an Apple forum thread asks for background WebRTC in Safari
  ([forum 774239](https://developer.apple.com/forums/thread/774239)). Whether a locked iPhone keeps a
  Safari call going is UNVERIFIED, and it decides whether the phone page must ask the person to keep
  the screen on. It belongs in the device lab (D188 §3 already lists the earpiece routing).
- **Reconnecting is never a new member**: both devices hold credentials of the same member, so a
  drop on either never doubles the roster.

### 4.7 A guest's TV in another household, and a TV already signed in

- **The TV reaches the host's core from the guest's house** through the discovery, by the locator
  in its sealed outcome, as a guest's phone does (discovery-protocol §3.1, *"A locator belongs to a
  grant on the core (a device's session, or a Watch Together membership)"*). The TV's link is the
  guest's household's, and its bitrate controller learns it as any player's does. The guest's own
  core is not involved.
- **A TV signed in to the guest's own household keeps that sign-in.** The grant sits beside it in
  `sessionStorage`, for one tab, and opens a different origin: the host's core's client origin
  under `orpheum-client.com`
  ([D308](../../decisions/working-notes.md#d308--apporpheum-appcom-serves-a-small-loader-and-each-core-serves-its-own-client)
  §3). So the two never share storage. When the session ends, the tab goes back to `orpheum.cc`, and
  from there to the TV's own household if it has one.
- **The TV owner is the person linking, in the case the owner described.** For a TV in a third
  household (a friend's living room), the TV's own OK (§4.1 step 3) is the friend's consent, and the
  grant's end with the session is what makes it a *temporary* commandeer.

### 4.8 Where it fits the discovery and realm model

The TV knows no core and no realm, so its page cannot be a realm's app. Two choices:

- **(a) `orpheum.cc` is the paid service's short address**: it serves, or redirects to, the
  loader's TV page, and the code is the service's (D293 §2). The phone's page is the host's core's
  client, which claims the code from the service as it claims any code it did not issue
  (`claimFromService` in `internal/core/auth/devicecodes.go`). Today that claim is made by a
  signed-in person for an account. Here it is made by a membership credential, for a membership
  grant, and the sealed outcome says which kind of grant it carries. This reuses everything D321
  built. It works for every household on the paid service, and for none that self-hosts.
- **(b) `orpheum.cc` is a neutral rendezvous for any core**, including self-hosted ones: it issues
  codes and relays sealed outcomes, reading nothing. That makes the service carry traffic for cores
  that are not customers ([D298](../../decisions/working-notes.md#d298--a-realm-is-the-hosts-accounts-a-master-realm-chosen-once-a-subrealm-per-further-core-none-when-self-hosting)
  §1: self-hosting has no realm), and it has to limit strangers' asking itself (D288 §1). A
  self-hosted core would also need an account-free way to claim there.

**Where a scanned QR code lands** is the hard part of the scan path. The phone's camera opens the
QR link in the system browser at the address the TV printed: `orpheum.cc` or the loader. The guest's
membership is kept on the host's core's client origin (`together-storage.ts`, read by
`loader-at-client-origin.tsx`), which that page cannot read. Three ways:

1. **The loader remembers, per browser, which client origins hold a Watch Together membership**
   (the core id and the origin, never a credential), and forwards the scanned code there. D308 §4
   has the loader keep nothing of a sign-in; this keeps a pointer, not a secret. It still fails for
   an iPhone guest who joined in a home-screen app, since the camera opens Safari (discovery-protocol
   §3.1).
2. **The phone scans inside the page** (Watch on a TV opens the camera). `BarcodeDetector` is in
   Chrome on Android but not in Safari, so iPhones need a QR decoder library: a dependency
   (AGENTS rule 3).
3. **No scan: the phone types the code**, with the committed check code. It always works, costs a
   comparison, and needs nothing new.

The sketch's suggestion is 3 first and 1 after, the QR link carrying the TV's key hash as D321's
does, so a scan needs no check code (Q1).

### 4.9 Threats

| Threat | Effect | Answer |
| --- | --- | --- |
| A guest is sent a code ("enter ABCD-EFGH to fix your stream") | the attacker's device watches the host's film as that guest | the code is entered only in the session's own Watch on a TV; the committed check code, or the key by camera (D321); the TV's country and age (D322); the host sees "‹guest› · TV" in the roster and is told when a TV is linked; the grant ends with the session |
| Someone sees the TV's code and enters it first (RFC 8628 §5.5) | a stranger's session on this TV | the TV's own OK names the host and guest (§4.1 step 3); codes live ten minutes |
| Guessing codes | as above | the service's limits on asking and claiming (D288 §6; the device code audit's H2) |
| The TV's grant is copied off the TV | it plays this session's film until the session ends | it is playback-only and bound to its role; the film cookie reaches one film; it ends with the session |
| A command forged from another page (YouTube's CSRF) | the TV is driven by a page the person visited | commands ride the authenticated WebSocket, never a cookie-only request |
| A guest links a TV the host did not expect | the host's upload carries one more player | it is one player per person, as before; the host can remove the person; Q8 asks whether the host may forbid TVs per invite |

The guest's power does not grow. A guest can already show the film on any screen they hold, so a
playback grant gives them nothing new, only an easier way to use what they have.

### 4.10 Echo, now that the film is on speakers

D188 §3 already names it: *"Her phone's microphone hears the TV, so the owner hears the film
echoed back. Earbuds on her phone remove the path the owner would hear."* The split makes this the
normal case, not an edge case: the film is loud, across the room, and the phone's browser has no
reference to cancel it ([D63](../../decisions/working-notes.md#d63--voice-chat-is-webrtc-and-headphones-are-the-supported-configuration)).
So the phone's page should ask the speaker question as "earbuds on this phone?", and steer towards
them. The fallback is [D65](../../decisions/working-notes.md#d65--playback-offset-is-per-participant-and-it-is-an-echo-control)'s
offset alignment, which is still open in the plan.

### 4.11 What it would take

- **Core** (`internal/core/together`, `internal/core/auth`, `internal/core/api`):
  - credentials carry a role;
  - a member-credential claim of a device code, whose confirmed outcome mints a playback-only
    credential sealed to the TV;
  - "Stop on TV" and per-device ends;
  - personal commands relayed within a member;
  - voice-live per member, sent to its playback devices;
  - `SendAwayNewest` by device;
  - a device bound per member.
- **Service** (orpheum-connect): an outcome kind for a membership grant, and `orpheum.cc` pointed
  at the TV page (option a).
- **Web**:
  - the TV page at `orpheum.cc`, a variant of the device-code page;
  - a playback-only viewer mode that keeps its grant in `sessionStorage`;
  - the phone's voice-and-remote page;
  - Watch on a TV, with code entry;
  - `voice-live.ts` reading the core's word;
  - the roster's one row per person.
- **Tests**:
  - a TV credential refused any role but playback;
  - a phone's command moving the TV;
  - subtitles reaching only the same member's TV;
  - pacing reaching the TV while the phone talks;
  - the grant ending on each of §4.2's ends;
  - a scanned code refused for a key that is not the QR code's.

  Two items go to the device lab: the iPhone locked mid-call, and echo with the TV loud.

## 5. Open questions for the owner

1. **Scan or type first?** Typing with a committed check code works everywhere today. Scanning
   needs the loader to remember where the person's memberships are (§4.8 way 1), or a QR library
   in the page (way 2, a dependency). Which, and in what order?
2. **Where the grant lives on the TV**: `sessionStorage`, lost when the TV's browser closes, or
   `localStorage` until the session ends, surviving a TV browser that is killed on input change.
3. **Does the TV's grant end when the phone is gone?** Never (it ends with the session); or after
   some minutes with no voice device of that person; or only by Stop on TV and Leave. A phone
   browser suspended on lock argues against a short timeout.
4. **What the TV's own remote may do**: nothing but Leave; or play and pause as the person's
   rights allow (D271 §4). The phone is the remote, but the TV's remote is in the hand.
5. **A `control` role**: add it to the protocol for a phone that controls without voice, or keep
   control as a page of the `voice` device (commands are per member already).
6. **Pacing when the phone is on mobile data**: pace the TV whenever the person's voice is live
   (simple, sometimes wasteful); or only when phone and TV share a public address (a hint, not
   proof).
7. **Upload overload**: send away the newest player only, keeping its person's voice; or the whole
   member, as now.
8. **May the host forbid a guest's TV**, a right per invite like D271 §4's? And is the host told
   when a TV is linked?
9. **`orpheum.cc`'s role** (§4.8): the paid service's short address only (a), or a neutral
   rendezvous for self-hosted cores too (b). If (a), what a self-hosted guest's TV does: open the
   owner's VPS address, or fall back to the laptop.
10. **One TV code for both purposes**: should `orpheum.cc`'s single code also activate the TV for a
    household (D321), the phone deciding which, or should Watch Together have a page and code of its
    own so a TV owner knows which they are agreeing to?
11. **Does Cut A's "voice on her phone while her computer plays" (D245 §1) use this flow**, with the
    laptop in the TV's place? The flow is the same, with the laptop's browser as the playback
    device, and it would close the plan's Open item before any TV.

## 6. Not tried

- **No product was run.** SharePlay's behaviour with an unsigned-in Apple TV, YouTube's TV code,
  Spotify Jam on a TV and Discord's voice transfer come from their help pages and reports, not
  from use. Whether YouTube's or SharePlay's link ends when the phone leaves was not found.
- **Netflix's own second-screen protocol** (often called MDX) was not found in any public source;
  only DIAL and the 2025 casting withdrawal were.
- **Apple's "smart volume"** wording on Apple's own pages was not found; the claim rests on
  secondary summaries.
- **Teleparty's TV and mobile apps** were not examined beyond search summaries; whether they split
  chat and video across devices is unknown.
- **Discord's support pages** were blocked to the fetch (a JavaScript challenge); the Discord claims
  rest on its blog.
- **iOS Safari with a locked screen during a WebRTC call**, and the echo of a loud TV into a
  phone's microphone, were not tested on a device.
- **TV browsers' `sessionStorage` across a TV's sleep or input change** (TV Bro on the owner's TCL
  TV, Tizen, webOS) was not checked.
- **No prototype**: no code was written, and nothing in §4 is measured.
