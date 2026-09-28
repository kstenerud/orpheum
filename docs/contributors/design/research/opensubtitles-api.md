> **ABOUTME:** What OpenSubtitles' REST API (opensubtitles.com) allows and requires of an
> application that fetches subtitles on request: whose key, whose account, the quotas, the rate
> limits, the search and the provider hash. Feeds D224, and the questions it left.

# The OpenSubtitles API, read for fetching on request (2026-09-28)

Gathered 2026-09-28 because the owner's registration of Orpheum as an API consumer showed the
warning *"This is a product designed for applications developpers, THIS IS NOT A VIP PACKAGE. If
you wish to use an application such as bazarr or jellyfin, it is a VIP package you need to
purchase"*. Sources are the API documentation on Stoplight
(`https://opensubtitles.stoplight.io/docs/opensubtitles-api/<slug>`; each claim names its page),
the OpenSubtitles forum, and the source of three published clients read locally: the Jellyfin
plugin, Bazarr and OpenSubtitles' own Kodi add-on. opensubtitles.com's own pages (the consumer
page, the terms) answer 403 behind Cloudflare; the terms were read from the Wayback Machine's copy
of 2025-11-24.

## 1. The key is the application's, and it ships with it

- *"Using one key per application is **mandatory** if you develop an application requiring users
  to create their own API key we will ban their access"*, and *"don't allow users to insert their
  API key (with exception of wrappers)"* (getting-started).
- An installed application carries its key: *"any app installed must contain it's own API key"*
  (the forum's administrator, 2026, threads 7998 and 7977). No page asks that it be kept secret.
  Jellyfin's plugin (`OpenSubtitlesPlugin.cs`), Bazarr (`bazarr/app/get_providers.py`) and the
  Kodi add-on (`resources/settings.xml`) each carry theirs in their source.
- What the documentation forbids is the developer's own login in the application: *"it is wrong
  idea that users will download subtitles under your account"* (getting-started).
- Every request sends `User-Agent: <AppName> v<X.Y>`, since 2023-09-08, or is refused with 403
  (changelog, best-practices). The name is the application's choice. Measured 2026-09-28: a search
  with Orpheum's key and `User-Agent: Orpheum/dev`, the form every Orpheum request carries,
  answered 200 with ten German subtitles for The Matrix.

**The warning** is addressed to end users who register a consumer thinking a program like Bazarr
needs one. A developer's key is what lets its users log in with their own accounts, free or VIP.
Jellyfin's plugin and Bazarr ship a key and ask the user only for a login, and neither requires
VIP. VIP for Bazarr belonged to the old opensubtitles.org API, which is being shut down.

## 2. The quota is the account's

| Who | Downloads a day |
| --- | --- |
| no login, the application's key alone | 5 per IP address |
| a consumer marked "under development", no login | 100, while it is marked so |
| a logged-in free account | 20 (the rank "Sub leecher"; ranks earned by uploading raise it to 300) |
| a VIP account | 1000 |

- A download counts when `/download` is called, not when the file is fetched; the link it
  returns lasts at most three hours (download).
- `/download` answers `link`, `file_name`, `requests`, `remaining`, `message`, `reset_time` and
  `reset_time_utc`; a spent quota is 406 with `remaining: -1` (error-codes). `/infos/user` answers
  `allowed_downloads`, `level`, `vip`, `downloads_count` and `remaining_downloads`.
- The documentation says counters reset at midnight UTC, but its own examples show
  `reset_time_utc` at other hours: a rolling day. **Show `reset_time_utc` as the provider gives
  it.**
- The price table marks *"Ads in Subtitles"* for the free plan: a free account's subtitles may
  carry lines of advertising.

## 3. Logging in is rationed

- Login is limited to 1 a second, 10 a minute and 30 an hour; after a 401 the same credentials
  must not be retried (login). The JWT it returns lasts 24 hours; a request with an expired one
  answers 500 "invalid", and the answer is to log in again.
- Login returns `base_url`: `api.opensubtitles.com`, or `vip-api.opensubtitles.com` for a VIP
  account, and every later request goes there (login).
- Every request is at most 5 a second per IP (best-practices), with `Accept: */*`, `Api-Key`,
  `User-Agent`, and `Authorization: Bearer <JWT>` once logged in, which `/download` and
  `/infos/user` require.
- Follow redirects; to avoid them, send query parameters sorted, lower-case, without defaults, an
  IMDb id without `tt` or leading zeros. Retry a 5xx after a second (best-practices).

**For us:** the server keeps the admin's password sealed (D194), logs in when it has no valid
token, keeps the token for its day and shares it among the household's requests, and stops after
a 401 until the admin changes the login.

## 4. Search and the provider hash

- `GET /subtitles` takes `imdb_id`, `tmdb_id`, `parent_imdb_id`/`parent_tmdb_id` with
  `season_number` and `episode_number`, `moviehash`, `languages` (comma-separated, sorted),
  `query`, `year`, `type`, `moviehash_match`, `hearing_impaired`, `foreign_parts_only`,
  `ai_translated`, `machine_translated` and `page` (search). With a `moviehash`, each result says
  `moviehash_match`, and the matches come first.
- `moviehash` is the file's size plus the sum of the first and the last 64 KiB read as
  little-endian 64-bit words, truncated to 64 bits, written as 16 hex digits; only for files over
  131,072 bytes. Test values: `breakdance.avi` (12,909,756 bytes) is `8e245d9679d31e12` (hash).
  The published implementations are GPL unless marked otherwise; ours is written from this
  description.

## 5. Our use

Nothing read forbids a household using one admin's account through a self-hosted server, or
fetching on request; the forum's administrator calls a personal media player *"what the API is
for"* (thread 7991). The terms page is the old opensubtitles.org disclaimer and prohibits
commercial use; it says nothing about the API.

## What this changes

- **D224 §2** said an admin may set a key of their own: the documentation forbids letting users
  enter a key. The key is the application's only.
- The key goes in the source, as every published client's does: the owner decides it
  (2026-09-28, asked).
- The count of downloads left and the time it resets are the provider's (D224 §4 already).
- A free account's subtitles may carry advertising lines: the owner decides what we do with them
  (asked).
