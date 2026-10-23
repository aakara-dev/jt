// Package branch derives git branch names from Jira issues.
package branch

import (
	"regexp"
	"strings"
)

const maxSlugLen = 40

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Name returns a branch name such as "feat/DATA-12-add-retry-to-loader".
// Bugs get a "fix/" prefix, everything else "feat/".
func Name(key, summary, issueType string) string {
	prefix := "feat"
	if strings.EqualFold(issueType, "bug") {
		prefix = "fix"
	}
	slug := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(summary), "-"), "-")
	if len(slug) > maxSlugLen {
		slug = slug[:maxSlugLen]
		if i := strings.LastIndex(slug, "-"); i > maxSlugLen/2 {
			slug = slug[:i]
		}
		slug = strings.Trim(slug, "-")
	}
	name := prefix + "/" + strings.ToUpper(key)
	if slug != "" {
		name += "-" + slug
	}
	return name
}
