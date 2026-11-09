// SPDX-License-Identifier: AGPL-3.0-or-later

// Package strictjson decodes JSON strictly (D456): the one way the Go code reads a JSON document
// (AGENTS.md rule 11). It checks a document against D456's rules in one pass over its bytes, then
// decodes it with encoding/json, which would otherwise fill one field from two keys, put U+FFFD
// where the text was invalid, and read past nothing a Decoder left behind. It is BONJSON's secure
// compliance, applied to JSON text: keys are NFC-normalised for the duplicate check.
//
// Unmarshal decodes bytes; Decode decodes a reader holding one document. Those are for a sender
// Orpheum controls. A Foreign's Unmarshal and Decode are for another's (D458): anything JSON allows,
// within the same limits, a repeated key keeping the value its Repeats says. Each refusal wraps
// ErrRefused and names the rule, the path ($."key"[index]) and the byte offset. A document that is
// not JSON at all is refused with encoding/json's own *json.SyntaxError, and one that is JSON but
// does not fit the target, with encoding/json's own errors, as callers knew them.
//
// # Invariants
//
//   - A key said twice in one object is refused, compared after NFC normalisation.
//     TestUnmarshal_RefusesWhatEachRuleRefuses.
//   - A key that names a field of the target's struct only when case is ignored is refused, at
//     any depth: in nested structs, behind pointers, in slices' and arrays' elements, maps' values,
//     and through embedded structs, which are followed as encoding/json promotes them. A map's
//     keys, and whatever an interface, a json.RawMessage or a type that decodes itself takes, are
//     free-form. TestUnmarshal_RefusesAKeyNamingAFieldOnlyInAnotherCase,
//     TestUnmarshal_AcceptsKeysInTheirFieldsCaseAndFreeFormKeysInAnyCase,
//     TestUnmarshal_FollowsEncodingJSONsChoiceBetweenFieldsClaimingOneName.
//   - Invalid UTF-8 is refused, raw or as a lone surrogate escape; so is a NUL, raw or escaped, in
//     any string. TestUnmarshal_RefusesWhatEachRuleRefuses.
//   - Data after the top-level value is refused, from bytes and from a reader.
//     TestUnmarshal_RefusesWhatEachRuleRefuses,
//     TestDecode_RefusesASecondValueThatEncodingJSONsDecoderLeftUnread.
//   - The limits are BONJSON's recommended defaults: nesting depth 500 (the top-level value is at
//     depth 1), 1,000,000 elements in one array or pairs in one object, 10,000,000 bytes in one
//     string after its escapes; and 100,000,000 bytes in a document, under BONJSON's 2,000,000,000.
//     A caller's own bound on a reader comes first. TestUnmarshal_HoldsEachLimitAtItAndRefusesPastIt,
//     TestLimits_AreBONJSONsRecommendedDefaults, TestCheck_RefusesADocumentPastItsBound.
//   - An unknown field is accepted (D456 §6), and a document that keeps every rule decodes exactly as
//     encoding/json decodes it. TestUnmarshal_AnUnknownFieldIsAccepted,
//     TestUnmarshal_DecodesADocumentAsEncodingJSONDoes, TestUnmarshal_NotJSONIsSaidInEncodingJSONsWords.
//   - A Foreign takes an escaped NUL, and reads invalid UTF-8 and a lone surrogate as encoding/json
//     does, as U+FFFD; keys compare without NFC; the limits, the data after the value and a key in
//     another case are refused as above. TestForeign_TakesWhatJSONAllowsAsEncodingJSONReadsIt,
//     TestForeign_StillRefusesWhatNoSenderIsAllowed.
//   - A Foreign keeps a repeated key's first or last value, and the members it does not keep are
//     cut from the document before it is decoded, so nothing of theirs is merged in.
//     TestForeign_ARepeatedKeyKeepsTheValueItsSenderKeeps,
//     TestForeign_FirstWinsReadsFfprobesRepeatedTagAsFfprobeShowsIt.
//   - No compiled file outside this package decodes with encoding/json.
//     TestArch_JSONIsDecodedStrictly.
//   - It is public, for the public discovery packages as for the module, and imports nothing of
//     the module's. TestArch_PublicPackagesImportNothingInternal; the depguard rule pkg-is-a-leaf.
//
// # Read before changing
//
//   - D456: the rules, and BONJSON's security rules they come from.
//   - D458: which senders get which rules.
//
// # Where to change
//
//   - strictjson.go; foreign.go for a foreign sender's rules.
package strictjson
