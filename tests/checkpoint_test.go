package tests

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/checkpoint"
)

func TestCheckpoint_Storage_SaveAndLoad(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agentvcr-ckpt-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	storage, err := checkpoint.NewStorageEngine(tempDir, 6)
	if err != nil {
		t.Fatalf("Failed to initialize storage engine: %v", err)
	}

	image := &checkpoint.SessionImage{
		ImageID:   "img-test-1",
		SessionID: "sess-100",
		AgentID:   "agent-007",
		StepIndex: 5,
		State:     checkpoint.StateHibernated,
		CreatedAt: time.Now(),
		ContextFrames: []checkpoint.AgentContextFrame{
			{Role: "user", Content: "Plan the migration"},
			{Role: "assistant", Content: "Evaluating dependency tree"},
		},
		WorkingMemory: map[string]interface{}{
			"db_target": "cockroachdb",
			"shards":    16,
		},
		PRNGSeed: 987654321,
	}

	// 1. Save Image
	if err := storage.Save(image); err != nil {
		t.Fatalf("Storage Save failed: %v", err)
	}

	if image.CompressedSize <= 0 || image.UncompressedSize <= 0 {
		t.Fatalf("Expected valid image sizes, got uncompressed=%d, compressed=%d",
			image.UncompressedSize, image.CompressedSize)
	}
	if image.SHA256Hash == "" {
		t.Fatalf("Expected non-empty SHA-256 hash")
	}

	// 2. Load Image and verify integrity
	loaded, err := storage.Load("img-test-1")
	if err != nil {
		t.Fatalf("Storage Load failed: %v", err)
	}

	if loaded.ImageID != image.ImageID || loaded.SessionID != image.SessionID {
		t.Fatalf("Loaded image metadata mismatch")
	}
	if len(loaded.ContextFrames) != 2 {
		t.Fatalf("Expected 2 context frames, got %d", len(loaded.ContextFrames))
	}
	if loaded.PRNGSeed != 987654321 {
		t.Fatalf("Expected PRNGSeed 987654321, got %d", loaded.PRNGSeed)
	}
}

func TestCheckpoint_DeltaCompressor_ComputeAndReconstruct(t *testing.T) {
	compressor := checkpoint.NewDeltaCompressor()

	base := &checkpoint.SessionImage{
		ImageID:   "base-img",
		SessionID: "sess-200",
		AgentID:   "coder",
		StepIndex: 1,
		CreatedAt: time.Now(),
		ContextFrames: []checkpoint.AgentContextFrame{
			{Role: "system", Content: "Initialize compiler"},
			{Role: "user", Content: "Build AST parser"},
		},
		WorkingMemory: map[string]interface{}{
			"target_lang": "go",
			"optimization": "O2",
		},
		PRNGSeed: 1001,
	}

	current := &checkpoint.SessionImage{
		ImageID:   "step-2-img",
		SessionID: "sess-200",
		AgentID:   "coder",
		StepIndex: 2,
		CreatedAt: time.Now(),
		ContextFrames: []checkpoint.AgentContextFrame{
			{Role: "system", Content: "Initialize compiler"},
			{Role: "user", Content: "Build AST parser"},
			{Role: "assistant", Action: "Generated lexer tokens"},
		},
		WorkingMemory: map[string]interface{}{
			"target_lang":  "go",
			"optimization": "O3", // Mutated
			"token_count":  150,  // Added
		},
		ToolCursors: []checkpoint.ToolCursorState{
			{ToolName: "lexer", InvocationID: "inv-1", ProgressPercent: 100.0},
		},
		PRNGSeed: 1002,
	}

	// 1. Compute incremental delta
	delta := compressor.ComputeDelta(base, current)
	if delta.DeltaBaseID != "base-img" {
		t.Fatalf("Expected DeltaBaseID 'base-img', got: %s", delta.DeltaBaseID)
	}
	if len(delta.ContextFrames) != 1 {
		t.Fatalf("Delta should only contain the 1 newly appended frame, got %d", len(delta.ContextFrames))
	}
	if len(delta.WorkingMemory) != 2 { // "optimization" and "token_count"
		t.Fatalf("Delta working memory should only contain mutations/additions, got: %v", delta.WorkingMemory)
	}

	// 2. Reconstruct from base + delta
	reconstructed, err := compressor.Reconstruct(base, delta)
	if err != nil {
		t.Fatalf("Reconstruction failed: %v", err)
	}

	if len(reconstructed.ContextFrames) != 3 {
		t.Fatalf("Expected reconstructed context frames to have 3 frames, got %d", len(reconstructed.ContextFrames))
	}
	if reconstructed.WorkingMemory["optimization"] != "O3" {
		t.Fatalf("Expected mutated value O3, got: %v", reconstructed.WorkingMemory["optimization"])
	}
	if reconstructed.WorkingMemory["target_lang"] != "go" {
		t.Fatalf("Expected preserved base value 'go', got: %v", reconstructed.WorkingMemory["target_lang"])
	}
	if reconstructed.WorkingMemory["token_count"] != 150 {
		t.Fatalf("Expected added value 150, got: %v", reconstructed.WorkingMemory["token_count"])
	}
}

