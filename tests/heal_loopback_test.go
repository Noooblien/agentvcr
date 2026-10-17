package tests

import (
	"strings"
	"testing"

	"github.com/Noooblien/agentvcr/pkg/heal"
)

func TestLoopback_DiagnosticsExtraction(t *testing.T) {
	interceptor := heal.NewConsoleInterceptor()

	// Ingest sample logs
	interceptor.IngestLog(
		heal.LogLevelError,
		"console",
		"Uncaught ReferenceError: React is not defined",
		"    at App (src/App.tsx:12:4)\n    at renderWithHooks (node_modules/react-dom/index.js:50:2)",
	)

	interceptor.IngestLog(
		heal.LogLevelError,
		"vite_hmr",
		"Failed to resolve import \"./components/Button\" from \"src/Header.tsx\". Does the file exist?",
		"    at src/Header.tsx:4:18",
	)

	interceptor.IngestLog(
		heal.LogLevelError,
		"unhandled_rejection",
		"TypeError: Cannot read properties of undefined (reading 'map')",
		"    at UserList (src/UserList.tsx:18:22)",
	)

	diags := interceptor.ExtractDiagnostics()
	if len(diags) != 3 {
		t.Fatalf("Expected 3 diagnostics, got %d", len(diags))
	}

	// Verify ReferenceError
	if diags[0].Category != "reference_error" {
		t.Errorf("Expected category 'reference_error', got %s", diags[0].Category)
	}
	if diags[0].TargetFile != "src/App.tsx" || diags[0].TargetLine != 12 {
		t.Errorf("Expected src/App.tsx:12, got %s:%d", diags[0].TargetFile, diags[0].TargetLine)
	}

	// Verify Bundler Build Error
	if diags[1].Category != "bundler_build_error" {
		t.Errorf("Expected category 'bundler_build_error', got %s", diags[1].Category)
	}
	if diags[1].TargetFile != "src/Header.tsx" || diags[1].TargetLine != 4 {
		t.Errorf("Expected src/Header.tsx:4, got %s:%d", diags[1].TargetFile, diags[1].TargetLine)
	}

	// Verify Null Pointer
	if diags[2].Category != "null_pointer_exception" {
		t.Errorf("Expected category 'null_pointer_exception', got %s", diags[2].Category)
	}
	if diags[2].TargetFile != "src/UserList.tsx" || diags[2].TargetLine != 18 {
		t.Errorf("Expected src/UserList.tsx:18, got %s:%d", diags[2].TargetFile, diags[2].TargetLine)
	}
}

func TestLoopback_HotPatchSynthesis(t *testing.T) {
	healer := heal.NewLoopbackHealer(3)

	diags := []heal.BrowserDiagnostic{
		{
			ID:           "DIAG-001",
			Category:     "reference_error",
			RawMessage:   "React is not defined",
			TargetFile:   "src/App.tsx",
			TargetLine:   5,
			SuggestedFix: "Import React",
		},
		{
			ID:           "DIAG-002",
			Category:     "null_pointer_exception",
			RawMessage:   "Cannot read properties of undefined (reading 'map')",
			TargetFile:   "src/UserList.tsx",
			TargetLine:   3,
			SuggestedFix: "Apply optional chaining",
		},
	}

	fileMap := map[string]string{
		"src/App.tsx": "export function App() {\n  return <div>Hello</div>;\n}",
		"src/UserList.tsx": "export function UserList({ users }) {\n  return (\n    <ul>{users.map(u => <li>{u.name}</li>)}</ul>\n  );\n}",
	}

	patches := healer.SynthesizeHotPatches(diags, fileMap)
	if len(patches) != 2 {
		t.Fatalf("Expected 2 patches synthesized, got %d", len(patches))
	}

	// Verify App.tsx patch
	patch1 := patches[0]
	if !strings.Contains(patch1.PatchedCode, "import React from 'react';") {
		t.Errorf("Expected patch1 to contain React import, got:\n%s", patch1.PatchedCode)
	}

	// Verify UserList.tsx patch
	patch2 := patches[1]
	if !strings.Contains(patch2.PatchedCode, "users?.map(") {
		t.Errorf("Expected patch2 to contain optional chaining 'users?.map(', got:\n%s", patch2.PatchedCode)
	}
}

func TestLoopback_ExecuteLoopbackToZeroErrors(t *testing.T) {
	healer := heal.NewLoopbackHealer(4)

	initialLogs := []heal.ConsoleLog{
		{
			Level:   heal.LogLevelError,
			Source:  "console",
			Message: "Uncaught ReferenceError: React is not defined",
			Stack:   "    at App (src/App.tsx:3:1)",
		},
	}

	files := map[string]string{
		"src/App.tsx": "export function App() {\n  return <h1>Live</h1>;\n}",
	}

	// Simulator removes the error once React is imported
	sim := func(currentFiles map[string]string) []heal.ConsoleLog {
		if strings.Contains(currentFiles["src/App.tsx"], "import React") {
			return []heal.ConsoleLog{} // 0 errors!
		}
		return initialLogs
	}

	session := healer.ExecuteLoopback(initialLogs, files, sim)
	if !session.ZeroErrorsAchieved {
		t.Errorf("Expected zero errors achieved, but ZeroErrorsAchieved=false")
	}
	if session.FinalErrorCount != 0 {
		t.Errorf("Expected final error count 0, got %d", session.FinalErrorCount)
	}
	if len(session.SynthesizedPatches) == 0 {
		t.Errorf("Expected synthesized patches")
	}
}

func TestLoopback_ScorecardRendering(t *testing.T) {
	session := &heal.LoopbackSession{
		SessionID:         "loopback_test_123",
		MaxIterations:     5,
		IterationsRun:     2,
		InitialErrorCount: 2,
		FinalErrorCount:   0,
		ZeroErrorsAchieved: true,
		DurationMs:        14.5,
		Diagnostics: []heal.BrowserDiagnostic{
			{
				ID:           "DIAG-001",
				Category:     "reference_error",
				RawMessage:   "React is not defined",
				TargetFile:   "src/App.tsx",
				TargetLine:   3,
				SuggestedFix: "Import React",
			},
		},
		SynthesizedPatches: []heal.HotPatchPlan{
			{
				PatchID:     "HOTPATCH-001",
				TargetFile:  "src/App.tsx",
				Confidence:  0.95,
				DiffSnippet: "+ import React from 'react';",
				Explanation: "Injected missing React import",
			},
		},
	}

	card := heal.RenderLoopbackScorecard(session)
	if !strings.Contains(card, "IN-BROWSER CONSOLE LOOPBACK & ZERO-ERROR HOT-PATCH ENGINE") {
		t.Errorf("Expected banner in scorecard")
	}
	if !strings.Contains(card, "ZERO ERRORS ACHIEVED") {
		t.Errorf("Expected ZERO ERRORS ACHIEVED verdict badge")
	}
	if !strings.Contains(card, "HOTPATCH-001") {
		t.Errorf("Expected HOTPATCH-001 in scorecard")
	}
}
