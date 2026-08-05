package main

import "regexp"

var linkRE = regexp.MustCompile(`<https://api\.github\.com/([^>]+)>;\s*rel="([^"]+)"`)

// nextPathFromLink returns the "next" page path found in linkHeader (a GitHub API response's Link header).
// If none is found, it returns currentPath unchanged, signaling "no more pages" to callers that compare the two.
func nextPathFromLink(linkHeader, path string) string {
	for _, m := range linkRE.FindAllStringSubmatch(linkHeader, -1) {
		if 2 < len(m) && m[2] == "next" {
			return m[1]
		}
	}

	return path
}