func TestCheckpoint_Manager_HibernateAndRestore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agentvcr-mgr-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := checkpoint.DefaultHibernationConfig(tempDir)
	manager, err := checkpoint.NewCheckpointManager("test-system", cfg)
	if err != nil {
		t.Fatalf("Failed to create checkpoint manager: %v", err)
	}

	// Step 1: Base image
	img1 := &checkpoint.SessionImage{
		ImageID:   "img-step-1",
		SessionID: "sess-mgr",
		AgentID:   "agent-x",
		StepIndex: 1,
		ContextFrames: []checkpoint.AgentContextFrame{
			{Role: "user", Content: "Start long task"},
		},
		WorkingMemory: map[string]interface{}{"status": "started"},
	}
	if err := manager.Hibernate(img1); err != nil {
		t.Fatalf("Failed to hibernate step 1: %v", err)
	}

	// Step 2: Incremental delta
	img2 := &checkpoint.SessionImage{
		ImageID:   "img-step-2",
		SessionID: "sess-mgr",
		AgentID:   "agent-x",
		StepIndex: 2,
		ContextFrames: []checkpoint.AgentContextFrame{
			{Role: "user", Content: "Start long task"},
			{Role: "assistant", Content: "Step 1 complete"},
		},
		WorkingMemory: map[string]interface{}{"status": "in_progress", "pct": 50},
	}
	if err := manager.Hibernate(img2); err != nil {
		t.Fatalf("Failed to hibernate step 2: %v", err)
	}

	// Restore Step 2 (requires delta reconstruction)
	restored, err := manager.Restore("img-step-2")
	if err != nil {
		t.Fatalf("Failed to restore step 2: %v", err)
	}

	if restored.State != checkpoint.StateRestored {
		t.Fatalf("Expected StateRestored, got: %s", restored.State)
	}
	if len(restored.ContextFrames) != 2 {
		t.Fatalf("Expected 2 context frames restored, got %d", len(restored.ContextFrames))
	}
	if fmt.Sprintf("%v", restored.WorkingMemory["pct"]) != "50" {
		t.Fatalf("Expected working memory pct=50, got: %v", restored.WorkingMemory["pct"])
	}

	// Verify Audit report
	audit := manager.Audit()
	if audit.TotalImages != 2 {
		t.Fatalf("Expected 2 audit images, got %d", audit.TotalImages)
	}
	if !audit.IntegrityVerified {
		t.Fatalf("Audit integrity verification should be true")
	}
	if audit.CompressionRatio <= 0 {
		t.Fatalf("Expected positive compression ratio, got: %.2f", audit.CompressionRatio)
	}
}

func TestCheckpoint_IntegrityViolation_Detection(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "agentvcr-corrupt-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	storage, err := checkpoint.NewStorageEngine(tempDir, 6)
	if err != nil {
		t.Fatalf("Failed to init storage engine: %v", err)
	}

	img := &checkpoint.SessionImage{
		ImageID:   "tamper-target",
		SessionID: "sess-t",
		ContextFrames: []checkpoint.AgentContextFrame{
			{Role: "user", Content: "Valid secret"},
		},
	}
	_ = storage.Save(img)

	// Tamper: manually overwrite file with altered data
	filePath := tempDir + "/tamper-target.ckpt.gz"
	_ = os.WriteFile(filePath, []byte("corrupted invalid gzip stream"), 0644)

	// Load should catch error
	_, err = storage.Load("tamper-target")
	if err == nil {
		t.Fatalf("Expected decompression error on tampered file")
	}
}

func TestCheckpoint_ScorecardAndMermaid(t *testing.T) {
	report := &checkpoint.CheckpointAuditReport{
		SystemID:              "test-system",
		Timestamp:             time.Now(),
		TotalImages:           2,
		TotalHibernated:       1,
		TotalRestored:         1,
		TotalRawBytes:         2048,
		TotalCompressedBytes:  512,
		CompressionRatio:      0.75,
		IntegrityVerified:     true,
		Images: []*checkpoint.SessionImage{
			{
				ImageID:          "ckpt-1",
				State:            checkpoint.StateHibernated,
				StepIndex:        5,
				UncompressedSize: 1024,
				CompressedSize:   256,
				SHA256Hash:       "abcd1234ef567890",
			},
		},
	}

	scorecard := checkpoint.RenderCheckpointScorecard(report)
	if !strings.Contains(scorecard, "AGENTVCR // SESSION CHECKPOINT") {
		t.Fatalf("Scorecard missing header: %s", scorecard)
	}
	if !strings.Contains(scorecard, "75.0% space saved") {
		t.Fatalf("Scorecard missing compression ratio: %s", scorecard)
	}

	mermaid := checkpoint.RenderCheckpointMermaid(report)
	if !strings.Contains(mermaid, "graph TD") || !strings.Contains(mermaid, "CKPT_ckpt_1") {
		t.Fatalf("Mermaid diagram missing checkpoint node: %s", mermaid)
	}
}
