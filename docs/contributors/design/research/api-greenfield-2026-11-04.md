> **ABOUTME:** A greenfield review of the core's HTTP API and socket, made while nothing is deployed
> to other people: the whole current surface with its callers, the planned features it must carry
> (music first), every inconsistency found, a full old → new route table, what the move costs, the
> owner's questions, and draft decision text.

# The API, greenfield (2026-11-04)

The owner, 2026-11-04: *"For the API endpoints: let's Greenfield them. Nothing is deployed to other
people so this is the cheapest time for invasive changes. We can use account or user where we use
people now. Look at our whole API and make sure it will handle the future things we have planned
(for example, music)."*

This is a proposal. Nothing here is built, and nothing in it is decided until the owner answers §6
and the text of §7 is entered in the log under a number.

**Method.** The inventory is read from `api/openapi.json` at "feat(web): Films and Series narrow by genre, decade and unwatched" (143 paths, 169 operations,
277 schemas) and from the socket messages in `internal/core/together/messages.go` and
`internal/core/live/messages.go`. "Called from" is a search of `web/apps` and `web/packages` for
each path as `api.GET("/…")` and the like (generated SDK excluded). The future needs are read from
the decision log, the plans and the research named against each.

## 1. What the core serves now

### 1.1 Outside the API document

| Path | What | Caller |
| --- | --- | --- |
| `GET /api/v1/together/ws` | The one WebSocket: Watch Together and the device presence channel ("live") | sdk `together.ts` and `live-connection.ts`; web `use-together.ts`, `live.tsx` |
| `/api/v1/bytes/…` | Media and art bytes (in the document; D216 puts them under the API prefix for the media cookie) | the player, `artUrl` |
| `/node/v1/{hello,commands,status,result,progress,changed}` | Node → core protocol (internal, `internal/nodeproto`) | the media node |
| `/bytes/v1/{file,chunks/,result/,probe,folders}` | The node's byte server (internal) | the core |
| `/debug/pprof/…` | Profiling | operators |
| `/` | The web client's files | browsers |

The node protocol is internal, versioned on its own, and out of scope here: core and node ship as
one binary.

### 1.2 The socket's messages

| Channel | Direction | Types |
| --- | --- | --- |
| Watch Together (ProtocolVersion 2) | client → core | `hello`, `ping`, `state`, `command` (`play`, `pause`, `seek`, `rate`, `changeItem` with a `fileId`, `stop`), `suspend`, `resume`, `rtc`, `note`, `standing`, `sound` |
| Watch Together | core → client | `welcome`, `pong`, `group`, `roster`, `rejected`, `ended` |
| Presence ("live"), opened by a hello of type `live` | device → core | `live`, `ping`, `front` |
| Presence | core → device | `devices`, `receive`, `sending`, `pong`, `signedOut` (casts carry `castId`, `fileId`, `titleName`, `membershipCredential`, `receiverDeviceId`) |

### 1.3 The HTTP operations, by the document's tags

`Purpose` is the operation's summary, cut at 90 characters. `Shape` is query parameters, the request
body's schema and the success response (schema names without their `Body`/`InputBody`/`OutputBody`
suffix). "none in web/" means no client in `web/` calls it.

**Setup and auth** (10)

