package docclass

import (
	"errors"
	"path"
	"strings"

	"obsidian-harness/internal/model"
)

type Class = model.DocClass

const (
	ClassUnknown    = model.DocClassUnknown
	ClassPlanMaster = model.DocClassPlanMaster
	ClassPlanWeek   = model.DocClassPlanWeek
)

type MatchField string

const (
	MatchFieldPath     MatchField = "path"
	MatchFieldFileName MatchField = "file_name"
)

type Match struct {
	RuleID  string
	Field   MatchField
	Pattern string
}

type Classification struct {
	Class   Class
	Matches []Match
}

func (c Classification) IsPlan() bool {
	return c.Class == ClassPlanMaster || c.Class == ClassPlanWeek
}

type Rule struct {
	ID                   string
	Class                Class
	PathPrefixes         []string
	PathContains         []string
	FileNameEquals       []string
	FileNameContains     []string
	RequirePathMatch     bool
	RequireFileNameMatch bool
}

type Classifier struct {
	rules []Rule
}

var (
	ErrRuleIDRequired      = errors.New("docclass: rule id is required")
	ErrRuleClassRequired   = errors.New("docclass: rule class is required")
	ErrRuleMatcherRequired = errors.New("docclass: rule must define at least one matcher")
)

func NewClassifier(rules []Rule) (Classifier, error) {
	normalized := make([]Rule, 0, len(rules))
	for _, rule := range rules {
		if strings.TrimSpace(rule.ID) == "" {
			return Classifier{}, ErrRuleIDRequired
		}
		if strings.TrimSpace(string(rule.Class)) == "" {
			return Classifier{}, ErrRuleClassRequired
		}
		if !hasMatchers(rule) {
			return Classifier{}, ErrRuleMatcherRequired
		}

		normalized = append(normalized, Rule{
			ID:                   strings.TrimSpace(rule.ID),
			Class:                rule.Class,
			PathPrefixes:         normalizePatterns(rule.PathPrefixes, normalizePathPattern),
			PathContains:         normalizePatterns(rule.PathContains, normalizePathPattern),
			FileNameEquals:       normalizePatterns(rule.FileNameEquals, normalizeFileNamePattern),
			FileNameContains:     normalizePatterns(rule.FileNameContains, normalizeFileNamePattern),
			RequirePathMatch:     rule.RequirePathMatch,
			RequireFileNameMatch: rule.RequireFileNameMatch,
		})
	}

	return Classifier{rules: normalized}, nil
}

func RecommendedRules() []Rule {
	return []Rule{
		{
			ID:    "plan-week-v1",
			Class: ClassPlanWeek,
			PathPrefixes: []string{
				"0-\u6392\u671f/",
				"\u6392\u671f/",
				"plans/",
				"plan/",
			},
			FileNameContains: []string{
				"week",
				"\u5468",
				"\u6267\u884c\u8868",
				"checkpoint",
			},
			RequireFileNameMatch: true,
		},
		{
			ID:    "plan-master-v1",
			Class: ClassPlanMaster,
			PathPrefixes: []string{
				"0-\u6392\u671f/",
				"\u6392\u671f/",
				"plans/",
				"plan/",
			},
			PathContains: []string{
				"/\u8ba1\u5212/",
				"/roadmap/",
				"/schedule/",
			},
			FileNameContains: []string{
				"\u8ba1\u5212",
				"\u6392\u671f",
				"roadmap",
				"schedule",
				"sprint",
				"plan",
			},
			RequireFileNameMatch: true,
		},
	}
}

func (c Classifier) Classify(filePath string) Classification {
	normalizedPath := normalizePath(filePath)
	fileName := path.Base(normalizedPath)

	for _, rule := range c.rules {
		matches := rule.match(normalizedPath, fileName)
		if len(matches) == 0 {
			continue
		}
		return Classification{
			Class:   rule.Class,
			Matches: matches,
		}
	}

	return Classification{Class: ClassUnknown}
}

func (r Rule) match(normalizedPath string, fileName string) []Match {
	pathMatches := collectPrefixMatches(r.ID, MatchFieldPath, normalizedPath, r.PathPrefixes)
	pathMatches = append(pathMatches, collectContainsMatches(r.ID, MatchFieldPath, normalizedPath, r.PathContains)...)
	fileMatches := collectExactMatches(r.ID, MatchFieldFileName, fileName, r.FileNameEquals)
	fileMatches = append(fileMatches, collectContainsMatches(r.ID, MatchFieldFileName, fileName, r.FileNameContains)...)

	if r.RequirePathMatch && len(pathMatches) == 0 {
		return nil
	}
	if r.RequireFileNameMatch && len(fileMatches) == 0 {
		return nil
	}
	if len(pathMatches) == 0 && len(fileMatches) == 0 {
		return nil
	}

	return append(pathMatches, fileMatches...)
}

func collectPrefixMatches(ruleID string, field MatchField, value string, prefixes []string) []Match {
	var matches []Match

	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			matches = append(matches, Match{RuleID: ruleID, Field: field, Pattern: prefix})
		}
	}

	return matches
}

func collectExactMatches(ruleID string, field MatchField, value string, exact []string) []Match {
	var matches []Match

	for _, expected := range exact {
		if value == expected {
			matches = append(matches, Match{RuleID: ruleID, Field: field, Pattern: expected})
		}
	}

	return matches
}

func collectContainsMatches(ruleID string, field MatchField, value string, contains []string) []Match {
	var matches []Match

	for _, fragment := range contains {
		if strings.Contains(value, fragment) {
			matches = append(matches, Match{RuleID: ruleID, Field: field, Pattern: fragment})
		}
	}

	return matches
}

func hasMatchers(rule Rule) bool {
	return len(rule.PathPrefixes) > 0 ||
		len(rule.PathContains) > 0 ||
		len(rule.FileNameEquals) > 0 ||
		len(rule.FileNameContains) > 0
}

func normalizePatterns(values []string, normalize func(string) string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		normalized := normalize(value)
		if normalized == "" {
			continue
		}
		out = append(out, normalized)
	}
	return out
}

func normalizePath(filePath string) string {
	normalized := normalizePathPattern(filePath)
	if normalized == "." {
		return ""
	}
	return strings.TrimPrefix(normalized, "./")
}

func normalizePathPattern(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, `\`, "/")
	value = path.Clean(value)
	if value == "." {
		return value
	}
	return strings.ToLower(value)
}

func normalizeFileNamePattern(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
