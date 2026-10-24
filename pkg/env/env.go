// Package env resolves jt environment variables.
//
// Every variable is read as JT_<NAME> first and falls back to the legacy
// JIRA_<NAME> so that existing jira-cli setups keep working.
package env

import "os"

const (
	// Prefix is the primary environment variable prefix.
	Prefix = "JT_"
	// LegacyPrefix is the jira-cli environment variable prefix.
	LegacyPrefix = "JIRA_"
)

// Get returns the value of JT_<name>, or JIRA_<name> if the former is unset.
func Get(name string) string {
	if v := os.Getenv(Prefix + name); v != "" {
		return v
	}
	return os.Getenv(LegacyPrefix + name)
}
