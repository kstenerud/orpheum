// SPDX-License-Identifier: AGPL-3.0-or-later

package httpgzip

import "strings"

// Accepts is whether an Accept-Encoding header accepts gzip: named, or by "*", with a weight
// above zero.
func Accepts(header string) bool {
	for coding := range strings.SplitSeq(header, ",") {
		name, parameters, _ := strings.Cut(strings.TrimSpace(coding), ";")
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "gzip" && name != "*" {
			continue
		}
		weight := strings.ReplaceAll(strings.ToLower(parameters), " ", "")
		if !strings.HasPrefix(weight, "q=0") || strings.Trim(strings.TrimPrefix(weight, "q=0"), ".0") != "" {
			return true
		}
	}
	return false
}
