> **ABOUTME:** Language, script and locale handling — what the competitors built, where it
> breaks, and the measurements that decide our version. Covers UI strings, collation, search
> tokenisation, multi-language metadata, subtitle text rendering, and the Unicode traps in Go
> and in our own node pipeline.

# Internationalisation

Researched **2026-09-19**. Sources: local clones of Jellyfin, jellyfin-web, Kyoo, Navidrome,
Immich, Streamyfin, Swiftfin; the libass and JASSUB repositories; and **original measurement**
in Go 1.26.2 and SQLite 3.46.1, which is flagged as such throughout.

The starting point was the observation in
[plans/internationalisation.md](../plans/internationalisation.md): the most popular third-party
metadata providers in this field are Chinese-market (MetaShark, Bangumi, Douban, Danmu), and
together they outweigh every official Jellyfin provider combined. The people most motivated to
extend a media server are not English-speaking by default. Everything below follows from taking
that seriously.

## 0. Summary of findings

| # | Finding | Basis |
| --- | --- | --- |
| 1 | Weblate is the field's convention — 9 of 11 projects surveyed | repo inspection |
| 2 | `react-i18next` is what both React Native media clients converged on | Kyoo, Streamyfin |
| 3 | Jellyfin's sort order is a **materialised ASCII transliteration**, English stop-words hard-coded, tuned by an environment variable | source |
| 4 | Jellyfin translates strings in **two** catalogues that overlap by 12 keys | measured |
| 5 | Jellyfin's metadata language is one value per library, resolved at **ingest**; the fallback chain request has been open since 2020 | source + issues |
| 6 | Kyoo has the right metadata model: language-invariant table + per-language translation table, selected by `Accept-Language` at query time | source |
| 7 | No single FTS5 tokeniser covers our case; running two costs ~1 MB per 20k titles | **measured** |
| 8 | `remove_diacritics 2` does not fold ø æ œ ß — in either tokeniser | **measured** |
| 9 | `x/text/collate` is correct for Latin/CJK, **wrong for Korean**, and its tables are from CLDR 23 (2013) | **measured** |
| 10 | CLDR sort keys are byte-comparable, ~140 B/title — so SQLite needs no custom collation | **measured** |
| 11 | Go's `\w \d \s \b` are ASCII-only: porting Jellyfin's naming regexes breaks *Amélie*, not just 進撃の巨人 | **measured** |
| 12 | NFC/NFD: a macOS node and a Linux node report **different bytes for the same file** | **measured** |
| 13 | libass defaults to VSFilter-bug-compatible bidi; JASSUB never turns it off and ships no libunibreak | source |
| 14 | Hermes has no `Intl.PluralRules` and no `Intl.Segmenter`, and its `Intl` results vary by Android version | Hermes docs |
| 15 | The Android CDD mandates **no** CJK, Arabic, Hebrew, Thai or Indic font — a compliant device may render tofu | **measured** (0 occurrences in the CDD) |
| 16 | Bundling a CJK font costs ~16 MB per weight, so fonts should be served, not shipped | **measured** |
| 17 | Netflix built a three-region microservice to serve UI strings — to avoid App Store review on a typo | Netflix, 2015 |
| 18 | Netflix runs **pseudo-localisation by default** in every debug build; text expands up to 40% | Netflix, 2018 |
| 19 | Netflix mandates IMSC 1.1 for Japanese subtitles; **libass has zero ruby support** | partner specs + libass source |
| 20 | Netflix largely sidesteps collation — its browse surface is ranked rows, not an A–Z grid. Ours is a grid | reasoning |
| 21 | No single font can cover Unicode: OpenType caps at 65,535 glyphs; Unifont's BMP file holds 58,911 | **measured** |
| 22 | Netflix Sans covers Latin, Cyrillic, Greek, Vietnamese — **no CJK, Arabic, Hebrew, Thai, Indic**. Netflix renders tofu too | Dalton Maag |
| 23 | Android's fallback chain has **no last-resort entry**; Apple has shipped one since 1998 | **measured** (AOSP `fonts.xml`) |
| 24 | Unicode Last Resort HE maps all 1,114,112 code points in **588 KB**, and FreeType supports its cmap format 13 | **measured** |
| 25 | CJK fallback must be chosen by **language**, not script — Han unification | Apple API + Levien |
| 26 | CTA-708 already enumerates the subtitle style space: **eight tags, six files** once `smcp` and variable axes are used | spec + **measured** |
| 27 | Seven CTA-708 styles for Latin+Greek+Cyrillic cost **5.63 MB raw / 2.42 MB woff2** | **measured** |
| 28 | A film-length Japanese subtitle uses **1,202 distinct code points**; subsetting Noto CJK to them gives **441 KB** instead of 15.7 MB | **measured** |
| 29 | libass synthesises bold and italic, so italic faces are optional | libass source |
| 30 | **Two FreeType CVEs are in CISA's exploited-in-the-wild catalogue**, one from March 2025 | NVD + CISA KEV |
| 31 | Netflix's subtitle spec permits **one** generic font family and no named or embedded fonts | Netflix partner spec |
| 32 | Jellyfin loads embedded fonts by default and its *trusted* fallback set by opt-in — backwards | jellyfin-web source |
| 33 | Dropping the TrueType hinting VM and bitmap tables makes the subset **14% smaller** | **measured** |

---

## 1. UI translation: what everyone actually uses

### The platform

| Project | Platform | Catalogue format | Languages |
| --- | --- | --- | --- |
| Jellyfin (server) | Weblate | flat JSON | 105 |
| jellyfin-web | Weblate | flat JSON | 108 |
| Immich | Weblate | flat JSON | 89 |
| Kyoo | Weblate | JSON | — |
| Swiftfin | Weblate | `.lproj` / `.strings` | 54 |
| Audiobookshelf | Weblate | JSON | — |
| PhotoPrism | Weblate | — | — |
| Stash | Weblate | JSON | — |
| Kavita | Weblate | JSON | — |
| Streamyfin | **Crowdin** | JSON | 34 |
| Navidrome | **POEditor** | JSON | 36 |

**Weblate is the convention, 9 of 11.** It is AGPL, self-hostable, and free-of-charge for libre
projects on `hosted.weblate.org`. The two dissenters are both proprietary SaaS with free OSS
tiers. There is no technical argument visible in the code for either dissent; the convention is
strong enough to follow without further analysis.

### The library

Both React Native clients in the survey independently chose the same stack:

```
Kyoo        react-i18next 17 + i18next-browser-languagedetector + i18next-http-backend
Streamyfin  react-i18next 17 + i18next 26 + expo-localization
```

That is convergent evidence from the only two projects solving our exact client problem.
Immich's web client uses `svelte-i18n` + `intl-messageformat` and its Flutter app uses
`easy_localization` — different frameworks, so not counter-evidence.

Two configuration details are worth stealing outright, both from Kyoo:

```ts
returnEmptyString: false,   // an untranslated key exports as "" — fall back, don't render blank
load: "currentOnly",        // don't also fetch the base language for "en-US"
```

`returnEmptyString: false` matters because translation platforms export empty strings for
untranslated keys. The default (`true`) renders them, so a partially-translated language shows
**blank UI** rather than English.

### Bundle vs fetch

Streamyfin statically imports all 34 catalogues into the app bundle. Kyoo carries
`i18next-http-backend` and can fetch them. For a TV and mobile client this is a real decision,
and it is not about bytes:

- **Bundled** translations can only be updated by shipping a new build. On tvOS and Android TV
  that means an app-store review cycle. This is the mechanism by which translations go stale.
- **Fetched from our own server** means a Weblate merge reaches every client the next time it
  connects, over the LAN, for free.

We are in the unusual position of *being* the server the client is already talking to. Fetching
should be the default, with the English catalogue bundled as the offline floor.

### Jellyfin's cautionary tale: two catalogues

Jellyfin translates strings in two separate places:

```
jellyfin-web/src/strings/en-us.json                              1,947 keys
jellyfin/Emby.Server.Implementations/Localization/Core/en-US.json  123 keys
overlapping keys: 12  (Artists, Books, Collections, Default, Favorites, Folders,
                       Genres, HeaderContinueWatching, Movies, MusicVideos, Photos, Shows)
```

