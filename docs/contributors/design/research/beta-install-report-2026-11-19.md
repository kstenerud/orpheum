> **ABOUTME:** The first closed-beta tester's install on OpenMediaVault (2026-11-19), from his own
> report and the owner's console, with the lead's triage of each difficulty. Titles from his library
> are left out (rule 12): a series is "a series of his", and so on. Input to the setup review the
> owner asked for the same day; not a specification.

# The first beta install (2026-11-19)

The owner (2026-11-19): *"it's a good example of the friction of setting things up. He got really
frustrated and spent hours trying to get this going, and still failed. There are too many steps and
too many fiddly things."* On posters, the tester: *"I never had to do this on jellyfin or kodi, why
do I need an account just to get the same functionality?"* His closing line, on a file view, said
with a grin: *"a plain folder view as a fallback would have saved me an evening."*

**Who and how.** A friend of the owner, technical, on OpenMediaVault (Docker through the compose
plugin and Portainer, media on a mergerfs pool), release v0.8.5. An AI agent in his browser did the
install from the documentation, with him typing passwords and deciding.

**Outcome.** The server at home installed and runs: core and node healthy, the media folder on the
mergerfs pool mounted, the first admin made, two libraries scanned. Away from home never worked. He
redeemed his invitation and chose his realm, but never connected and claimed his server: the
operator panel's Accounts table showed his account verified, with **Server: none**. Every realm
sign-in he tried therefore said "The realm, name or password is wrong", which sent him to check his
password, not to the missing step. (The second tester's account showed no server either.)

## What he met, in order

The numbers are his report's. **Kind**: *steps* (a step that could go), *words* (the guide or a
page says it wrong or not at all), *bug*, *feature* (asked for, not built).

### Finding the instructions

| # | What happened | Kind |
| --- | --- | --- |
| 1 | orpheum.cc shows only the realm sign-in: no visible install, download or documentation link. | words |
| 2 | The guide is reachable only through visually hidden links (`/llms.txt`, `/agent-setup.md`). | words |
| 3 | Both lead to GitHub: the guide is two hops from orpheum.cc. | words |

### Fitting the guide to his machine (OMV)

| # | What happened | Kind |
| --- | --- | --- |
| 4 | OMV's global environment file was empty (no omv-extras Jellyfin setup before). Editing it touches every compose app; he put the five values in Orpheum's own Environment box, which the compose file already supports. | steps |
| 5 | The guide requires a dedicated `appuser`, which needs a password typed in OMV; no alternative is offered (a login-less system user, or his own UID). | steps |
| 6 | mergerfs pool, appdata at `/appdata`: the guide never mentions mergerfs. It worked; the node reports the mount as `Kind: unknown`, `fuse.mergerfs`. | words |
| 7 | OMV's UI cannot show a folder inside a shared folder: the media path had to be known already. | steps |
| 8 | The folder's capital letter was first given in lower case; `create_host_path: false` stopped the start with a clear error. Worth keeping. | (good) |

### Entering the compose file

| # | What happened | Kind |
| --- | --- | --- |
| 9 | This OMV has one Add form, and its Environment box is hidden behind a **Show environment file** tickbox the guide does not mention; the guide says Add, then Add again. | words |
| 10 | Pasting 7 KB into OMV's CodeMirror editor: typing risks auto-indent mangling the YAML. "Paste, don't type" would help. | words |
| 11 | The compose file's own header repeats the out-of-date OMV wording and the global-env assumption. | words |
| 12–15 | His agent's own slips (did not ask about away access up front, a wrong OMV URL, the setup code read from a screenshot). Not Orpheum's. | — |

### Setup: first admin, libraries, first scan

