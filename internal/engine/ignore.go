package engine

import "strings"

func applyIgnore(issue *Issue, rules []IgnoreRule) {
	for i := len(rules) - 1; i >= 0; i-- {
		rule := rules[i]
		if ignoreMatches(*issue, rule) {
			issue.Ignored = true
			issue.IgnoreReason = rule.Reason
			if issue.IgnoreReason == "" {
				issue.IgnoreReason = "matched ignore rule"
			}
			return
		}
	}
}

func ignoreMatches(issue Issue, rule IgnoreRule) bool {
	if rule.File != "" && !matchPattern(rule.File, issue.RelativePath) {
		return false
	}
	if rule.Keyword != "" && rule.Keyword != issue.Keyword && rule.Keyword != issue.KeywordLocation {
		return false
	}
	if rule.Property != "" && !propertyMatches(issue, rule.Property) {
		return false
	}
	return true
}

func propertyMatches(issue Issue, ruleProperty string) bool {
	if ruleProperty == issue.Property || ruleProperty == issue.InstanceLocation {
		return true
	}
	if strings.HasPrefix(ruleProperty, "/") {
		return false
	}
	return matchPattern(ruleProperty, issue.Property)
}