| Method | Path | operationId | Purpose | Shape | Called from |
| --- | --- | --- | --- | --- | --- |
| GET | `/client-manifest` | getClientManifest | The core's client's files by their hashes, signed by its identity key (D308) | → 200 ClientManifest | loader-run-client.ts |
| GET | `/me` | getMe | Who this session belongs to | → 200 User | account.tsx, app.tsx, browse.tsx, client-main.tsx, live.tsx, person.tsx, together-join.tsx |
| POST | `/me/locator` | freshLocator | A fresh locator for this device, by which it finds its server at a discovery | → 200 Locator | **none in web/** |
| GET | `/me/own-link` | getOwnLink | Your own link, which leads your devices back to this server through its discovery | → 200 OwnLink | accounts.ts |
| DELETE | `/session` | signOut | Sign out, revoking this session | → 204 | app.tsx |
| POST | `/session` | signIn | Sign in; returns a session token for the Authorization header | SignIn → 201 Session | accounts.ts |
| POST | `/session/handoff` | handOffSession | Sign in at the discovery's loader; returns a code the client origin takes the session… | SignIn → 201 HandOff | loader-at-loader-origin.tsx |
| POST | `/session/redeem` | redeemSession | Take the session a sign-in at the loader kept, by its code, once | Redeem → 201 Session | loader-at-client-origin.tsx |
| GET | `/setup` | getSetup | Whether the server still needs its first admin, so the sign-in page opens on setting it up | → 200 Setup | accounts.ts |
| POST | `/setup` | setup | Create the first admin with the one-time setup code from the log | Setup → 201 User | accounts.ts |

**Accounts** (37)

| Method | Path | operationId | Purpose | Shape | Called from |
| --- | --- | --- | --- | --- | --- |
| POST | `/activations` | activateDevice | Enter the code a device shows; both then show a check code (D288) | Activation → 200 Activation | accounts.ts |
| POST | `/activations/answer` | answerActivation | Say whether the check codes match: only a match activates the device | ActivationAnswer → 204 | accounts.ts |
| GET | `/invitations` | listInvitations | Every invitation: those waiting, and those used with the account each made (admin) | → 200 Invitations | accounts.ts |
| POST | `/invitations` | createInvitation | Invite someone to make their own account under a role, noting who it is for (admin; D246) | Invite → 201 Invite | accounts.ts |
| DELETE | `/invitations/{id}` | withdrawInvitation | Withdraw an invitation still waiting: none of its links admits anyone (admin) | → 204 | accounts.ts |
| PUT | `/me/admin-away` | setAdminAway | Whether you administer from away from home too: never (the default) or always; set… | AdminAway → 204 | accounts.ts |
| GET | `/me/casts` | listMyCasts | The casts this device sends or receives, each with a credential for it | → 200 MyCasts | cast.ts |
| POST | `/me/casts` | startCast | Start casting from this device to another of one's own, connected now (D337, D364) | StartCast → 201 Cast | cast.ts |
| POST | `/me/casts/{id}/accept` | acceptCast | Say Use, on its receiver, to a cast it was asked about (D364 §2) | → 200 Cast | cast.ts |
| POST | `/me/casts/{id}/end` | endCast | End a cast this device sends or receives, and say where it stood | EndCast → 200 EndCast | cast.ts |
| POST | `/me/casts/{id}/next` | moveCastOn | Move a cast this device sends or receives on to what plays after its film, the next… | → 200 Cast | cast.ts |
| POST | `/me/casts/{id}/play` | playInCast | Play a film on a cast's receiver, or nothing, the receiver ready for the next (D364… | CastFilm → 200 Cast | cast.ts |
| GET | `/me/devices` | listMyDevices | One's own devices, most recently used first (D218) | → 200 Devices | accounts.ts |
| POST | `/me/devices/sign-out-elsewhere` | signOutElsewhere | Sign out every device but this one; they stay listed | → 204 | accounts.ts |
| DELETE | `/me/devices/{id}` | removeMyDevice | Remove one of one's devices; it is signed out at its next request | → 204 | accounts.ts |
| PUT | `/me/devices/{id}` | changeMyDevice | Rename one of one's devices, or set how far it moves its sound | ChangeDevice → 204 | accounts.ts |
| PUT | `/me/password` | changePassword | Change one's own password; other sessions stay signed in | ChangePassword → 204 | accounts.ts |
| POST | `/pairings` | requestPairing | A code for this device to show, which a person signed in enters to activate it (D288) | PairingRequest → 200 Pairing | pairing.ts |
| POST | `/pairings/session` | takePairedSession | The session a confirmed pairing made, once | PairingSecret → 201 Session | pairing.ts |
| POST | `/pairings/state` | getPairingState | How this device's pairing stands, and the check code to show | PairingSecret → 200 PairingState | pairing.ts |
| POST | `/registration` | register | Use an invitation or a registration link: make the account or set a new password, and… | Register → 200 Session | accounts.ts |
| POST | `/registration/link` | getLinkKind | What a link is for, without using it: an invitation or a registration link | LinkKind → 200 LinkKind | accounts.ts |
| GET | `/roles` | listRoles | Every role, with its capabilities and the libraries it reaches (admin; D243 §4) | → 200 Roles | roles.ts |
| POST | `/roles` | createRole | Make a role: its name, capabilities and libraries (admin; D243 §4) | RoleChange → 201 CreateRole | roles.ts |
| DELETE | `/roles/{id}` | deleteRole | Delete a role the admin made that no one holds (admin) | → 204 | roles.ts |
| PUT | `/roles/{id}` | updateRole | Change a role; a library taken back is gone for its members at their next request… | RoleChange → 204 | roles.ts |
| GET | `/users` | listAccounts | Everyone, for the admin | → 200 Accounts | accounts.ts |
| POST | `/users` | createAccount | Set a person's account up in full: a name, a password and a role (D246 §2) | CreateAccount → 201 CreateAccount | accounts.ts |
| DELETE | `/users/{id}` | removePerson | Remove a person: their sessions and devices end, and their own watching is deleted (S40) | → 204 | accounts.ts |
| GET | `/users/{id}` | getPerson | A person's page, for the admin: who they are and what they have watched | → 200 Person | accounts.ts |
| PUT | `/users/{id}` | renamePerson | Rename a person; names are for display and sign-in, and stay unique | RenamePerson → 204 | accounts.ts |
| PUT | `/users/{id}/admin-note` | setAdminNote | Change the admin's note of who a person is for (admin) | AdminNote → 204 | accounts.ts |
| GET | `/users/{id}/devices` | listPersonDevices | A person's devices, for the admin | → 200 Devices | accounts.ts |
| DELETE | `/users/{id}/devices/{deviceId}` | removePersonDevice | Remove a person's device, for the admin: a lost or given-away one | → 204 | accounts.ts |
| DELETE | `/users/{id}/own-link` | revokeOwnLink | End a person's own link, which leads them back to the server through a discovery (D292 §4) | → 204 | accounts.ts |
| POST | `/users/{id}/registration-invitation` | issueRegistrationInvitation | A new registration link for a person, withdrawing the old; their password works until used | → 200 IssuedSecret | accounts.ts |
| PUT | `/users/{id}/role` | setPersonsRole | Give a person a role, their whole access, from their next request (admin; D243 §4) | PersonsRole → 204 | roles.ts |

**Browse** (19)

| Method | Path | operationId | Purpose | Shape | Called from |
| --- | --- | --- | --- | --- | --- |
| GET | `/bytes/art/{sourceSha256}/{variant}` | getArt | One size of one image, in WebP or JPEG (D212) | → 200 image/webp | sdk artUrl (every poster) |
| GET | `/collections/{id}` | getSeries | A series with its seasons, each with how many episodes it has | → 200 Collection | series.tsx |
| GET | `/collections/{id}/not-in-order` | getNotInOrdering | A page of a series' episodes the ordering the person watches it in leaves out, in… | ?cursor&limit → 200 Season | series.tsx |
| PUT | `/collections/{id}/preferences` | setSeriesPreferences | Keep a person's choices in one series: the ordering they watch it in, which its pages,… | SetSeriesPreferences → 204 | series.tsx |
| GET | `/collections/{id}/seasons/{season}` | getSeason | A page of one season's episodes, in the ordering the person watches the series in | ?cursor&limit → 200 Season | series.tsx |
| GET | `/files/{id}/waits` | getFileWaits | The work a file's viewer waits on, its read-through for its loudness and its timeline,… | → 200 FileWaits | file-waits.ts |
| GET | `/genres` | getGenres | The genres with a title the person reaches, each with how many | ?kind&library → 200 Genres | list-chips.tsx |
| GET | `/home` | getHome | The home screen: what the person stopped in the middle of | → 200 Home | continue-watching.tsx |
| DELETE | `/interest/{id}` | deleteInterest | Say this session left a film's or series' page, or a series' season's, which lets its… | ?season → 204 | page-interest.ts |
| PUT | `/interest/{id}` | putInterest | Say this session shows a film's or series' page, or a series' season's, which raises… | ?season → 204 | page-interest.ts |
| GET | `/items/{id}` | getTitle | A title's detail: versions, files, streams, extras and, for an episode, its place | → 200 ItemDetail | lab-cast.tsx, title.tsx |
| GET | `/libraries/{id}/folders` | getLibraryFolder | One folder of an other library: the folders in it, and its videos (D349) | ?folder&sort&order → 200 Folder | library-browse.tsx |
| GET | `/people/{id}` | getCreditedPerson | A person: their name, their picture and the titles crediting them | → 200 CreditedPerson | credits.tsx |
| GET | `/search` | search | Titles and series by name, in any script (D48); superseded by GET /titles?query= | ?query&library&limit → 200 Search | **none in web/** |
| GET | `/search/grouped` | searchGrouped | Titles, people and genres by what was typed, from its first letter, each group the… | ?query&group&cursor&limit → 200 GroupedSearch | **none in web/** |
| DELETE | `/tiles/interest` | deleteTilesInterest | Say this session shows no list's tiles, which lets their work fall back (D362, D324) | → 204 | tile-interest.ts |
| PUT | `/tiles/interest` | putTilesInterest | Say this session shows the tiles of these films, series and videos in view, which… | TilesInterest → 204 | tile-interest.ts |
| GET | `/titles` | getTitles | Films or series, by name in the library's locale or newest first | ?kind&sort&order&library&home&cursor&from&limit&query&genre&person&decade&unwatched → 200 Titles | alphabet-rail.tsx, browse.tsx, home.tsx, lab-cast.tsx, library-browse.tsx |
| GET | `/titles/letters` | getTitleLetters | The alphabet rail's letters a list by name or file name has titles under (D349) | ?kind&sort&library&query&genre&person&decade&unwatched → 200 Letters | alphabet-rail.tsx |

**Identification** (9)

| Method | Path | operationId | Purpose | Shape | Called from |
| --- | --- | --- | --- | --- | --- |
| GET | `/items` | getItemsByIdentification | The titles and series in one state of identification | ?identificationState → 200 Items | identify.tsx |
| GET | `/items/{id}/candidates` | getItemCandidates | An ambiguous film's or series' search results, to choose from, or those of a person's… | ?name&year → 200 Candidates | candidate-chooser.tsx |
| POST | `/items/{id}/confirm-match` | postItemConfirmMatch | Confirm a guessed match as it stands, "Identified correctly"; permanent… | → 204 | match-correction.tsx |
| POST | `/items/{id}/match` | postItemMatch | Choose a film's or series' match; permanent (edit_metadata) | Match → 204 | candidate-chooser.tsx |
| POST | `/items/{id}/match-by-link` | postItemMatchByLink | Choose a film's or series' match by its address or id at TMDB or IMDb; permanent… | MatchByLink → 204 | candidate-chooser.tsx |
| DELETE | `/items/{id}/overrides/{field}` | deleteOverride | Remove a field's override, so it follows its source again (edit_metadata) | → 204 | **none in web/** |
| PUT | `/items/{id}/overrides/{field}` | putOverride | Set a field to a person's value, kept over every source (edit_metadata) | PutOverride → 204 | **none in web/** |
| POST | `/items/{id}/refresh` | postItemRefresh | Fetch a film's or series' data and its missing images again, whenever a person likes… | → 202 Refresh | identify.tsx |
| POST | `/items/{id}/rescan` | postItemRescan | Rescan a film or series: scan its files, then match it again from them, unless a… | ?full → 202 RescanStarted | match-correction.tsx |

**Libraries** (17)

| Method | Path | operationId | Purpose | Shape | Called from |
| --- | --- | --- | --- | --- | --- |
| GET | `/files` | getFiles | What the scanner saw: every file with its filing and reason (admin; D163) | ?filing&libraryId&cursor&limit → 200 Files | files.tsx |
| POST | `/files/{id}/retire` | postFileRetire | Retire one file that cannot be read now: it becomes a tombstone, its history kept… | → 200 Retired | title.tsx |
| GET | `/libraries` | getLibraries | The libraries; an admin also sees their roots and files by filing | → 200 Libraries | libraries-reached.ts, libraries.tsx |
| POST | `/libraries` | postLibrary | Create a library over a node's roots, and scan it (admin) | CreateLibrary → 201 Created | create-library.tsx |
| POST | `/libraries/rescan` | postLibrariesRescan | Rescan every library of films or of series: scan it, then match each film and series… | ?full → 202 RescansStarted | libraries.tsx |
| DELETE | `/libraries/{id}` | deleteLibrary | Delete a library: its titles are hidden with their history, its work cancelled (admin;… | → 200 Retired | library-manage.tsx |
| PUT | `/libraries/{id}/change-detection` | putLibraryChangeDetection | Choose again how a library notices changes: watching, checking every so many hours, or… | ChangeDetection → 204 | library-manage.tsx |
| POST | `/libraries/{id}/mounts/gone` | postLibraryMountGone | Say a disk missing from a mount point in a root is gone; its files are removed (admin;… | RootAction → 202 Rescan | library-manage.tsx |
| POST | `/libraries/{id}/removals` | postLibraryRemovals | Approve a root's withheld removals; its next scan removes what it does not see (admin;… | RootAction → 202 Rescan | system-waiting.tsx |
| POST | `/libraries/{id}/rescan` | postLibraryRescan | Rescan a library of films or of series: scan it, then match each film and series in it… | ?full → 202 RescanStarted | library-manage.tsx |
| POST | `/libraries/{id}/roots` | postLibraryRoot | Add a folder to a library, and scan it (admin; D359) | AddLibraryRoot → 202 | library-manage.tsx |
| POST | `/libraries/{id}/roots/repoint` | postLibraryRootRepoint | Say a root moved: it and its files take the new path with their history, and a scan… | Repoint → 200 Repointed | library-manage.tsx |
| POST | `/libraries/{id}/roots/retire` | postLibraryRootRetire | Retire a root for good: its files become tombstones and their work is cancelled… | RootAction → 200 Retired | library-manage.tsx |
| POST | `/libraries/{id}/scan` | postLibraryScan | Ask a library's nodes to scan it (admin) | → 202 Scan | library-manage.tsx |
| GET | `/nodes` | getNodes | The nodes, and the roots each connected one configured, for a new library (admin) | → 200 Nodes | create-library.tsx, library-manage.tsx |
| POST | `/nodes/retire` | postNodeRetire | Retire a node for good: its roots are retired and its work is cancelled (admin; D231 §7) | RetireNode → 200 Retired | system.tsx |
| GET | `/nodes/{name}/folders` | getNodeFolders | The folders in a node's root or a folder inside it, to choose a library folder (admin;… | ?path → 200 NodeFolders | folder-picker.tsx |

**Playback** (26)

| Method | Path | operationId | Purpose | Shape | Called from |
| --- | --- | --- | --- | --- | --- |
| GET | `/bytes/media/chunks/{token}/master.m3u8` | getChunksMasterPlaylist | The master playlist of a playback's film cut into chunks at each quality (D247 §10) | → 200 video/mp4 | player (URL from POST /playback) |
| GET | `/bytes/media/chunks/{token}/{streamName}/{fileName}` | getChunkFile | One stream's playlist, or its init or a chunk, relayed from the film's node and made… | → 200 video/mp4 | player (hls.js) |
| GET | `/bytes/media/direct/{token}` | getDirect | A file's own bytes, relayed from its node, honouring Range | → 206 | player (<video src>) |
| GET | `/bytes/media/sub/{token}/{fileName}` | getSubtitle | One of a playback's subtitle tracks, as ASS or WebVTT (D217 §6) | → 200 text/x-ssa | player (subtitle tracks) |
| POST | `/client-paths` | reportClientPath | How a page chose its path to the core, and why, for the server's log | ClientPath → 204 | path-choice.ts |
| POST | `/client-problems` | reportClientProblem | A failure the page met, for the server's log | ClientProblem → 204 | failure-reports.ts, loader-at-client-origin.tsx |
| DELETE | `/collections/{id}/seasons/{season}/watched` | markSeasonUnwatched | Mark every episode of a season with a file unwatched by hand (D370) | → 200 WatchedMark | watched-mark.tsx |
| PUT | `/collections/{id}/seasons/{season}/watched` | markSeasonWatched | Mark every episode of a season with a file watched by hand (D370) | → 200 WatchedMark | watched-mark.tsx |
| DELETE | `/collections/{id}/watched` | markSeriesUnwatched | Mark every episode of a series with a file, in the ordering the person watches it in,… | → 200 WatchedMark | watched-mark.tsx |
| PUT | `/collections/{id}/watched` | markSeriesWatched | Mark every episode of a series with a file, in the ordering the person watches it in,… | → 200 WatchedMark | watched-mark.tsx |
| GET | `/files/{id}/availability` | getFileAvailability | Whether a file can be read now, and if not why: what a player whose fetch failed asks… | ?waitSeconds → 200 FileAvailability | playback-failure.ts |
| PUT | `/files/{id}/preferences` | setFilePreferences | Keep a person's choices in one file: its subtitle offset, its subtitle and audio… | SetFilePreferences → 204 | audio-tracks.tsx, subtitles.tsx |
| GET | `/files/{id}/subtitle-candidates` | getSubtitleCandidates | The subtitles OpenSubtitles can give for a file, each unverified, and the downloads… | ?language → 200 SubtitleCandidates | subtitle-search.tsx |
| POST | `/files/{id}/subtitles` | postFileSubtitle | Fetch a subtitle for a file; spends one of the account's daily downloads (D224) | FetchSubtitle → 200 FetchSubtitle | subtitle-search.tsx |
| GET | `/files/{id}/up-next` | getUpNext | What plays after a file as it ends, and what before it: the next and previous episodes… | → 200 UpNext | up-next.tsx |
| DELETE | `/items/{id}/hide` | unhideProgress | Undo hiding a work | → 204 | continue-watching.tsx |
| POST | `/items/{id}/hide` | hideProgress | Hide a work from Continue Watching until it next has activity; not a completion (D188) | → 204 | continue-watching.tsx |
| DELETE | `/items/{id}/watched` | markTitleUnwatched | Mark a film, episode or other video unwatched by hand: as never watched (D370) | → 200 WatchedMark | watched-mark.tsx |
| PUT | `/items/{id}/watched` | markTitleWatched | Mark a film, episode or other video watched by hand: it leaves Continue Watching and… | → 200 WatchedMark | watched-mark.tsx |
| GET | `/me/preferences` | getMyPreferences | A person's own choices, which hold in every file they play, as their settings show them | → 200 MyPreferences | autoplay-setting.tsx, library-configure.tsx |
| PUT | `/me/preferences` | setMyPreferences | Keep a person's own choices, which hold in every file they play: the boost, levelling,… | SetMyPreferences → 204 | autoplay-setting.tsx, debug-tools.tsx, library-configure.tsx, sound.tsx |
| POST | `/playback` | startPlayback | Decide how a file plays on this client, and where its bytes are (cut-a §6) | StartPlayback → 200 StartPlayback | watching.ts |
| DELETE | `/playback/{token}` | leavePlayback | Say the player left the film's page or the player: its work is no longer wanted (D258 §2) | ?reason → 204 | interest.ts |
| PUT | `/playback/{token}/interest` | sayInterest | Say which quality and sound the player wants, from where, and whether it is paused (D309) | Interest → 204 | interest.ts |
| POST | `/playback/{token}/report` | reportPlayback | Report where a playback is and what changed; the position never moves back (D217 §7) | Report → 200 Report | play-state.ts |
| PUT | `/watched` | restoreWatched | Put the person's watched marks back as a mark by hand's answer gave them: its Undo (D370) | RestoreWatched → 204 | watched-mark.tsx |

**Watch Together** (13)

| Method | Path | operationId | Purpose | Shape | Called from |
| --- | --- | --- | --- | --- | --- |
| POST | `/together` | openTogether | Start a Watch Together session, nothing playing, ending the one you host and leaving… | → 200 OpenTogether | watch-together.tsx |
| POST | `/together/join` | joinTogether | Join a session by your link, with the name the others see; no account needed (D246 §3) | JoinTogether → 200 JoinTogether | together-join.tsx |
| POST | `/together/join/check` | checkTogetherLink | Whether your link would admit you now, asked before its join's form; refused as the… | CheckTogetherLink → 204 | together-join.tsx |
| POST | `/together/leave` | leaveTogether | Leave the session you joined (a viewer) | → 204 | together-leave.tsx |
| GET | `/together/mine` | listMyTogether | The sessions you are in, as host or as a member who joined signed in, each with a… | → 200 MyTogether | together-mine.ts |
| POST | `/together/playback` | playTogether | A viewer's playback of what the session plays now (D246 §3) | ViewerPlayback → 200 StartPlayback | together-playback.ts |
| DELETE | `/together/{id}` | endTogether | End the session for everyone (host) | → 204 | watch-together.tsx |
| GET | `/together/{id}` | getTogether | The session's links and members, with your notes (host) | → 200 TogetherDetails | hosted-details.ts |
| POST | `/together/{id}/eject/{member}` | ejectFromTogether | Remove a member; their devices are told and their credential admits nothing (host; D119) | → 204 | together-people.tsx |
| GET | `/together/{id}/ice` | getTogetherICE | A member's voice relay credentials, which end with the membership (D243 §8) | → 200 ICE | together-voice.tsx |
| POST | `/together/{id}/links` | addTogetherLink | A link for one person to join by, once, with a note of who it is for (host; D246 §3) | TogetherLink → 201 TogetherLink | watch-together.tsx |
| DELETE | `/together/{id}/links/{link}` | removeTogetherLink | Withdraw a link not yet used (host) | → 204 | together-people.tsx |
| POST | `/together/{id}/members/{member}/links` | addTogetherLinkFor | A new link for a guest's place: they stay in until it is used, and whoever joins by it… | → 201 TogetherLink | together-people.tsx |

**Settings** (11)

| Method | Path | operationId | Purpose | Shape | Called from |
| --- | --- | --- | --- | --- | --- |
| GET | `/client-settings` | getClientSettings | The server settings every client reads: how often a screen asks again (D237), and more | → 200 ClientSettings | app.tsx |
| GET | `/config` | getConfig | Every server setting beside its starting value, and the config file's keys… | ?onlyModified → 200 Config | create-library.tsx, server-settings.tsx |
| PUT | `/config` | putConfig | Change any server settings, all or none (admin) | PutSettings → 200 Settings | server-settings.tsx |
| GET | `/config/unused` | getUnusedConfig | Setting rows this release does not read: retired keys, or typos (admin) | → 200 Unused | system-unused-settings.tsx |
| DELETE | `/config/unused/{key}` | deleteUnusedConfig | Delete a setting row this release does not read; a setting in use is set to default… | → 204 | system-unused-settings.tsx |
| POST | `/config/{key}/reset` | resetConfig | Set a server setting to default: its starting value, no longer marked modified, so it… | → 200 Setting | server-settings.tsx |
| GET | `/secrets` | getSecrets | Which secrets are set; never their values (admin) | → 200 Secrets | settings.tsx |
| DELETE | `/secrets/{name}` | deleteSecret | Clear a secret (admin) | → 204 | settings.tsx |
| PUT | `/secrets/{name}` | putSecret | Set a secret; write-only (admin) | PutSecret → 204 | settings.tsx |
| GET | `/settings` | getSettings | The important server settings, the settings page's normal view (admin) | → 200 Settings | **none in web/** |
| PUT | `/settings` | putSettings | Change important server settings, all or none (admin) | PutSettings → 200 Settings | **none in web/** |

**Jobs** (2)

| Method | Path | operationId | Purpose | Shape | Called from |
| --- | --- | --- | --- | --- | --- |
| GET | `/jobs` | listJobs | The job view: the newest jobs, what each works on, how far it is and why (admin) | → 200 Jobs | jobs.tsx |
| POST | `/jobs/{id}/cancel` | cancelJob | Cancel a queued or running job; a node's work is stopped (admin) | → 204 | jobs.tsx |

**Running it** (25)

| Method | Path | operationId | Purpose | Shape | Called from |
| --- | --- | --- | --- | --- | --- |
| GET | `/backups` | listBackups | List the backups directory, newest first (admin; D242 §5) | → 200 Backups | system-backups.tsx |
| POST | `/backups` | makeBackup | Back the persistent directory up now, into the backups directory, as a job the job… | MakeBackup → 202 MadeBackup | system-backups.tsx |
| GET | `/doctor` | getDoctor | What the running server finds wrong with itself, and what to do (admin) | → 200 Doctor | **none in web/** |
| GET | `/filing-reports` | listFilingReports | The reports of works filed wrongly, newest first (admin; D306 §3) | → 200 FilingReports | filing-report.tsx |
| GET | `/filing-reports/{name}` | getFilingReport | One report of a work filed wrongly (admin; D306 §3) | → 200 FilingReport | filing-report.tsx |
| GET | `/health` | getHealth | Whether the server is starting, migrating, ready or degraded | → 200 Health | newer-client.ts |
| POST | `/items/{id}/filing-report` | postFilingReport | Write a report of how a film, episode or series was filed, as Markdown on the server's… | → 201 FilingReport | filing-report.tsx |
| POST | `/lab-input-events` | reportLabInputEvents | What the button test heard, as it heard it, for the server's log (tv-device-tests.md H2) | LabInputEvents → 204 | lab-events.ts |
| POST | `/lab-results` | reportLabResult | What a lab page measured on a device, for the server's log (tv-device-tests.md) | LabResult → 200 LabResult | lab-send.ts |
| GET | `/lab-throughput` | getLabThroughput | The throughput test's body: bytes of no meaning, for a lab page to time its loads by… | ?bytes&leg → 200 application/octet-stream | lab-throughput.tsx |
| GET | `/needs-attention` | getNeedsAttention | How many folders and disks are offline, wait for the admin or could not be read… | → 200 NeedsAttention | needs-attention.tsx, status-tray.tsx |
| PUT | `/nodes/device-reader-limit` | putDeviceReaderLimit | Set how many may read one of a node's devices at once, or give it back to its kind's… | DeviceReaderLimit → 204 | system-work.tsx |
| GET | `/now-playing` | getNowPlaying | Every playback heard from lately: who, on which device, what, and how (admin) | → 200 NowPlaying | now-playing.tsx |
| POST | `/now-playing/{id}/stop` | stopPlayback | Stop a playback, with a message its player shows within one report interval (admin;… | StopPlayback → 204 | now-playing.tsx |
| GET | `/reachability/{nonce}` | echoReachability | Answer a reachability check's own request, which came back through the public name | → 204 | the core itself and orpheum-connect's probe |
| POST | `/rtc/offer` | postRTCOffer | Answer a WebRTC offer, after which the client sends its requests over data channels… | SessionDescription → 200 SessionDescription | transport-choice.ts |
| GET | `/system` | getSystem | Every fact about the server and each node, with doctor's findings: the System page… | → 200 System | system.tsx |
| DELETE | `/system/discovery` | leaveDiscovery | Take the core out of its discovery (admin; D273) | → 204 | system-discovery.tsx |
| GET | `/system/discovery` | getDiscovery | Where the core stands with its discovery (admin; D273) | → 200 DiscoveryStatus | system-discovery.tsx |
| POST | `/system/discovery` | connectDiscovery | Enrol the core with a discovery, which answers with the code its owner claims it by… | ConnectDiscovery → 200 DiscoveryStatus | system-discovery.tsx |
| POST | `/system/discovery/claim` | claimDiscovery | Enrol the core with its household's account by the key the account made (admin; D310 §5) | ClaimDiscovery → 200 DiscoveryStatus | system-discovery.tsx |
| POST | `/system/discovery/identify` | identifyDiscovery | What a discovery's issuer shows: its identity and name, to compare before enrolling… | IdentifyDiscovery → 200 IdentifiedDiscovery | system-discovery.tsx |
| POST | `/system/reachability` | checkReachability | Check the public name and voice relay from outside in, naming a failing hop (admin;… | → 200 Reachability | system-reachability.tsx |
| GET | `/system/resource-use` | getResourceUse | What the core and each node used of their machines, each minute, beside the plays and… | → 200 ResourceUse | system-resource-use.tsx |
| GET | `/system/update` | getSystemUpdate | What the server knows of an update: a security fix its version lacks, or a newer… | → 200 Update | update.ts |

## 2. What the API must carry later

| Planned | Where | What it asks of the API |
| --- | --- | --- |
| **Music** | D1 (video first, audio second); D186 (a shared `item`; generic orderings: a group is a season or a disc; artists as person entities; credits must not call their column `role`); [music-opensubsonic-2026-10-18.md](music-opensubsonic-2026-10-18.md); [closed-beta.md](../plans/closed-beta.md) (revisited when the closed beta ends) | Item kinds `album`, `artist`, `track`; a `music` library kind; group listings for discs; credits with an artist's role that is not `role`; pagination that holds at 500,000 tracks (D186 §5); progressive audio rather than HLS chunks; playlists that may repeat an entry; lyrics, ReplayGain, stars, play counts. OpenSubsonic would be a separate adapter (about 45 endpoints, app passwords per device), with our own API serving a lighter first-party view. |
| Cast, crew and ratings | [inventory-2026-10-27.md](../plans/inventory-2026-10-27.md) §12 | `/people/{id}` as the one person page; credits on every item kind. |
| Box sets and a person's own collections (C-11) | inventory §12; the `collection` table's `box_set` and `user` kinds | A collection is an item whose members are items; the API must not equate collection with series. |
| Watchlist (C-10), ratings, play statistics | inventory §12 | Per-user state on any item: the same `PUT/DELETE /items/{id}/<state>` shape as watched and hidden. |
| Metadata editor, NFO write | inventory §12 | Overrides on any item kind (already `/items/{id}/overrides/{field}`). |
| Parental controls, a rules engine, limits per role | inventory §12; D130 | Role capabilities named in one place, machine-readable per operation. |
| Sharing with another server; another server as a source | [sharing-model.md](../plans/sharing-model.md); D174 ("every item carries its source") | Item ids that stay opaque integers; a `source` field on items; a grantee that is not a household user. |
| Scoped API keys | inventory §12 (`grant.kind` `api_key` exists) | Capabilities per operation, so a key's scope is a list of them. |
| Offline downloads | D87; [offline-download.md](../plans/offline-download.md) | A rendition and a lease beside `/playback`, keyed by file. |
| A Jellyfin-compatible API, DLNA | D18 (a separate adapter) | Nothing in our own shape; adapters translate. |
| Store apps for TVs, native phone apps | inventory §12 | A stable, documented API; the pairing endpoints frozen (api-compatibility.md). |
| Several cores per account, a switcher (E-12) | D310 | Nothing per core in paths; the account is the paid service's (D289 §1). |
| Profiles with a PIN (E-6), TOTP or passkeys (E-7), OIDC/LDAP (maybe) | inventory §12 | Room under `/session` and `/me` for a second factor; users stay the noun. |
| Self-hosted discovery (E-10); the three operating modes | navigation-and-widgets-answers-2026-10-27.md | The loader endpoints served the same in every mode. |
| Change journal and delta sync | D8 | A cursor that is opaque and the same everywhere. |
| A metrics endpoint, plugins (D20), a second media node | inventory §12 | Room at the top level (`/metrics`), nodes addressed by name. |
| Not wanted for now: the compatibility gate, live TV, photos | closed-beta.md; D1 | — |

## 3. The problems found

### 3.1 Three words for a user, and the word "person" taken twice

- The path is `/users`. The schemas are `AccountBody`, `AccountsOutputBody`, `CreateAccount…`,
  `PersonOutputBody`, `RenamePersonInputBody`, `PersonsRoleInputBody`, `UserBody`. The operations
  are `listAccounts`, `createAccount`, `getPerson`, `renamePerson`, `removePerson`, `setPersonsRole`,
  `listPersonDevices`, `removePersonDevice`. `RoleBody.holderCount` is described as "people";
  `QuotaBody.reservedForPeople`; `NowPlayingBody` has `personName` beside `userId`.
- **"Person" also means a credited person**: `CreditBody.personId`, `CreditedPersonOutputBody`,
  `PersonTitleBody`, `PersonRoleBody`, and the `person` item kind (D186). Music will add artists to
  the same table.
- The web client's pages follow the confusion: `/people` and `/people/{userId}` are the admin's
  page of users, so the credited person had to go to `/credited/{id}` (`router.ts` says why).
- "Account" is also the paid service's word: orpheum-connect has `/accounts`, and D289 §1 says only
  the paying owner has one; household users never do.

### 3.2 Titles, items and collections cross over

- `GET /titles` lists films **and series**, but the `title` table is the playable leaf and a series
  is a `collection`. Its entry kinds are `movie`, `series`, `other`.
- `GET /items/{id}` is `getTitle` and answers only for titles; a series is `GET /collections/{id}`
  (`getSeries`). Yet `/items/{id}/match`, `/refresh`, `/hide` and the overrides take a series' id,
  while a series' watched mark is `/collections/{id}/watched` and a film's is `/items/{id}/watched`.
- `GET /items` is a different list again, the identification list, with its own entry shape
  (`ItemBody`, kinds `movie`, `episode`, `extra`, `series`) beside `/titles`' `EntryBody`.
- "Collection" in the path means series alone; box sets and a person's own collections (C-11) will
  need the word with its real meaning.

### 3.3 Seasons are video-only

`/collections/{id}/seasons/{season}` addresses a season by its number. D186 §2 already made the
orderings generic (`position.group_number`: a season or a disc). The path should say group.

### 3.4 Kind words, parameters and pagination differ

- Library kinds are `movies`, `shows`, `other` (plural, and "shows"); item kinds are `movie`,
  `series` (singular, and "series"). The database has `mixed`, which the API does not offer. Search
  hits add `episode` and `extra`.
- The library is `library` in `/titles` and `/search` but `libraryId` in `/files`.
- The last page's `nextCursor` is "empty" in `/titles` and "absent" in seasons.
- Lists that grow without bound carry no cursor: a credited person's titles, `/genres`, `/users`,
  `/jobs`, `/filing-reports`, `/invitations`. At music's scale (D186 §5) a prolific artist's page
  and the genre list are the first to break.

### 3.5 Actions are addressed three ways

- `POST /files/{id}/retire` names its file in the path; `POST /nodes/retire` names its node in the
  body; `PUT /nodes/device-reader-limit` the node and device in the body.
- Root actions: `/libraries/{id}/roots/repoint` and `/roots/retire` beside `/mounts/gone` and
  `/removals`.
- `POST /together/{id}/eject/{member}` beside `POST /together/{id}/members/{member}/links`.
- Hiding is `POST`/`DELETE /items/{id}/hide`; watched is `PUT`/`DELETE /items/{id}/watched`.
  Both are a per-user state that is on or off.
- One flow, two nouns: the device asks at `/pairings`, the person answers at `/activations`.
- The lab's routes sit at the top level as `/lab-input-events`, `/lab-results`, `/lab-throughput`.

### 3.6 "Interest" means two things in three places

`PUT /interest/{id}?season=` (one item), `PUT /tiles/interest` (several, by `itemIds`) and
`PUT /playback/{token}/interest`. The first two say "I may play this soon, prepare it"; the third is
the player saying which quality and sound it wants (D309). Two endpoints for one job (rule 11), and
one word for two.

### 3.7 Superseded and uncalled routes stay served

`GET /search` is marked superseded by library browsing (D349) and has no caller; `GET
/search/grouped` is the one being built into the client now. `GET /settings` and `PUT /settings`
have no caller: the settings page uses `/config`. `GET /doctor` has no caller. `POST /me/locator`
and the overrides have no caller in `web/` yet (they are for devices and the metadata editor, and
stay).

### 3.8 Two views of the settings

`/settings` (the important ones) and `/config` (every one, with its starting value) are two shapes of
the same rows. The page uses `/config`.

### 3.9 Video words where the thing is generic

`CastFilm`, `LadderBody.filmPictureKbps`, `filmOfExtra`, `isFilmsOwnPicture`, `filmsWithoutSound`;
`titleId` and `titleName` in casts, now playing and the socket's messages. Most are about a video
file and may stay; the ones naming what is played (`CastFilm`, `titleId`/`titleName` in casts and
now playing) should say item.

### 3.10 The socket

- Its path is `/together/ws` but it carries the presence channel too, opened by a hello of type
  `live` instead of `hello`.
- `changeItem` carries a `fileId`. That is right (a group plays a file), but the name says item.

### 3.11 Who may call what is prose

56 of 169 operations say "(admin)" in their summary; others are for an account manager or a
metadata editor (`requireAccountManager`, `requireMetadataEditor`). Nothing in the document says so
in a form a client, a test or a scoped API key can read. The document declares no security scheme:
`Authorization` is a header parameter on 155 operations.

### 3.12 What is already right

The error body (`{"error":{code,message,detail,failedHop}}`, cut-a §5.2) is one shape everywhere.
Ids are opaque integers. The bearer token never goes in a URL (D154); the byte paths take the media
cookie (D216). `Idempotency-Key` (D244). The `/me` family is consistent. Handlers are named for
their operations (`TestArch_APIHandlersAreNamedForTheirOperations`). The node protocol is separate
and versioned. None of this changes.

## 4. The routes, old → new

The prefix stays `/api/v1` (Q5). Only the operations that change, and the ones held frozen, are
listed: 29 move, 10 keep their path and change their operationId, 4 go, 126 stay as they are
(9 of those are frozen, listed with why). "Why" points to §3 and to the questions in §6.

Beside the paths, these change in bodies and queries:

- **Kinds, one vocabulary.** Libraries and items share singular words: `movie`, `series`,
  `episode`, `extra`, `other`, and later `album`, `artist`, `track`, with `music` for a library.
  A library's kind becomes `movie`, `series`, `mixed` or `other` (`mixed` already exists in the
  database). `GET /items?kind=` takes any of them.
- **One list body.** `GET /items` answers `{items: ItemEntry[], nextCursor?}`; an entry carries
  `id`, `kind`, `name`, art, and the fields its kind has (a discriminated union on `kind`).
  `identification` becomes one more filter and one more field.
- **One item body.** `GET /items/{id}` answers a union on `kind`: today's title detail for
  `movie`, `episode`, `extra`, `other`; today's series body for `series`; later `album` and `artist`.
- **One pagination.** Every list that can grow takes `cursor` and `limit` and answers `nextCursor`,
  **absent** on the last page: `/items`, `/items/{id}/groups/{group}`, `/people/{id}` (its items),
  `/search`, `/genres`, `/files`, `/users`, `/invitations`, `/jobs`, `/filing-reports`.
- **Users.** Schemas `Account*` and `Person*` for a user become `User*`; `personName` →
  `userName`; `RoleBody.holderCount` is described as users; `QuotaBody.reservedForPeople` →
  `reservedForUsers`.
- **Credited persons.** `CreditedPersonOutputBody` → `PersonBody`; its `titles` → `items`, paged.
  A credit's job is `job` (`cast`, `director`, `writer`, later `artist`, `composer`…), not `role`
  (D186 §2).
- **Casts and now playing.** `CastFilm` → `CastItem`; `titleId`/`titleName` → `itemId`/`itemName`
  in casts, now playing and the presence messages.
- **Capabilities.** Each operation carries `x-orpheum-capability` (`signedIn`, `admin`,
  `manageUsers`, `editMetadata`, `deviceTests`, `none`), and the document declares the bearer
  scheme. A test asserts each handler's `require…` call matches its declared capability.
- **The socket.** `/together/ws` → `/socket`; the presence channel's hello becomes
  `{type:"hello", channel:"live"}`, Watch Together's `{type:"hello", channel:"together"}`. The
  message types do not change.

| Old | operationId | New | New operationId | Why |
| --- | --- | --- | --- | --- |
| `POST /activations/answer` | answerActivation | `POST /pairings/check` | answerPairingCheck | §3.5; Q10 |
| `POST /activations` | activateDevice | `POST /pairings/code` | enterPairingCode | §3.5; Q10 |
| `GET /client-manifest` | getClientManifest | same |  | frozen: the discovery's loader calls it (D289 §4); Q10 |
| `POST /client-problems` | reportClientProblem | same |  | frozen: the loader; Q10 |
| `GET /collections/{id}/not-in-order` | getNotInOrdering | `GET /items/{id}/not-in-order` | listNotInOrder | §3.2 |
| `PUT /collections/{id}/preferences` | setSeriesPreferences | `PUT /items/{id}/preferences` | setItemPreferences | §3.2 |
| `DELETE /collections/{id}/seasons/{season}/watched` | markSeasonUnwatched | `DELETE /items/{id}/groups/{group}/watched` | unmarkGroupWatched | §3.3 |
| `PUT /collections/{id}/seasons/{season}/watched` | markSeasonWatched | `PUT /items/{id}/groups/{group}/watched` | markGroupWatched | §3.3 |
| `GET /collections/{id}/seasons/{season}` | getSeason | `GET /items/{id}/groups/{group}` | getGroup | season or disc; §3.3; Q3 |
| `DELETE /collections/{id}/watched` | markSeriesUnwatched | `DELETE /items/{id}/watched` | unmarkWatched | §3.2 |
| `PUT /collections/{id}/watched` | markSeriesWatched | `PUT /items/{id}/watched` | markWatched | one handler for every kind; §3.2 |
| `GET /collections/{id}` | getSeries | `GET /items/{id}` | getItem | folded; §3.2 |
| `GET /doctor` | getDoctor | — |  | removed unless the owner wants it kept for operators; Q9 |
| `GET /files` | getFiles | same |  | query `libraryId` → `library`; §3.4 |
| `DELETE /interest/{id}` | deleteInterest | `DELETE /interest` | withdrawInterest | §3.6 |
| `PUT /interest/{id}` | putInterest | `PUT /interest` | sayInterest | body `{items:[{id, group?}]}`; §3.6 |
| `DELETE /items/{id}/hide` | unhideProgress | `DELETE /items/{id}/hidden` | unhideItem | §3.5 |
| `POST /items/{id}/hide` | hideProgress | `PUT /items/{id}/hidden` | hideItem | state is PUT/DELETE, like watched; §3.5 |
| `GET /items/{id}` | getTitle | same | getItem | every kind, series too; §3.2 |
| `GET /items` | getItemsByIdentification | `GET /items?identification=…` | listItems | folded into the one list; §3.2 |
| `POST /lab-input-events` | reportLabInputEvents | `POST /lab/input-events` |  | §3.5 |
| `POST /lab-results` | reportLabResult | `POST /lab/results` |  | §3.5 |
| `GET /lab-throughput` | getLabThroughput | `GET /lab/throughput` |  | §3.5 |
| `POST /libraries/{id}/mounts/gone` | postLibraryMountGone | `POST /libraries/{id}/roots/mount-gone` |  | every root action under /roots; §3.5 |
| `POST /libraries/{id}/removals` | postLibraryRemovals | `POST /libraries/{id}/roots/removals` |  | §3.5 |
| `POST /me/casts/{id}/play` | playInCast | same |  | body `CastFilm` → `CastItem`; §3.9 |
| `PUT /nodes/device-reader-limit` | putDeviceReaderLimit | `PUT /nodes/{name}/reader-limits` | setReaderLimit | `deviceKey` stays in the body; §3.5 |
| `POST /nodes/retire` | postNodeRetire | `POST /nodes/{name}/retire` |  | the node in the path, as /files/{id}/retire has its file; §3.5 |
| `GET /now-playing` | getNowPlaying | same |  | `personName` → `userName`, `titleId`/`name` → `itemId`/`itemName`; §3.1 |
| `POST /pairings/session` | takePairedSession | same |  | frozen core of api-compatibility.md |
| `POST /pairings/state` | getPairingState | same |  | frozen core of api-compatibility.md |
| `POST /pairings` | requestPairing | same |  | frozen core of api-compatibility.md |
| `GET /people/{id}` | getCreditedPerson | same | getPerson | the name users gave back; its items paginate; §3.1, §3.4; Q4 |
| `PUT /playback/{token}/interest` | sayInterest | `PUT /playback/{token}` | setPlaybackWants | what the player wants is the playback's state; frees the word; §3.6 |
| `GET /reachability/{nonce}` | echoReachability | same |  | frozen: orpheum-connect's probe; Q10 |
| `POST /rtc/offer` | postRTCOffer | same |  | frozen: the loader; Q10 |
| `GET /search/grouped` | searchGrouped | `GET /search` | search | the grouped one is the search; §3.7 |
| `GET /search` | search | — |  | removed: superseded by library browsing (D349); Q9 |
| `POST /session/handoff` | handOffSession | same |  | frozen: the loader; Q10 |
| `POST /session/redeem` | redeemSession | same |  | frozen: the loader; Q10 |
| `GET /settings` | getSettings | — |  | removed: no caller; /config is the one view; §3.8; Q9 |
| `PUT /settings` | putSettings | — |  | removed; §3.8; Q9 |
| `DELETE /tiles/interest` | deleteTilesInterest | `DELETE /interest` | withdrawInterest | folded; §3.6 |
| `PUT /tiles/interest` | putTilesInterest | `PUT /interest` | sayInterest | folded; §3.6 |
| `GET /titles/letters` | getTitleLetters | `GET /items/letters` | listItemLetters | §3.2 |
| `GET /titles` | getTitles | `GET /items` | listItems | §3.2; Q2 |
| `POST /together/{id}/eject/{member}` | ejectFromTogether | `POST /together/{id}/members/{member}/eject` |  | member actions under /members; §3.5 |
| `DELETE /users/{id}/devices/{deviceId}` | removePersonDevice | same | removeUserDevice | §3.1 |
| `GET /users/{id}/devices` | listPersonDevices | same | listUserDevices | §3.1 |
| `PUT /users/{id}/role` | setPersonsRole | same | setUserRole | §3.1 |
| `DELETE /users/{id}` | removePerson | same | removeUser | §3.1 |
| `GET /users/{id}` | getPerson | same | getUser | §3.1 |
| `PUT /users/{id}` | renamePerson | same | renameUser | §3.1 |
| `GET /users` | listAccounts | same | listUsers | §3.1 |
| `POST /users` | createAccount | same | createUser | §3.1 |

## 5. What the move costs

Counts are files that name the path as a string (Go in `internal` and `cmd`; TypeScript in
`web/apps` and `web/packages`, the generated SDK excluded), with tests in brackets.

| Path family | Go files | Web files |
| --- | --- | --- |
| `/titles` | 13 (10) | 34 (27; includes the page routes) |
| `/items` | 19 (13) | 9 (1) |
| `/collections` | 9 (6) | 3 (1) |
| `/users` | 12 (8) | 2 (0) |
| `/people/` | 2 (1) | 5 (3) |
| `/interest`, `/tiles/interest` | 4 (3) | 3 (2) |
| `/search` | 9 (6) | 0 |
| `/libraries` | 17 (13) | 14 (7) |
| `/together` | 7 (4) | 14 (6) |
| `/config` | 7 (6) | 3 (0) |
| `/genres` | 2 (1) | 1 (0) |

**Go (`internal/core/api` and its tests).** Each handler is the method its operationId names
(`TestArch_APIHandlersAreNamedForTheirOperations`), so the ten renamed operations and the folded
ones rename methods: `getTitles` and the identification list into `listItems`; `getTitle` and
`getSeries` into `getItem`; the series and title watched handlers into one; the two interest
handlers into one. The store's queries (`internal/store/queries/*.sql`) mostly stay; the item list
needs one query that unions what `/titles` and the identification list read today. The kind
vocabulary changes a `CHECK` on `library.kind`, a schema change under rule 8 (folded into the
baseline while D228's window is open). The presence and Watch Together hellos change in
`internal/core/live` and `internal/core/together`. The capability extension needs a small change
where operations are registered and one new `TestArch_` test.

**Generated.** `api/openapi.json` and `web/packages/sdk/src/gen/api.ts`, by `make generate`. No hand
edits.

**SDK.** `together.ts` and `live-connection.ts` (the socket path and hellos), `client.ts` (`artUrl`
stays), `rtc-together.ts` if it builds the path.

**Web.** `accounts.ts`, `roles.ts`, `people.tsx` and `person.tsx` (users; the page files become
`users.tsx` and `user.tsx`), `credits.tsx` (the credited person at `/people/{id}`), `series.tsx`,
`watched-mark.tsx`, `title.tsx`, `identify.tsx`, `candidate-chooser.tsx`, `match-correction.tsx`,
`browse.tsx`, `home.tsx`, `alphabet-rail.tsx`, `library-browse.tsx`, `list-chips.tsx`,
`continue-watching.tsx`, `interest.ts`, `play-state.ts`, `watching.ts`, `cast.ts`,
`library-manage.tsx`, the Watch Together files (`watch-together.tsx`, `together-*.tsx`,
`together-mine.ts`, `together-playback.ts`, `hosted-details.ts`), `use-together.ts`, `live.tsx`,
`router.ts` (`/people` → `/users`, `/credited/{id}` → `/people/{id}`), `app-shell.tsx` (the menu's
"People" → "Users"), and the tests' fixtures that spell the paths (the bulk of the 27 `/titles`
test hits). The cast receiver makes no API call and does not change.

**Docs.** [cut-a.md](../plans/cut-a.md) §5 (the client API map, §5.1's messages), the package
`doc.go` files that name routes, and the 24 docs that name a moving route in backticks (102
mentions) by the dated-note rule of the decision conventions.

**Outside this repository.** orpheum-connect calls `/api/v1/reachability/{nonce}` and serves the
loader, which calls `/session/handoff`, `/session/redeem`, `/client-manifest`, `/client-problems`
and `/rtc/offer`. This proposal moves none of them, so connect's pin needs no change (Q10).

**Size.** Of the order of the D186 reshape's API half: one batch, done as three commits that each
pass the gate: (1) users and credited persons, (2) items, groups, interest and search, (3) the rest
(actions, lab, settings, socket, capabilities). Done before music and before any other server is
shared with, it costs this once; done after, every one of them is a deprecation (api-compatibility.md).

## 6. Questions for the owner

1. **User or account for the people of a household?** The paid service already uses account for
   the one who pays (D289 §1), and household users never have one. *Recommend **user**: path,
   schemas, operations and the page; "account" stays the paid service's.*
2. **One noun for everything browsable: items, or titles?** `title` is the playable leaf in the
   schema; a series, an album and an artist are not titles. *Recommend **items**: `GET /items`,
   `GET /items/{id}` for every kind, with the kind as a discriminator.*
3. **Seasons, or generic groups?** *Recommend **groups** (`/items/{id}/groups/{n}`), as D186 §2
   made the orderings; the client still says "Season 2" and later "Disc 2".*
4. **Artists under `/people`, or their own `/artists`?** D186 makes artists person entities.
   *Recommend **`/people`**, with an album's credits naming the artist's job; one person can be both
   an actor and a singer.*
5. **Rename in place under `/api/v1`, or open `/api/v2`?** Nothing is deployed to others and the core
   serves its own client. *Recommend **in place**: no second version to serve, and D289 §4's "one
   client build per API major" keeps its meaning for when it counts.*
6. **The web client's page addresses and menu label.** *Recommend pages `/users` and `/users/{id}`
   for the admin's, `/people/{id}` for a credited person (dropping `/credited/`), and the menu
   label "Users".*
7. **Action subresources, or pure REST?** `/retire`, `/rescan`, `/eject` are verbs. *Recommend
   **keep actions** where the thing is an act, but always on the addressed resource
   (`/nodes/{name}/retire`), and state that is on or off as `PUT`/`DELETE` of a noun (`/hidden`,
   `/watched`).*
8. **Who may call an operation: a declared capability, or an `/admin` prefix?** A prefix splits
   resources by caller (a user's own devices and an admin's view of them would live apart) and
   cannot express "account manager" or "metadata editor". *Recommend **a declared capability per
   operation**, tested against the handler, which scoped API keys can later reuse.*
9. **Remove the uncalled routes now?** `GET /search` (superseded, D349), `GET`/`PUT /settings`,
   `GET /doctor`. *Recommend **yes** for the first two; keep `/doctor` only if the owner wants it as
   an operator's command (`orpheum doctor` could call it), otherwise remove it too.*
10. **Touch the loader's endpoints and the pairing core?** They are called by the discovery's loader
    and by devices, and api-compatibility.md freezes pairing. *Recommend **leave them** as they are
    (and so connect's pin), but move the person's side of pairing from `/activations` to
    `/pairings/code` and `/pairings/check`, which no device calls.*
11. **When does the compatibility gate start?** The owner marked it not wanted for now.
    *Recommend: after this rename lands, at the start of the open beta, so the renamed API is the
    first one held.*

## 7. Draft decision text

> **D<n> — The API is renamed once, before anyone else depends on it: users, items, groups, people;
> one list shape; capabilities declared per operation**
>
> **Decided** (2026-11-??), at the owner's request: *"For the API endpoints: let's Greenfield
> them. Nothing is deployed to other people so this is the cheapest time for invasive changes. We
> can use account or user where we use people now. Look at our whole API and make sure it will
> handle the future things we have planned (for example, music)."* Research:
> `design/research/api-greenfield-2026-11-04.md`.
>
> 1. **A household member is a user** in paths, schemas, operations, fields and pages. "Account" is
>    the paid service's word (D289 §1). "Person" is a credited person only.
> 2. **Everything browsable is an item.** `GET /items` is the one list, filtered by kind, library,
>    genre, person, decade, unwatched and identification state; `GET /items/{id}` answers every
>    kind as a union on `kind`. Per-user state on an item is `PUT`/`DELETE /items/{id}/<state>`
>    (`watched`, `hidden`, later `starred`, `watchlisted`). `/titles` and `/collections` as paths
>    are gone.
> 3. **Orderings are groups** (D186 §2): `/items/{id}/groups/{group}` is a season or a disc.
> 4. **Credited persons are `/people/{id}`**, artists among them when music comes. A credit's job is
>    `job`, never `role` (D186 §2).
> 5. **One kind vocabulary**, singular, shared by libraries and items: `movie`, `series`, `episode`,
>    `extra`, `other`, `mixed` for a library, and later `music`, `album`, `artist`, `track`.
> 6. **One list shape**: `cursor` and `limit` in, `nextCursor` out, absent on the last page, on every
>    list that can grow. One parameter name per filter (`library`, not `libraryId`).
> 7. **An action is on the resource it acts on** (`/nodes/{name}/retire`,
>    `/together/{id}/members/{member}/eject`); a state that is on or off is a noun put and deleted.
>    One endpoint per job: interest is `PUT`/`DELETE /interest` for one item or many; the player's
>    wants are `PUT /playback/{token}`.
> 8. **Who may call an operation is declared in the document** as `x-orpheum-capability`, and a
>    `TestArch_` test holds each handler to its declaration. No `/admin` prefix.
> 9. **The rename is in place under `/api/v1`.** The loader's endpoints (`/session/handoff`,
>    `/session/redeem`, `/client-manifest`, `/client-problems`, `/rtc/offer`,
>    `/reachability/{nonce}`) and the device's side of pairing do not move.
> 10. **Removed:** `GET /search` (superseded, D349; `/search` is the grouped search),
>     `GET`/`PUT /settings` (`/config` is the one view), and `GET /doctor` unless kept as answered.
> 11. **The socket is `/socket`**, and each channel opens with `{type:"hello", channel}`.
>
> **Rejected:** a `/v2` beside `/v1` (two APIs to serve, for no client that needs the old one);
> an `/admin` prefix (splits one resource by caller, and cannot say account manager or metadata
> editor); mimicking Jellyfin's or OpenSubsonic's shapes (they are adapters, D18).
>
> **Amends:** cut-a.md §5's client API map and §5.1's hello, which take the new names;
> D186 §2's credit column, which this names `job`.
>
> **Status:** Proposed. Not built.