| # | What happened | Kind |
| --- | --- | --- |
| 16 | The films library was given the media root, which holds films, series and two unrelated folders: a folder of series showed as a series, episodes as films, 895 not identified. The library form pre-lists the root as a tick-box, which makes it the easy path. | steps, bug |
| 17 | Adding a subfolder of a folder the library already reads is refused with "The server could not accept that. Check what was entered and try again." The overlap is not named. | bug |
| 18 | Taking a folder out of a library offers only "This folder is gone for good…" and "This folder moved…"; no plain **Remove**. | words |
| 19 | The first library rescans every 24 hours; a library made later defaults to **Only when I ask**. | bug? |
| 19a | The first scan of a large library shows little progress on the library page; the real progress (749 waiting) is on the System page, which nothing points to. | feature |
| 19b | A file FFmpeg cannot read ("Invalid data found when processing input"), world-readable, gets the permissions advice (`chown`/`chmod`): the advice seems attached to every read failure. | bug |
| 19c | No GPU: the failed hardware probes are shown in full. Harmless. | — |
| 20 | **No posters at all**, because no TMDB key is set. Only the System page says so. The guide never says a key is needed, and Jellyfin and Kodi never asked him for one: their authors register the app once and ship the key. | steps |
| 21 | A series of his sits in his films folder and showed as a series from the films library. His filing; Orpheum is right. | — |
| 22 | The session lasted across closing the tab. The guide does not say how long a sign-in lasts. | words |

### Other devices

| # | What happened | Kind |
| --- | --- | --- |
| 23, 28c, 28d | **A blank page on an NVIDIA Shield** whose Chrome was 67 (2018, sideloaded long ago; the Play Store offers no Chrome there). account.orpheum.cc rendered; orpheum.cc did not. Sideloading Chrome 157 fixed it. No "this browser is too old" message; no minimum browser in the guide. | bug, words |
| 24 | The guide names no supported TV browsers or apps, and setup-problems.md has no blank-page entry. | words |
| 25 | Both a phone and a TV were first pointed at orpheum.cc, not at the server's own address at home. The sign-in page does not say "at home, open your server's address". | words |
| 26 | **The first screen asks for a realm he had never set up**, and nothing says a realm exists only after an invitation, an account, a key and a claim, or that none of it is needed at home. The owner's server's sign-in he had been given was taken for one for Orpheum in general. | steps, words |
| 27 | "Realm" is jargon (D526: "household"). | words |
| 27b | His proposal: name and password alone, orpheum.cc mapping names to realms. Options his reviewer listed: the realm kept but hidden, filled from the invitation link and remembered per device; the account's e-mail looking up the realm; global user names. | (design) |
| 28 | Nothing on the server or on orpheum.cc says how to get an orpheum.cc account (by invitation only) or whom to ask. | words |
| 28b | account.orpheum.cc/signin's sentence is the clearest account of the two levels anywhere ("This is your Orpheum service account, not your server's…"), but a new user reaches it only by searching. | words |
| 29 | Good: a TV signs in by a code. The guide's "Watching" could say so. | words |
| 30 | Tablet: tapping the sides skips ±10 s, but nothing hints at it, and a swipe does nothing. Low priority. | feature |
| 31 | TV: six large cards a row, 1.5 rows a screen; no card-size setting. A "Sample preview" item he did not add (a scene release's sample file, very likely). | feature, words |
| 32 | Needs attention: "1 files or folders could not be read" names no file; the intro sentence is garbled; four series with unnumbered episodes. One of them numbers its episodes across the whole show (`N. Title`) inside `Series 1`, `Series 2` folders. | bug, feature |
| 33 | **No folder view**: when names do not parse, nothing in Orpheum reaches the files. | feature |

## The lead's reading

**The away-from-home chain is where he failed, and it has five places to stop:** an invitation, an
account with a realm, a key made on one site, Connect on the server, the key pasted with the
password again for Claim; then each person signs in once at home. He stopped after the second, and
nothing anywhere said a step was left: the account page did not say "no server linked yet", the
server's Remote access did not say an invitation was needed, and the sign-in's one message ("wrong")
is the same for every failure, by design (`discovery/single/doc.go`).

**Posters are the second failure, and the cheapest to remove:** a key shipped with the release,
the Settings field kept to override it.

**Most of the rest is words:** the guide's OMV steps, a visible Install link, a pointer from the
sign-in to the home address, a minimum browser and a "too old" page instead of a blank one. A few
are bugs to fix as such (17, 19, 19b, 32's wording).

The proposals for the owner are in the setup review that follows this report.
