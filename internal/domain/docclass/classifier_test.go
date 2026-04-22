package docclass

import "testing"

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
