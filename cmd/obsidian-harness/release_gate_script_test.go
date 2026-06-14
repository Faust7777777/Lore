package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseGateHasV1BetaAcceptanceSwitch(t *testing.T) {
	scriptPath := filepath.Join("..", "..", "scripts", "release-gate.ps1")
	raw, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", scriptPath, err)
	}
	script := string(raw)

	required := []string{
		"[switch]$V1BetaAcceptance",
		"if ($V1BetaAcceptance)",
		"V1 beta MCP external boundary",
		"Test(MCPV1ExposesOnlyReadAndProposalTools|ExternalMCPDoesNotExposeDirectWrites)$",
		"V1 beta governed markdown intake",
		"TestRuntimeSmokeExternalMCPGovernedMarkdownNoteIntake$",
		"V1 beta persona memory candidate acceptance",
		"TestRunPersonaMemoryCandidateAcceptanceScaffold$",
		"V1 beta external transcript import",
		"TestRuntimeImportExternalTranscriptJSONL",
		"TestRunImportExternalJSONL",
		"V1 beta operator queue and inbox",
		"TestRuntimeOperatorQueue",
		"TestRunInboxJSONCommand",
		"Test(RenderOperatorQueue|EmitOperatorQueueJSON|OperatorQueueNudge)$",
		"V1 beta usage purpose/model breakdown",
		"Test(SummarizeUsageByModel(SplitsSamePurposeAcrossModels|DoesNotMixAcrossPurposes|FallsBackToUnknown)|SummarizeUsageBreakdownEmptyPurposeFoldsIntoChat)$",
		"Test(EmitUsageJSONIncludesByModelBreakdown|RenderUsageReportShowsModelDetailUnderPurpose|RenderUsageReportShowsCrossPurposeModelTotals)$",
	}
	for _, want := range required {
		if !strings.Contains(script, want) {
			t.Fatalf("release-gate.ps1 missing %q", want)
		}
	}
}