Twelve concepts translated twice, by two sets of translators, in two Weblate components,
displayed in the same UI. "Continue Watching" is one of them.

Worse is *why* the server catalogue exists at all. Looking at what is in it:

- `AuthenticationSucceededWithUserName`, `FailedLoginAttemptWithUserName` — activity-log and
  notification text, delivered by email or push to someone who is not looking at a client.
- `HeaderContinueWatching`, `Favorites`, `Collections` — names of **virtual folders**, which
  become real strings in DLNA and in the API.
- `ChapterNameValue` — `"Chapter {0}"`, used to **generate a name that is then stored in the
  database**.

That last one is the actual defect, and it generalises into a rule:

> **Never store a translated string in the database.** Store the fact and the parameters;
> render at display time. A chapter is `{index: 3}`, not `"Chapter 3"`, because
> `"Chapter 3"` is frozen in whatever language the scanner happened to be running in.

Once that rule holds, the server-side catalogue shrinks to the genuinely server-side cases —
notifications delivered outside a client session — and those should be rendered in the
*recipient's* language, which the server knows, not the server's.

### Server-side formatting in Go, if we need it

`golang.org/x/text/message` + `catalog` gives a plural-aware `Printf` with CLDR plural rules.
Measured binary cost on top of a hello-world (Go 1.26.2, linux/amd64, x/text v0.42.0):

```
baseline                     2,396,870 B
+ x/text/language              +328 KB
+ x/text/message               +594 KB
+ x/text/collate             +1,835 KB
```

`x/text/language` is needed regardless (§4). `message` is cheap. `collate` is the expensive one
and is discussed in §5.

---

## 2. Multi-language metadata: the data model

This is the part that is expensive to retrofit, and the field splits cleanly in two.

### Jellyfin: one language, chosen at ingest

`PreferredMetadataLanguage` resolves item → parent chain → library → server
(`BaseItem.GetPreferredMetadataLanguage()`), and is used at **fetch** time to decide which title
to write into the database. There is exactly one title per item. Consequences:

