package docclass

import (
	"errors"
	"testing"
)

func TestRecommendedRulesClassifyWeekPlanByManagedPathAndName(t *testing.T) {
	classifier, err := NewClassifier(RecommendedRules())
	if err != nil {
		t.Fatalf("NewClassifier() error = %v", err)
	}

	got := classifier.Classify("0-\u6392\u671f\\Week1 \u9010\u65e5\u6267\u884c\u8868.md")
	if got.Class != ClassPlanWeek {
		t.Fatalf("Classify() class = %q, want %q", got.Class, ClassPlanWeek)
	}
	if !got.IsPlan() {
		t.Fatal("IsPlan() = false, want true")
	}
}

func TestRecommendedRulesClassifyMasterPlanByFilename(t *testing.T) {
	classifier, err := NewClassifier(RecommendedRules())
	if err != nil {
		t.Fatalf("NewClassifier() error = %v", err)
	}

	got := classifier.Classify("03-\u6570\u636e\u5e93/\u6570\u636e\u5e93\u590d\u4e60\u8ba1\u5212.md")
	if got.Class != ClassPlanMaster {
		t.Fatalf("Classify() class = %q, want %q", got.Class, ClassPlanMaster)
	}
}

func TestRecommendedRulesDoNotTreatManagedCoreDocsAsPlansByPathOnly(t *testing.T) {
	classifier, err := NewClassifier(RecommendedRules())
	if err != nil {
		t.Fatalf("NewClassifier() error = %v", err)
	}

	got := classifier.Classify("0-\u6392\u671f/00-\u7cfb\u7edf/\u4eba\u7269\u80cc\u666f\u753b\u50cf.md")
	if got.Class != ClassUnknown {
		t.Fatalf("Classify() class = %q, want %q", got.Class, ClassUnknown)
	}
}

func TestNewClassifierRejectsInvalidRules(t *testing.T) {
	// A rule must carry an id, a class, and at least one matcher, or
	// construction fails -- a misconfigured rule set must be caught up
	// front rather than silently classifying everything as Unknown.
	valid := Rule{ID: "r1", Class: ClassPlanWeek, FileNameContains: []string{"week"}}
	if _, err := NewClassifier([]Rule{valid}); err != nil {
		t.Fatalf("valid rule should construct, got %v", err)
	}
	cases := []struct {
		name    string
		rule    Rule
		wantErr error
	}{
		{"blank id", Rule{ID: "  ", Class: ClassPlanWeek, FileNameContains: []string{"week"}}, ErrRuleIDRequired},
		{"empty class", Rule{ID: "r", Class: "", FileNameContains: []string{"week"}}, ErrRuleClassRequired},
		{"no matchers", Rule{ID: "r", Class: ClassPlanWeek}, ErrRuleMatcherRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewClassifier([]Rule{tc.rule}); !errors.Is(err, tc.wantErr) {
				t.Fatalf("NewClassifier(%s) error = %v, want %v", tc.name, err, tc.wantErr)
			}
		})
	}
}

func TestNormalizePathHandlesSeparatorsAndDotForms(t *testing.T) {
	// normalizePath feeds every match, so a path must classify the same
	// regardless of how the caller spelled it: Windows backslashes become
	// forward slashes, "./" prefixes and a bare "." collapse to empty,
	// ".." segments are cleaned, and case folds. (Runs on a Windows host,
	// so backslash inputs are real.)
	cases := []struct {
		in   string
		want string
	}{
		{`Plans\Week.md`, "plans/week.md"},
		{"./Plans/Week.md", "plans/week.md"},
		{"a/../b/C.MD", "b/c.md"},
		{"   ", ""},
		{".", ""},
	}
	for _, tc := range cases {
		if got := normalizePath(tc.in); got != tc.want {
			t.Fatalf("normalizePath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
