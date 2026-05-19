package model

import (
	"testing"
	"time"
)

// TestNormalizeUsageDayBridgesUTCRecordsAndLocalQueries verifies the
// timezone-bridge contract that the three usage store backends rely on:
// a UTC RecordedAt and a local-time query for "today" must agree on
// the day bucket whenever they refer to the same wall-clock day in the
// user's local zone.
//
// Concrete scenario: a model call at 2026-05-17 16:30 UTC corresponds
// to 2026-05-18 00:30 in Asia/Shanghai (+08:00). A user who runs
// `lore usage` at 2026-05-18 00:30 local must see that call counted
// against "today". NormalizeUsageDay is the seam that achieves this by
// rebasing both inputs into time.Local before truncation.
func TestNormalizeUsageDayBridgesUTCRecordsAndLocalQueries(t *testing.T) {
	originalLocal := time.Local
	t.Cleanup(func() { time.Local = originalLocal })
	time.Local = time.FixedZone("CST", 8*3600)

	recordedUTC := time.Date(2026, 5, 17, 16, 30, 0, 0, time.UTC)
	queryLocal := time.Date(2026, 5, 18, 0, 30, 0, 0, time.Local)

	bucketed := NormalizeUsageDay(recordedUTC)
	queried := NormalizeUsageDay(queryLocal)
	if !bucketed.Equal(queried) {
		t.Fatalf("UTC record bucket %v != local query bucket %v; expected both at 2026-05-18 local midnight", bucketed, queried)
	}

	// The day-of-week match locks the intent: the bucket really is on
	// local 2026-05-18, not on 2026-05-17 (which is what plain
	// NormalizeDay on a UTC ts would yield).
	if got := bucketed.In(time.Local).Format("2006-01-02"); got != "2026-05-18" {
		t.Fatalf("bucket day = %q, want 2026-05-18", got)
	}

	// A query for the previous local day must NOT match the record.
	queryYesterday := time.Date(2026, 5, 17, 12, 0, 0, 0, time.Local)
	if NormalizeUsageDay(queryYesterday).Equal(bucketed) {
		t.Fatal("yesterday-local query should not collide with today-local bucket")
	}
}