- Two people in one household cannot have different languages. Requests for this: *Per user
  system language*, *Multiple metadata languages*, *Metadata in multiple languages*, *force user
  specific metadata language*, *Metadata in English, not in selected language* (#5236, open),
  *Jellyfin ignores language settings for metadata* (#15932, open).
- There is **no fallback chain** — only en-US. *EF Core Migration — Language Selection Fallback
  Heirarchy* (#2521) has been open since **2020-03-05** with five reactions and no owner.
- Artwork inherits the same problem from the other side: *Separate Language Settings for Titles
  and Posters* (#10076). Posters have title text baked into the pixels, so poster language is a
  separate axis from title language.

Individually these are low-reaction issues. That is the trap: i18n complaints are spread thin
across many small language communities, so they never win a reaction contest and never get
prioritised — while collectively they describe a structural gap.

### Kyoo: language-invariant core + translation table

```
shows                          show_translations
  pk, id, slug                   pk, language        ← PRIMARY KEY (pk, language)
  kind, status                   name
  genres  (ENUM ARRAY)           description, tagline
  rating, runtime                aliases[], tags[]
  startAir, endAir               poster, thumbnail, banner, logo
  original (JSONB)               trailerUrl
```

Three details are the good ideas:

1. **Artwork lives in the translation row.** Poster, thumbnail, banner and logo are per-language,
   which is exactly the axis Jellyfin #10076 is asking for.
2. **`genres` is an enum, `tags` is free text.** Genres are a closed set of 23 values, stored as
   symbols and rendered from the *UI* catalogue — so a Chinese user gets 科幻 from our own
   translation file rather than whatever string TMDB returned. Tags cannot be enumerated, so they
   stay per-language. This is a cheap, sharp distinction and we should copy it wholesale.
3. **`original` is a distinguished record**, not just another translation. The original-language
   title always exists, which gives both a guaranteed fallback anchor and a meaningful
   "sort by original title" option.

Selection happens at query time, per request, driven by the HTTP `Accept-Language` header:

```sql
SELECT DISTINCT ON (t.pk) …
FROM show_translations t
WHERE t.language = ANY($langs)          -- omitted when fallback is allowed
ORDER BY t.pk,
         array_position($langs, t.language),
         t.language                     -- "ensure a stable sort to prevent future pages
                                        --  to contains the same element again"
```

Two lessons beyond the obvious:

- **`Accept-Language` already is the fallback chain** that Jellyfin #2521 has wanted for six
  years. It is an ordered, weighted preference list with a `*` wildcard, it is standard, and
  every HTTP client already sends something reasonable.
- **Language selection participates in `ORDER BY`, so it must be a total order.** Kyoo's comment
  says why: a non-deterministic tie-break makes keyset pagination return the same row on two
  different pages.

One caution, because it shows how fiddly this is. Kyoo's `processLanguages` parses the `q=`
weights and then sorts **ascending**:

```ts
.sort(([_, q1], [__, q2]) => q1 - q2)
```

Combined with `array_position` — which prefers the *earliest* entry — `Accept-Language:
en-us, ja;q=0.5` appears to select Japanese. There is no test covering it. This is an argument
for using a library rather than hand-parsing: Go's `language.ParseAcceptLanguage` +
`language.NewMatcher` implement the full BCP 47 lookup including likely-subtags and mutual
intelligibility (nb/no, zh-Hans/zh-Hant, pt-BR/pt), which is a lot of behaviour to reproduce by
hand and get right.

### `DISTINCT ON` under SQLite

`DISTINCT ON` is Postgres-only. The SQLite equivalent is a window function, available since
3.25:

```sql
SELECT … FROM (
  SELECT t.*, ROW_NUMBER() OVER (
    PARTITION BY t.item_id
    ORDER BY lang_rank.rank, t.language) AS rn
  FROM item_translations t
  JOIN lang_rank ON lang_rank.language = t.language
) WHERE rn = 1
```

with `lang_rank` a temp table or CTE built from the request's language list.

---

## 3. Sorting and collation

### What Jellyfin does

`BaseItem.GetSortName()` materialises a sort string into the database at scan time:

1. lowercase;
2. strip configured stop-words (`SortRemoveWords`, default `["the", "a", "an"]`) from the start,
   the middle and the end;
3. strip `SortRemoveCharacters` (`, & - { } '`), replace `SortReplaceCharacters` (`. + %`);
4. zero-pad digit runs to 10 characters, so `Episode 2` sorts before `Episode 10`;
5. `RemoveDiacritics()`;
6. if anything non-ASCII survives, **transliterate the whole string to ASCII**.

The transliterator is ICU4N with this default rule chain:

```
Any-Latin; Latin-Ascii; Lower; NFD; [:Nonspacing Mark:] Remove; [:Punctuation:] Remove;
```

overridable by the environment variable `JELLYFIN_TRANSLITERATOR_ID`. The dependency is pinned
at `ICU4N.Transliterator 60.1.0-alpha.356`.

Four things are wrong with this, and they are instructive because each is a separate mistake:

- **The stop-words are English and server-wide.** A French library gets "Le", "La", "Les" left
  in; and because the rule also strips from the *middle* and the *end*, a Spanish title
  containing " a " loses it.
- **`Any-Latin` on Han characters produces pinyin**, which is correct for Chinese and wrong for
  Japanese: 千と千尋の神隠し does not sort under S for *Sen*.
- **Transliteration destroys the distinctions the target locale cares about.** Swedish Å Ä Ö
  collapse to A A O and sort at the front, when Swedish puts them after Z.
- **An environment variable to tune the global transliteration rule** is the tell. It exists
  because there is one sort order for the whole server and somebody needed a different one.

### What `x/text/collate` actually does — measured

CLDR collation is the correct tool. Go's implementation is `golang.org/x/text/collate`. I ran it
rather than trusting the docs (Go 1.26.2, x/text v0.42.0):

```
Swedish   collate:  Anka | Zebra | Åke | Ärlig | Öland          ← correct, Å Ä Ö after Z
          codepoint: Anka | Zebra | Ärlig | Åke | Öland         ← wrong twice over

de-u-co-phonebk  Mahler | Mueller | Müller | Muller             ← correct: ü ≡ ue
de (default)     Mahler | Mueller | Muller | Müller             ← correct: ü after u

zh (pinyin)      陈七 | 李四 | 王五 | 张三 | 赵六                  ← chen li wang zhang zhao ✓
zh-u-co-stroke   王五 | 张三 | 李四 | 陈七 | 赵六                  ← by stroke count ✓
ja               あさひ | さくら | サクラ | ざくろ | 亜              ← kana equivalence ✓

mixed library, en:
  collate:   Wing | Ångström | Éire | Ørsted | über | Zarvo | Броненосец Потёмкин | 十字
  codepoint: Wing | Zarvo | Ångström | Éire | Ørsted | über | Броненосец Потёмкин | 十字
```

The mixed-library line is the whole argument in one row: naive ordering dumps every accented
title into a ghetto after Z.

`collate.Numeric` handles the episode-number case natively:

```
collate.Numeric: Episode 1 | Episode 2 | Episode 10
default        : Episode 1 | Episode 10 | Episode 2
```

which deletes Jellyfin's hand-rolled `ModifySortChunks` zero-padding entirely.

**Where it fails.** Korean:

```
ko  collate:   Apple | Zebra | 가나다 | 나비 | 하늘
    codepoint: Apple | Zebra | 가나다 | 나비 | 하늘     ← identical
```

CLDR's Korean tailoring reorders Hangul ahead of Latin. `x/text/collate` does not do it; the
source says so plainly — `// TODO: implement script reordering` — and
[golang/go#19087](https://github.com/golang/go/issues/19087) has been open since **2017-02-14**
with **zero comments**. Nobody is working on it.

**And the tables are old.** `collate/tables.go` declares:

```go
const UnicodeVersion = "6.2.0"   // 2012
const CLDRVersion    = "23"      // 2013
```

The rest of `x/text` is actively maintained (commits within the last month) and tracks current
Unicode; `collate` alone is frozen at CLDR 23. It covers 90 locales including
`zh-u-co-stroke`, `zh-Hant-u-co-pinyin`, `de-u-co-phonebk`, `ja`, `ko`, `th`.

Honest verdict: **correct for Latin, Cyrillic, Greek, Chinese and Japanese; wrong for Korean;
thirteen years behind on data.** For a household media server that is a good trade, but it
should be written down, because the alternative — cgo-linked ICU — costs cross-compilation,
which [D31](../../decisions/working-notes.md) went out of its way to buy.

### Sort keys make this a storage question, not a query question — measured

`collate.Collator.KeyFromString` produces a **byte-comparable** key: `bytes.Compare` on two keys
gives the same answer as the collator. That means the key can live in a `BLOB` column, be
indexed, and be ordered with SQLite's default `BINARY` collation. Measured:

```
n=10,000 titles
  collate.SortStrings (query time)   16.3 ms
  key generation (once)              10.2 ms      ~1 µs/title
  sort the keys (byte compare)        2.1 ms
  key size                           ~140 B/title (multi-word);  19–29 B (single word)
```

So both options are affordable at our scale, and they differ in kind rather than in cost:

- **Sort in Go at query time** — 16 ms for 10k, but it requires materialising the whole
  candidate set in memory, which forfeits `ORDER BY … LIMIT 50` and index-driven paging.
- **Materialise a key per (item, locale)** — ~140 B per item per locale (1.4 MB per 10k items),
  and SQLite does `ORDER BY sort_key LIMIT 50` against an index, O(log n).

The second wins, and it is structurally the same thing Jellyfin does — a materialised sort
column — with correct CLDR data instead of ASCII transliteration, and **one row per locale in
use** instead of one server-wide answer. A household has one to three.

### Why not a custom SQLite collation

Both pure-Go drivers support it (`modernc.org/sqlite.RegisterCollationUtf8`,
`ncruces/go-sqlite3` `Conn.CreateCollation`), so this is a choice, not a limitation. Two reasons
against:

1. **An index is bound to the collation it was created with.** `ORDER BY title COLLATE sv` can
   only use an index declared `COLLATE sv`. So you need one index per locale either way — which
   is precisely the materialised-key design, with extra steps.
2. **It makes the database file non-portable.** Open it with the `sqlite3` CLI, or any recovery
   tool, and every query touching that collation fails with *no such collation sequence*. A
   `BLOB` column of sort keys degrades to "an opaque column" instead.

[D13](../../decisions/working-notes.md)'s "the database is a rebuildable index" stance argues
the same way: prefer the form that any tool can still read.

---

## 4. Search across scripts

### Navidrome is the prior art, on our exact stack

Navidrome is Go + SQLite + FTS5, and it has been through this. The shape it arrived at:

```go
func getSearchStrategy(tableName, query string) searchStrategy {
    if conf.Server.Search.Backend == "legacy" || conf.Server.Search.FullString {
        return newLegacySearch(tableName, query)
    }
    if containsCJK(query) {            // Han | Hiragana | Katakana | Hangul
        return newLikeSearch(tableName, query)
    }
    return newFTSSearch(tableName, query)
}
```

with tables declared:

```sql
CREATE VIRTUAL TABLE media_file_fts USING fts5(
    title, album, artist, album_artist,
    sort_title, …, search_normalized,
    content='', content_rowid='rowid',
    tokenize='unicode61 remove_diacritics 2'
)
```

Details worth carrying over verbatim:

- **`content=''`** — a contentless FTS5 table, populated by triggers, storing only the index.
  Navidrome also ships `VerifyFTS` (`integrity-check`) and `RebuildFTS`, because FTS5 index
  corruption is a real operational event and the index is derivable.
- **FTS5's boolean operators are case-sensitive.** `AND OR NOT NEAR` are operators; `and or not
  near` are plain tokens. Navidrome neutralises user input by lowercasing them. Otherwise
  searching for the band *Chairlift NEAR Sea* throws a syntax error.
- **No minimum query length for CJK** — their comment: *"single CJK characters are meaningful
  words."* Their 2-character minimum applies to Latin only.
- **`(token OR token*)`** rather than bare `token*`: adds no matches, but gives bm25 a
  high-IDF exact hit so literal matches outrank prefix matches.

### The Ø bug, and why it is not a Navidrome bug

Navidrome shipped FTS5 in migration `20260220173400_add_fts5_search`. On **2026-07-02** they
shipped another:

> The FTS5 migration back-filled `artist.search_normalized` with a SQL approximation that
> cannot transliterate atomic letters (Ø, æ, ß, …), and the scanner never rewrote the column,
> leaving artists like "GØGGS" unfindable by ASCII searches.

Two distinct lessons in one migration. The first is about Unicode; the second is about us.

**On Unicode.** I reproduced it from scratch in SQLite 3.46.1:

```
tokenizer: unicode61 remove_diacritics 2
  "Amelie"  → finds "Amélie"       ✓   é is NFD-decomposable
  "Bjork"   → finds nothing        ✗   Bjørk
  "Oeuvre"  → nothing              ✗   Œuvre
  "Strasse" → nothing              ✗   Straße
  "GOGGS"   → nothing              ✗   GØGGS

indexed tokens, via fts5vocab: [amelie, bjørk, complete, gøggs, …, straße, œuvre]
```

`remove_diacritics` removes **combining marks**. Ø, Æ, Œ, ß, Ð, Ł are single code points with
no decomposition, so nothing is removed. The same is true with `trigram remove_diacritics 1`:
`GOGG` still misses `GØGGS`. This is not a tuning problem; no tokeniser option fixes it.

**On us.** The bug was caused by writing the normalisation logic twice — once in Go for the
scanner, once as a SQL `REPLACE` chain for the migration backfill — and the two drifted. The
rule: **normalisation has exactly one implementation.** A migration that needs it calls the Go
function row by row (as their fix does) rather than approximating it in SQL.

### CJK and Thai are worse than expected — measured

```
tokens indexed by unicode61 for whole titles:
  千と千尋の神隠し  →  ["千と千尋の神隠し"]        one token, the entire title
  花样年华        →  ["花样年华"]              one token
  기생충          →  ["기생충"]                one token
  ผู้หญิงลั้ลลา     →  ["ผ", "หญ", "งล", "ลลา"]   garbage fragments
```

So `千尋` finds nothing, and Thai is actively broken — unicode61 splits it at combining marks
into fragments that match no real word. Korean "works" only by accident: the whole title
happened to be one token.

The trigram tokeniser behaves oppositely:

```
tokenizer: trigram
  "神隠し" (3) → 千と千尋の神隠し   ✓
  "千尋"  (2) → nothing           ✗   trigram needs ≥3 characters
  "기생충" (3) → 기생충            ✓
  "หญิง"      → ผู้หญิงลั้ลลา        ✓   Thai works
  "øgg"       → GØGGS            ✓   substring, so atomic letters match themselves
  "Bjor"      → nothing          ✗   no folding without remove_diacritics
```

Summarised:

| | Latin + diacritics | ø æ œ ß | CJK | Thai | prefix / bm25 |
| --- | --- | --- | --- | --- | --- |
| `unicode61 remove_diacritics 2` | ✓ | ✗ | ✗ | ✗ | ✓ |
| `trigram` | ✗ | substring only | ✓ (≥3 chars) | ✓ | ✗ |
| `trigram remove_diacritics 1` | ✓ | ✗ | ✓ (≥3) | ✓ | ✗ |

**No single tokeniser covers our case.** So measure the cost of running two — 20,000 titles,
mixed Latin/CJK, contentless FTS5:

```
content table                      532 KB
unicode61 remove_diacritics 2      476 KB index   (0.9× content)   build 28 ms
trigram                          1,004 KB index   (1.9× content)   build 30 ms
```

Both indexes for 20k titles: **1.5 MB.** That settles it — this is not a trade-off, it is a
rounding error.

Query latency, same corpus, `LIMIT 50`:

```
unicode61  "matrix"   0.013 ms      unicode61  "mat*"     0.103 ms
trigram    "神隠し"    0.012 ms      trigram    "霸王别姬"   0.014 ms
LIKE %神隠し% (dense)  0.079 ms      LIKE %ゴジラ% (no match, full scan)  1.554 ms
trigram    "ゴジラ" (no match)                               0.001 ms
```

Navidrome's LIKE fallback is fine at household scale — 1.5 ms worst case at 20k rows — but
trigram is three orders of magnitude better on the miss, which is the common case while
someone is typing. The rule that falls out:

- **1–2 characters** → `LIKE '%…%'` (below trigram's floor, and single CJK characters are
  meaningful)
- **≥3 characters, any script** → trigram index
- **Latin word queries** → unicode61 index, for prefix matching and bm25 ranking
- plus a **normalised-alternates column** (Bjørk→Bjork, R.E.M.→REM, Straße→Strasse) generated in
  Go by the single normalisation function and fed into the unicode61 index

---

## 5. Subtitles: libass defaults to being wrong

This is our subsystem ([D11](../../decisions/working-notes.md)) and it has a specific trap.

Jellyfin's *Right-to-left languages subtitles having punctuation in the wrong order*
(jellyfin-web #4179) blames the browser: *"current web browsers rendering the every VTT file in
the same way and ignoring right to left."* True, and an argument for our decision to render
everything with libass rather than native WebVTT. But libass has the same failure available, on
by default.

`ASS_Feature` in `libass/ass.h` — three of the four values are i18n, and **all are disabled by
default**:

- **`ASS_FEATURE_WHOLE_TEXT_LAYOUT`** — when disabled, *"text is split into VSFilter-compatible
  segments and text in each segment is processed in isolation… The individual runs are then
  laid out **left-to-right, even if they contain right-to-left text**."* That is literally the
  #4179 bug, reproduced inside libass for ASS subtitles.
- **`ASS_FEATURE_BIDI_BRACKETS`** — bracket pairing per the revised UBA (Unicode 6.3). Returns
  −1 at runtime if libass was built against an old FriBidi.
- **`ASS_FEATURE_WRAP_UNICODE`** — line breaking per UAX #14, *"if the track language is set,
  some additional language-specific tweaks may be applied."* Unavailable unless libass was built
  with **libunibreak**. Without it there is no correct line breaking for Thai, Japanese or
  Chinese, which have no spaces to break at.

The defaults are bug-compatibility with a 2000s Windows filter. That is the right default for
libass — fansubbed ASS is authored against VSFilter's behaviour — and the wrong default for us
on anything that is not authored ASS: SRT, VTT, PGS-converted text, and CC.

**JASSUB, which is our web path, makes this concrete.** Its submodules are
`libass, fribidi, freetype, harfbuzz, brotli` — **no libunibreak** — and it never calls
`ass_track_set_feature`. So the browser client today would get VSFilter-segmented bidi, pre-6.3
bracket pairing, and no Unicode line breaking. Getting this right means patching JASSUB's build
to add libunibreak and exposing the feature toggles — a real, previously-uncosted line item for
D11.

Three consequences for our design:

1. Build libass with **HarfBuzz** (Arabic shaping breaks without it — HandBrake shipped that
   way), a **current FriBidi**, and **libunibreak**.
2. Enable `WHOLE_TEXT_LAYOUT` and `BIDI_BRACKETS` for non-ASS sources; leave them off for
   authored `.ass`/`.ssa`, where VSFilter compatibility is the point.
3. **Pass the track language to libass.** `WRAP_UNICODE` uses it, and we have it already —
   [D24](../../decisions/working-notes.md) normalises track language to BCP 47. This is the
   payoff for that work.

`ass_track_set_feature` returns −1 when a feature is unavailable at runtime. That is a capability
probe: check it once, and report the answer, rather than silently rendering wrong.

---

## 6. Client-side traps

### Hermes has no plural rules and no segmenter

React Native has had `Intl` since 0.70, but Hermes does not bundle ICU — it projects the
platform's. Per Hermes' own `IntlAPIs.md`:

- Android is backed by **ICU4j, whose version varies by Android release**, so *"results of
  running the same code may vary between devices running different Android versions."*
- `Intl.NumberFormat` lacks `notation`, `compactDisplay` and `signDisplay` on iOS;
  `Intl.DateTimeFormat` lacks `dayPeriod` and `fractionalSecondDigits` on Android.
- **`Intl.PluralRules` and `Intl.Segmenter` are absent** from the supported list on both
  platforms.

Three things follow:

- **Plural handling must not depend on `Intl.PluralRules`.** i18next's `compatibilityJSON`
  setting exists for exactly this; Streamyfin pins `"v4"` explicitly. Either pin it or ship
  `@formatjs/intl-pluralrules`.
- **No client-side word segmentation** for CJK. Which is fine, because §4 puts search on the
  server.
- **Client-side collation is non-deterministic across devices.** `localeCompare` on an Android 10
  phone and an Android 14 TV can order the same list differently. This is an independent argument
  for §3's conclusion: **the server owns sort order**, the client renders the list it is given.

### You cannot assume a font exists for the script

Measured against the **Android 15 Compatibility Definition Document**, which is the document
that decides what a certified Android device must contain. Searching it:

```
"CJK"       0 occurrences
"Chinese"   0 occurrences
"Arabic"    0 occurrences
"Hebrew"    0 occurrences
"Thai"      0 occurrences
```

What it *does* mandate is Latin, Greek and Cyrillic coverage including Latin Extended A–D and
the Unicode 7.0 currency block; that `NotoColorEmoji.ttf` must not be removed; and that the
`sans-serif` family be Roboto 2.x **"for the languages that Roboto supports."**

So a device can ship with no CJK, Arabic, Hebrew, Thai or Indic font at all and still be a
compliant Android device. On a cheap TV box built for one market — which is exactly the hardware
in [test-hardware.md](test-hardware.md) — that is not hypothetical. The symptom is tofu boxes in
the UI and, worse, in subtitles, where the user's reaction is "the subtitles are broken", not
"my TV lacks a font".

"Just bundle a font" is not free, and this is the measurement that says so:

```
NotoSansCJKjp-Regular.otf       15.70 MB   (one weight, Japanese-first pan-CJK)
NotoSansCJKjp-Bold.otf          16.24 MB
NotoSerifCJK full OTC           ~132 MB    (release asset, zipped)
```

Roughly 16 MB per weight per CJK variant. Bundling that into a TV app for every user, most of
whom will never see a CJK glyph, is the wrong shape.

The right shape falls out of the architecture we already have: **the server serves fonts on
demand.** [D11](../../decisions/working-notes.md) already serves embedded subtitle font
attachments as addressable resources, so script-fallback fonts are the same mechanism with a
different key — the client reports which scripts it can already render, the server supplies what
is missing, once, cached, over the LAN. Being the server the client is already talking to keeps
paying.

### RTL requires an app restart

`I18nManager.forceRTL()` sets a native flag read at startup. Changing it does not re-lay-out the
running app — react-native #16215, #32509, #47583, and #49451 (still reproducing on RN 0.77).
The common workaround is `react-native-restart`.

On a TV that is a worse experience than on a phone. Two defensible stances: follow the OS
locale's direction only and never offer an in-app direction switch, or offer it with an explicit
"this will restart the app" confirmation. The first is simpler and matches how people actually
set up a TV.

Jellyfin has the layout bugs to match (jellyfin-web #7297, Arabic series detail page), which is
what happens when RTL is not in the test matrix. If we support Arabic and Hebrew, one RTL locale
belongs in the device-lab pass.

---

## 7. Two Unicode traps in our own pipeline

These are not from the competitor survey. They are hazards in what we have already designed, and
both were verified by running them.

### Go's regex character classes are ASCII-only

Jellyfin's filename parsing uses `[\w\s]+?` for the series name. In .NET, `\w` is Unicode-aware
by default. **In Go's RE2 it is not.** Porting the expression verbatim:

```
Breaking Bad - 01.mkv    \w: "Breaking Bad" ep=01    \p{L}: "Breaking Bad" ep=01
進撃の巨人 - 01.mkv        \w: NO MATCH                \p{L}: "進撃の巨人" ep=01
기생충 - 01.mkv            \w: NO MATCH                \p{L}: "기생충" ep=01
Матрица - 01.mkv          \w: NO MATCH                \p{L}: "Матрица" ep=01
Amélie - 01.mkv           \w: NO MATCH                \p{L}: "Amélie" ep=01
```

It is not only a CJK problem. **`Amélie` fails.** And:

```
\w+           on 進撃の巨人 → ""   ;  on ٤٢ → ""
\d+           on ٤٢ (Arabic-Indic digits) → ""
[[:alpha:]]+  on 進撃の巨人 → ""
\p{L}+        on 進撃の巨人 → "進撃の巨人"
```

`\s` has the same problem and it bites harder than it looks, because the separator in CJK
filenames is frequently U+3000 IDEOGRAPHIC SPACE, and Windows renamers emit U+00A0:

```
"進撃の巨人　-　01.mkv"    \s+: false    [\p{Z}\s]+: true
"Film Title.mkv"    \s+: false    [\p{Z}\s]+: true
```

It fails **silently** — the expression simply does not match and the filename falls through to a
worse one, producing a misparse rather than an error. The remedy is mechanical: ban `\w \d \s \b
[[:alpha:]]` in the naming-regex corpus, require `\p{L} \p{N} \p{M} \p{Z}`, and enforce it with a
test that runs the whole corpus against a fixture of non-Latin filenames. Cheap now, invisible
later.

### NFC vs NFD across media nodes

[D33](../../decisions/working-notes.md) has media nodes on arbitrary machines reporting paths to
a core. macOS/APFS hands back decomposed filenames; Linux hands back whatever bytes are on disk,
usually composed; SMB and NFS each add their own behaviour.

```
macOS-style NFD : "Amélie"  len=8  bytes= 41 6d 65 cc 81 6c 69 65
Linux-style NFC : "Amélie"  len=7  bytes= 41 6d c3 a9 6c 69 65
string equal?          false
equal after NFC(both)? true
```

Same file. Different bytes, different length, different hash, `!=` in Go. A macOS node and a
Linux node reporting the same SMB share therefore report **two different paths for one file**.
Under a `UNIQUE(path)` constraint that is two library items; under a reconciling scanner it is a
delete-and-add churn cycle every time the reporting node changes.

The fix is a boundary rule, which fits the *parse, don't validate* stance already in the
principles: **normalise to NFC at the core's ingest boundary, and keep the raw bytes for I/O.**
Both halves matter. Identity, dedup, display and search use the NFC form; opening the file uses
the original bytes, because APFS normalises on lookup but ext4 does not — an NFC string will not
open an NFD file on Linux.

This belongs in the node report contract ([D39](../../decisions/working-notes.md)) as an explicit
field pair, not as an assumption.

---

## 8. How the commercial players do it

Asked directly, and worth its own section because the answer is not "the same thing, bigger".
Netflix has published more engineering detail on this than anyone else in the field; the
primary sources are the 2015 *Localization Technologies at Netflix* post, the 2018
*Pseudo Localization @ Netflix* post, and their public partner-facing delivery specifications.

### The strings live in a service, not in the repository

Netflix's **Global String Repository** is a microservice — Java, Cassandra and Elasticsearch,
deployed across three AWS regions, exposed through the same edge APIs as their other metadata
and held to the same availability requirements, with a Java client that caches. Strings are
organised into **bundles** (a container across languages) and **namespaces** (a working area,
with a default namespace for publishing), so parallel feature branches do not collide the way
files in git do. A developer edits the English string in a bundle; translation workflows trigger
automatically; completed translations become available immediately.

Applications choose their integration:

- **runtime** — "allows fast propagation of changes to UIs"
- **build time** — the repository is used only for translation and the data ships in the build

The stated reason for building any of this is worth quoting, because it is the argument
[D51](../../decisions/working-notes.md) made from first principles:

> For device UIs, a build might need additional approval from a third party like Apple. This
> causes unnecessary delays, especially in cases where a fix to a string needs to be rolled out
> immediately.

Netflix built a three-region distributed service to avoid App Store review latency on a typo.
We get the same property for free, because unlike them we *are* the server the client is already
connected to, on the same LAN.

Two further notes. In 2015 they listed "enhancing support for quantity strings (plurals) and
gender based strings" as **future work** — even Netflix shipped a global string service before
it handled plurals. And they wrote "we plan on releasing it as open source software." They did
not: there is no such repository under `github.com/Netflix`, and a search of the org for
`string`/`i18n`/`localization`/`locale` returns zero results. Part of why every open-source
project in §1 ends up on Weblate with files in git is that the commercial pattern was never
published as code.

### Pseudo-localisation, on by default, is the cheapest idea here

This is the one thing in this section I would copy immediately.

The problem Netflix states: translated text can be **up to 40% longer** than English — worst in
German, Hebrew, Polish, Finnish and Portuguese — and UIs designed in English assume English
string lengths, line heights and glyphs. With 26 languages, one layout mistake becomes "26
defects that need to be logged, managed, and resolved," when it could have been fixed once at
the English design stage. Their real example: *"Don't miss out."* → *"Lassen Sie sich nichts
entgehen!"*

Their transform, applied by **NFi18n** to every string served out of the Global String
Repository:

```
Find Help Online   →   [ƒîกี้ð Ĥéļþ Öกี้ļîกี้é one two]
```

Three parts, each catching a different defect:

| Element | Catches |
| --- | --- |
| `[` … `]` markers | Clipping — if a developer cannot see both brackets, an inflexible element truncated the string |
| ASCII → extended equivalents | Line-height overflow, missing font/encoding support — **and un-externalised strings, which stay plain English and therefore stand out** |
| `one two three four…` padding | Expansion, simulating the 40% |

Three details that show this was actually used rather than merely built:

- **It is opt-out, not opt-in.** Pseudo-localisation is "the default development language for all
  UI developers" in debug builds.
- **They exclude fields whose length is already capped upstream** — synopsis text, titles —
  because padding those produces false positives.
- **The hard part was metadata, not strings.** Titles, synopses, cast names and maturity ratings
  come from other systems, and without pseudo-localising those too "the experience felt
  incomplete, half pseudo localized, half English." That is precisely the string-catalogue /
  metadata split of [D45](../../decisions/working-notes.md) and
  [D46](../../decisions/working-notes.md), and Netflix confirms which half is harder.

Their own retrospective is the most useful part: *"architecting, and implementing the solution
would not be half of the battle, I would argue it was even less. The real work starts while
advocating, influencing, educating, and convincing development teams."* On a survey six weeks
in, the complaint was readability, and they proposed replacing the `one two three` padding with
vowel multiplication — `[ƒîîîกี้ð Ĥéééļþ ÖÖกี้ļîîîกี้ééé]`.

For us the organisational half is free — there is no team to convince — so we get the expensive
part of the idea at the cheap part's price.

### Locale testing is a screenshot pipeline, not a person with a TV

**Hydra** catalogues "every possible unique screen" across devices and locales. It does not
capture anything itself: existing UI automation runs data-driven tests in parallel across all
supported locales under Jenkins, screenshots go to S3, and metadata — page name, feature area,
UI platform, and a *unique screen definition* — goes to a relational database behind a REST API.
A German localisation specialist filters to the non-member flow in German across PS3, Web and
Android and reviews it in a fraction of the time.

Adding a locale is **one line in a property file**. Their stated gap: nothing detects that a
screen has *changed*, so review is not incremental.

This is the shape our [device-lab.md](../plans/device-lab.md) already has, pointed at a second
axis. If the UI automation exists anyway, locale × device screenshotting is nearly free, and it
is the only way anyone has found to make RTL and expansion bugs visible without owning 26
people.

### Subtitles: they mandate a richer format than we can render

This is where the gap between us and them is real rather than cosmetic, and it is worth knowing
before someone reports it as a bug.

Netflix's partner specifications require **IMSC 1.1** for Japanese timed text and **TTML1** for
right-to-left languages. The Japanese IMSC 1.1 creation guide calls for:

| Attribute | Purpose |
| --- | --- |
| `tts:writingMode="tbrl"` | Vertical text, top-to-bottom right-to-left |
| `tts:textCombine="all"` | *Tate-chū-yoko* — horizontal double-digit numbers inside vertical text |
| `tts:rubyPosition`, `tts:rubyAlign`, `tts:rubyReserve` | Ruby (furigana), with reserved space so vertical subtitles do not shift between events |
| `tts:textEmphasis="filled circle outside"` | *Bouten* emphasis marks |
| `ebutts:multiRowAlign="start"` | Left justification preserved across wrapped lines |

**ASS cannot express most of this, and libass does not implement it.** Searching the libass
source for `ruby` returns **zero** matches in `ass_render.c`, `ass_parse.c`, `ass_types.h` and
`ass_shaper.c`. Vertical text exists only as the VSFilter `@font` convention — a flag on the
font descriptor — with open bugs against it (libass #695 *"@font vertical writing is displayed
wrong"*, #600 *"libass mangles positioning of Japanese punctuation when rendering vertically"*).

So [D11](../../decisions/working-notes.md)'s "libass everywhere" has a ceiling: it renders
everything a personal library actually contains — SRT, ASS, VTT, PGS, CC — correctly once
[D50](../../decisions/working-notes.md)'s build flags are right, and it cannot render
broadcast-grade Japanese typesetting. That is the correct trade for a home server and it should
be stated rather than discovered.

### Metadata: per-territory, and the artwork problem solved at the source

Netflix localises titles, synopses and genre tags per territory, and tests artwork variants
regionally. The relevant detail is how they handle the problem
[D45](../../decisions/working-notes.md) identified — that posters have title text baked into the
pixels. Netflix's partner guidelines ask for the **title treatment** (the logo/title lockup) as a
**vectorised file — AI, EPS or PDF** — and accept localised title treatments as separate
deliverables. They compose the artwork per locale because they can compel their suppliers to
deliver the layers separately.

We cannot. TMDB hands us finished JPEGs with the title already rendered into them, which is why
per-language artwork has to be a *row* in our schema rather than a composition step. Same
problem, opposite solution, forced by who we can make demands of.

### What they avoid entirely, which we cannot

The most useful observation is a negative one. **Netflix's primary browse surface is ranked rows,
not an alphabetical grid** — "Trending Now", "Because you watched", personalised artwork,
personalised ordering. Alphabetical sort exists but is a secondary affordance inside "Browse by
Language".

Personalised ranking replaces collation. That means the single hardest problem in §3 — that
Swedish, German, Chinese and Korean each want a different order for the same list, and Go's
implementation gets Korean wrong — is a problem Netflix largely does not have to solve on the
browse screen, because the browse screen is not sorted alphabetically.

A personal media server's primary surface **is** the A–Z library grid. So collation is more
central for us than for them, on a far smaller budget. That asymmetry is why
[D47](../../decisions/working-notes.md) spends a column of the schema on it.

### Cultural data comes from CLDR, for everyone

Netflix states plainly that date, time, calendar, timezone, number, currency, phone-number and
sorting data "are collected and stored in the CLDR (Unicode Common Locale Data Repository)."
This is the one place where we and Netflix use the identical source — and it sharpens §3's
finding, because `x/text/collate` is pinned at **CLDR 23 (2013)** while Netflix consumes current
CLDR through ICU. Same data, thirteen years apart.

### A different model worth knowing about: Mozilla Fluent

Not a media company, but the deepest published thinking on the problem
[D51](../../decisions/working-notes.md) settled, and it disagrees with the choice we made.

Fluent's central idea is **asymmetric localisation**: a translation may be structurally more
complex than its source, and *the localiser* adds that complexity without the developer
anticipating it and without any other language being affected. Polish needs grammatical case and
four plural forms; English needs one string; in Fluent that asymmetry lives entirely in the
Polish file.

```fluent
brand-name = { $case ->
   *[nominative] Firefox
    [genitive] Firefoksa
}
hello = Witaj w { brand-name(case: "dative") }
```

Fluent also has message references with arguments (above), compound messages with attributes that
map onto UI components (`confirm`, `.ok`, `.cancel`), and — the detail that matters operationally
— **error recovery**: a malformed Fluent resource degrades gracefully, where ICU MessageFormat
rejects the message and you ship missing UI text.

i18next covers **plurals** per language through CLDR plural categories, which is most of the
value. It does not give a localiser a way to introduce a *gender* or *case* variant the English
source does not have. For a media server's UI — mostly nouns, counts and short labels — that is
an acceptable gap, and the catalogue format is what would have to change if it ever is not. Worth
recording as the known limit of D51 rather than a reason to reverse it.

## 9. Font fallback, and whether a "plan B" font exists

§6 established that a device may have no font for a script. This section answers the next
question: what do you render instead, and is there one font that can always be the answer.

### No. One font cannot cover Unicode, and this is structural

OpenType caps a font at **65,535 glyphs**. Unicode 18 has over 150,000 assigned characters. The
constraint is visible in the only project that seriously tried — GNU Unifont splits by plane
precisely because a single file cannot hold the rest. Measured from the binaries:

```
unifont-18.0.01.otf         5.07 MB   numGlyphs = 58,911   (Plane 0, the BMP)
unifont_upper-18.0.01.otf   5.68 MB   numGlyphs = 57,729   (Planes 1–15)
```

58,911 glyphs is not a design choice; it is the ceiling arriving.

So every platform uses a **cascade** — an ordered fallback list — and the interesting question
is not "which font" but "what goes at the end of the list".

### What the platforms actually do

| Platform | Mechanism | Notes |
| --- | --- | --- |
| **Android** | `fonts.xml` — **161 families**, named families first, then a long list keyed by script subtag (`und-Arab`, `und-Deva`, `und-Thai`, …) | Measured from AOSP `main`. **The chain ends at `und-Yezi` with no last-resort entry** — grep for `lastresort`/`notdef` returns nothing. Fall off the end and you get tofu. |
| **Apple** | `CTFontCopyDefaultCascadeListForLanguages`, which takes a **locale** and returns a prioritised list matching the original font's style, weight and width | Ships `.LastResort` in `/System/Library/Fonts`, and has since Mac OS 8.5 (1998) |
| **Windows** | `IDWriteFontFallback` (8.1+), queried by string | Avoids loading unused fonts; hard to cache |
| **Linux** | fontconfig | The weak link — "at least Debian simply doesn't have a correct config", and **browsers hardcode common CJK font names** to work around it |

Two things fall out that matter more than the API differences.

**Font choice for CJK must be by language, not by script.** Han unification gives Japanese,
Simplified Chinese and Traditional Chinese the same code points with different expected glyph
forms — "a rough analogy is to use the same code point to encode Latin capital A and Greek
capital alpha". Apple's cascade API takes a locale for exactly this reason, so it "will order
the CJK fonts with the preferred language first". A single script-keyed fallback renders
Japanese text with Chinese letterforms: not tofu, just subtly wrong, and far harder to notice.

**Bundling Noto is what everyone considers and nobody does** — "tens of MB at least", which
matches §6's measured 15.7 MB for one weight of `NotoSansCJKjp`.

### Even Netflix does not solve this by bundling

**Netflix Sans**, commissioned from Dalton Maag for a company operating in 190 countries, covers
— per Dalton Maag's own portfolio page — **"Cyrillic, Greek, Latin, Latin (Vietnamese
Extension)"**. Over 800 languages, and **no CJK, Arabic, Hebrew, Thai or Indic**. Amazon Ember,
also Dalton Maag, has the same shape of coverage; Amazon distributes it free for Fire TV apps.

So the custom brand typefaces of the two largest streaming services cover exactly the scripts
the Android CDD already mandates, and everything else falls through to the platform cascade.
**On a device with no Japanese font, Netflix renders tofu too.** Their remedy is not technical:
they have commercial relationships with the device manufacturers, and their apps are
preinstalled on hardware built to carry them.

We do not have that. But we have something they do not: we are a server on the same LAN as the
client, so we can simply send the font. That is why [D53](../../decisions/working-notes.md)'s
serve-on-demand answer is not a poor imitation of the commercial practice — for our topology it
is strictly better than it.

### The three candidates for the end of the chain

Measured from the actual binaries, all under permissive font licences:

| Font | Size | Glyphs | Codepoints mapped | Licence |
| --- | --- | --- | --- | --- |
| **Noto** (per script) | 15.7 MB for `NotoSansCJKjp-Regular` alone | — | per script | OFL 1.1 |
| **GNU Unifont** | 5.07 MB + 5.68 MB | 58,911 + 57,729 | ~116,600 | GPLv2+ w/ Font Embedding Exception **and** OFL 1.1 (dual, since 13.0.04) |
| **Unicode Last Resort** | 9.15 MB | 5,776 | **1,114,112** | OFL 1.1 |
| **Unicode Last Resort HE** | **0.56 MB** | 387 | **1,114,112** | OFL 1.1 |

**Noto** is Google's answer and the name is literally "No Tofu". It is the right tier-1 choice —
real, well-drawn glyphs — but it is ~150 families, not a font, and you must choose per script
*and* per language.

**GNU Unifont** is the completist option: a 16-pixel-grid design, now shipped as OpenType with
scalable outlines, whose stated purpose is "to render something meaningful for each Unicode code
point, rather than a blank box ('tofu')". 10.75 MB buys essentially all of assigned Unicode —
less than one weight of Noto CJK. It is legible and it is ugly.

**Unicode Last Resort** is the one that is not really a font. Designed by Apple, Michael
Everson and Unicode, it maps **every one of the 1,114,112 code points** to a glyph that
identifies the *Unicode block*: a representative character (ක for Sinhala, Ω for Greek, Я for
Cyrillic) inside a square frame, with the block name and code-point range embedded in the frame
and visible at large sizes. It distinguishes unassigned code points and the 66 noncharacters
from merely-unavailable ones. The Unicode Standard's own framing is that it lets users "readily
identify what type of font needs to be installed."

The **High-Efficiency** variant is the interesting one: **588 KB** for full coverage, achieved
with a cmap **format 13** (many-to-one range mappings) subtable — the right table for a font
where thousands of code points share one glyph. Its cost is compatibility: the README warns
format 13 "may not be supported in some environments, such as most Windows and Adobe apps."

**For us that caveat does not bite,** and this is checkable rather than hopeful. Our entire text
stack is FreeType — libass links it natively, and JASSUB compiles it to WASM. FreeType's
`src/sfnt/ttcmap.c` implements `tt_cmap13` and `TT_CONFIG_CMAP_FORMAT_13` is enabled by default
in `ftoption.h`. JASSUB patches FreeType only to disable pthreads. So the 588 KB variant works
everywhere we render text.

### The dissent, which is worth taking seriously

Firefox deliberately **refuses** to use Last Resort. Mozilla bug 1416762 — *"Avoid using the
LastResort font on macOS for glyph fallback — prefer to draw hexboxes for unsupported
characters"* — was filed and fixed in November 2017, with the rationale that hex boxes give
"better cross-platform consistency and because these actually convey more information (the exact
codepoint involved) than the LastResort glyphs."

That is correct for a browser and wrong for us, and the difference is who is reading. A
developer wants the code point. A person on a sofa wants to know whether the app is broken or
their TV is missing a Japanese font — and a framed か answers that where `U+304B` does not. The
resolution is to take both: Last Resort in release builds, hex boxes in development, which pairs
with [D52](../../decisions/working-notes.md)'s debug-build transform.

## 10. Hosting a font set: what to host, and how much it costs

§9 settled the floor. This section settles the tier above it — a **set** of served faces broad
enough to cover the styles subtitles actually ask for. Every number below was measured with
`fonttools` against the real binaries.

### The set is already enumerated, by CTA-708

The question "how many faces cover most subtitle styles" has a standardised answer.
**CEA/CTA-708 defines exactly eight font tags**: undefined, monospaced serif, proportional
serif, monospaced sans-serif, proportional sans-serif, casual, cursive, and small capitals.
The W3C's 608-to-WebVTT mapping gives each a concrete CSS stack:

```css
monospaced serif        font-family: Courier, monospace, serif
proportional serif      font-family: Times New Roman, serif
monospaced sans-serif   font-family: Helvetica, monospace, sans-serif
proportional sans-serif font-family: Arial, Swiss, sans-serif
casual                  font-family: Dom, Impress, casual
cursive                 font-family: Coronet, Marigold, cursive
small capitals          font-family: Engravers Gothic, monospace, sans-serif;
                        font-variant: small-caps
```

Every named face there is proprietary, which is exactly why an open-source server has to bring
its own. Two observations from the captioning literature sharpen the list. Joe Clark's critique
counts them as "really 6½" and makes the load-bearing point: **small capitals is not a font
style** — "small caps can be applied to any typeface category, including cursive." And he notes
that **all eight are optional in decoders**, so nothing breaks if we collapse the exotic ones.

Measured, "6½" is six files, because `smcp` is a feature and not a face.

### Measured: seven styles, Latin + Greek + Cyrillic, all weights

| CTA-708 style | OFL substitute | raw | woff2 | coverage |
| --- | --- | --- | --- | --- |
| proportional sans-serif | `NotoSans[wdth,wght]` | 2001 K | 848 K | Latin+Greek+Cyrillic, 3094 cp, **`smcp` present** |
| proportional serif | `NotoSerif[wdth,wght]` | 1842 K | 765 K | same, `smcp` present |
| monospaced sans-serif | `NotoSansMono[wdth,wght]` | 1668 K | 704 K | same, `smcp` present |
| monospaced serif | `CourierPrime-Regular` | 69 K | 24 K | Latin+Greek, 383 cp |
| casual | `ComicNeue-Regular` | 55 K | 22 K | Latin only, 303 cp |
| cursive | `DancingScript[wght]` | 130 K | 55 K | Latin only, 559 cp |
| small capitals | — | 0 | 0 | the `smcp` feature on the sans face |
| **total, 6 files** | | **5.63 MB** | **≈2.42 MB** | |

Adding real italics for sans and serif takes it to 8 files, **10.18 MB / 4.30 MB woff2**. There
is no `NotoSansMono-Italic`, and it does not matter, because **libass synthesises both**:
`ass_font.c` has `ass_glyph_embolden()` (via `FT_Outline_Embolden`) and `ass_glyph_italicize()`,
applied when the face lacks the style. So real italics are a quality upgrade, not a requirement.

**Variable fonts are what make this tractable.** The three Noto faces carry `wght` *and* `wdth`
axes, so one file replaces what would otherwise be nine weights × three widths. Without variable
fonts, "enough faces to cover the common styles" is a combinatorial problem; with them it is six
files and 2.4 MB.

Casual and cursive are Latin-only in any usable open font, and that is fine — they are
decoder-optional and have no meaningful CJK or Arabic equivalent. For non-Latin scripts they
collapse to the sans face.

### The part that is ours alone: subset per subtitle track

This is where being the server changes the answer rather than just the logistics.

Google Fonts serves CJK by cutting each font into ~100 fixed `unicode-range` slices, because a
web font server has no idea what text the page will render. **We do know.** We have the subtitle
file. So we can subset to the exact code points that track uses.

Measured against `NotoSansCJKjp-Regular.otf` (15.70 MB whole):

| Input | distinct code points | subset raw | subset woff2 |
| --- | --- | --- | --- |
| 20,000 chars of Japanese — about one film's dialogue | **1,202** | **582 K** | **441 K** |
| 21,000 chars of Chinese | 1,816 | 988 K | 732 K |
| 168,000 chars of Japanese — a generous upper bound | 2,405 | 1220 K | 900 K |
| everything | 65,535 | 15.70 M | 11.15 M |

**A film-length Japanese subtitle needs 441 KB, not 15.7 MB — a 36× reduction.** The reason the
number is so small is a property of the language rather than of the compression: 168,000
characters of running Japanese contain only 2,405 distinct code points, because the Jōyō set is
2,136 kanji and real text saturates well below that.

**Do it at scan time, not at request time.** [D11](../../decisions/working-notes.md) already
extracts subtitles at scan time in the media node, which is where FFmpeg lives; the subset font
is one more scan-time artifact beside the extracted track. That keeps a font parser out of the
core's request path entirely.

**And the tool is already in the build.** `hb-subset` is part of HarfBuzz — `util/hb-subset.cc`
as a CLI, and a public C API (`hb_subset_input_create_or_fail`,
`hb_subset_input_unicode_set`, `hb_subset_or_fail`) — and we ship HarfBuzz anyway for libass
([D50](../../decisions/working-notes.md)). No new dependency.

### Embedded fonts are the one genuinely dangerous part of this

Serving a font **extracted from a media file** means redistributing attacker-supplied binary to
every client in the household, where each client's FreeType parses it. This is not a theoretical
surface.

**Two FreeType bugs are in CISA's Known Exploited Vulnerabilities catalogue** — confirmed
exploited in the wild, not merely reported:

| CVE | CVSS | KEV added | What it parses |
| --- | --- | --- | --- |
| **CVE-2020-15999** | 9.6 | 2021-11-03 | Heap overflow in `Load_SBit_Png` — **PNG images embedded inside fonts**. CISA: "part of an exploit chain" |
| **CVE-2025-27363** | 8.1 | 2025-05-06 | Out-of-bounds write parsing **TrueType GX / variable-font subglyph structures**; arbitrary code execution, FreeType ≤ 2.13.0 |

The second is from **March 2025**, so this is a live surface rather than a historical one. And
FreeType ≤ 2.13.0 is exactly what an old Fire TV stick is likely to be carrying — the same
devices §6 already flagged for missing fonts. The server is an **amplifier**: one bad file
reaches every device in the house, including the ones nobody updates.

**What the commercial players do: they do not accept fonts at all.** Netflix's IMSC 1.1 Text
Profile is unambiguous — `tts:fontFamily` *"Should not be used. If specified, the
tts:fontFamily should be equal to `proportionalSansSerif`."* One generic family, chosen by the
player, for a company that has **contracts with the people supplying the content**. If Netflix
will not accept a named font from a vetted studio partner, accepting an arbitrary binary from a
torrent by default is hard to justify.

**What the open-source field does is the exact inverse.** `jellyfin-web`'s `renderSsaAss()`
walks every media attachment and hands each one to libass as a URL:

```js
const supportedFonts = ['application/vnd.ms-opentype', 'application/x-truetype-font',
                        'font/otf', 'font/ttf', 'font/woff', 'font/woff2'];
attachments.forEach(i => {
    if (supportedFonts.includes(i.MimeType)) availableFonts.push(apiClient.getUrl(i.DeliveryUrl));
});
```

Two things to notice. The filter is on `i.MimeType` — **metadata from inside the container**,
which the attacker writes — and their own comment says its purpose is to skip cover art, not to
screen anything. And the *fallback* font set, the trusted one, is behind
`config.EnableFallbackFont`, default off. So Jellyfin's untrusted font source is on by default
and its trusted one is off by default. That is backwards, and it is the field's norm.

### Hardening, and its measured cost

If embedded fonts are used at all, the subsetting pass from earlier in this section is also the
place to strip the dangerous parts. `hb-subset` exposes both controls:
`HB_SUBSET_FLAGS_NO_HINTING` (`--no-hinting`) and `--drop-tables+=…`.

Which tables matter, measured against real fonts:

```
NotoSans[wdth,wght].ttf        risky tables present: glyf, prep
CourierPrime-Regular.ttf       risky tables present: cvt , fpgm, glyf, prep
NotoSansCJKjp-Regular.otf      risky tables present: none  (CFF, no TrueType hinting VM)
```

`fpgm`/`prep`/`cvt ` are the **TrueType hinting bytecode** — a stack VM interpreted by FreeType,
and a classic exploitation target. `CBDT`/`CBLC`/`sbix`/`SVG `/`COLR`/`CPAL` are the embedded
bitmap and colour tables, which is where CVE-2020-15999 lived and which a subtitle font has no
use for.

The cost of dropping all of it:

```
CJK subset, as before                            582 K
CJK subset, --no-hinting                         498 K
CJK subset, --no-hinting --drop-tables+=CBDT,…   498 K
```

**Hardening makes the file 14% smaller.** There is no trade-off to weigh here, which is unusual
enough to be worth stating plainly.

One caveat against overclaiming: re-serialisation is **not** sanitisation. `hb-subset` parses the
hostile input with HarfBuzz, which has its own bug history, and a determined malicious font aimed
at the *subsetter* is a live possibility. What it does buy is that the client parses a file our
tooling emitted, with the hinting VM and bitmap tables gone, rather than the original bytes. The
subsetter itself must therefore run inside the same Landlock+seccomp jail as FFmpeg
([D26](../../decisions/working-notes.md)) — which it does, because it runs in the node.

## 11. What this costs, and when

Ordered by *when the decision must be made*, not by size:

| Work | Deadline | Why then |
| --- | --- | --- |
| NFC normalisation in the report contract | **now** | It is a field in a wire format ([D39](../../decisions/working-notes.md)) |
| Unicode-safe regex corpus + its test | **now** | The scanner is Phase 1 |
| Translation table + `original` record + genre enum | **before the schema** | A migration otherwise ([library-model.md](../plans/library-model.md)) |
| Sort-key column, per (item, locale) | **before the schema** | Same |
| Two FTS5 indexes and the routing rule | **before the search schema** | The tokeniser is baked into the table |
| "No translated strings in the database" | **now** | It is a rule, and it is free if adopted early |
| `Accept-Language` on the read API | with the API | Standard, and it is the fallback chain |
| react-i18next + fetched catalogues | with the first client | Extraction is miserable to retrofit |
| libass build flags + feature toggles | with the subtitle subsystem | Changes what we compile |
| Weblate | **not yet** | §1 — it needs translators to exist first |
| RTL in the device lab | with the device lab | One locale, one pass |

Weblate deserves the "not yet". Immich's integration is two GitHub Actions workflows, a GitHub
App with scoped tokens, and a required PR check that fails any PR touching `i18n/!(en)**.json`
without the bot's approval — i.e. **humans may only edit the English catalogue; every other file
is machine-owned**. Their release flow locks the Weblate component, forces a commit and push,
finds the single `weblate` PR, approves it, merges it, polls for the merge, and unlocks, with
cleanup on timeout. That is the real cost of a translation platform, and none of it buys anything
until there are translators. Design the catalogue format for it now; connect it when somebody
volunteers.

## 12. What was not settled

- **Korean collation.** `x/text/collate` gets it wrong and nobody is fixing it. Options are: live
  with it, ship a small Korean tailoring of our own, or link ICU via cgo for the sort-key
  generator only (it runs on the *node*, not the core, so the cross-compilation argument is
  weaker there than it first appears). Not resolved.
- **Whether per-language artwork is worth the storage** at household scale. Kyoo says yes; we have
  no measurement.
- **CJK filename identification.** That the top four Jellyfin metadata plugins are Chinese-market
  says the built-in identification fails for those libraries, but not *how*. Separating "the
  regexes were ASCII-only" from "TMDB has poor coverage" from "the release-naming conventions are
  different" needs a corpus we do not have.
- **Which locales a household actually uses.** The sort-key-per-locale design is cheap if the
  answer is 1–3 and annoying if it is 10. Measurable once there is a server.
