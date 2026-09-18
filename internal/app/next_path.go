package app

import "regexp"

var linkRE = regexp.MustCompile(`<https://api\.github\.com/([^>]+)>;\s*rel="([^"]+)"`)

// LinkHeader is the value of a GitHub API response's Link header.
type LinkHeader string

// NextPath returns the "next" page path found in the header.
// If none is found, it returns path unchanged, signaling "no more pages" to callers that compare the two.
func (l LinkHeader) NextPath(path string) string {
	for _, m := range linkRE.FindAllStringSubmatch(string(l), -1) {
		if 2 < len(m) && m[2] == "next" {
			return m[1]
		}
	}

	return path
}
