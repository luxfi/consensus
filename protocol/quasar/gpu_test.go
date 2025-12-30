//go:build cgo
// +build cgo

package quasar

import (
	"testing"
)

func TestGPUOrchestrator_Initialization(t *testing.T) {
	gpu, err := NewGPUOrchestrator(DefaultGPUConfig())
	if err != nil {
		t.Fatalf("Failed to create GPU orchestrator: %v", err)
	}

	stats := gpu.Stats()
	t.Logf("GPU Orchestrator Stats: enabled=%v, backend=%s", stats.Enabled, stats.Backend)
}

func TestGPUOrchestrator_GlobalInstance(t *testing.T) {
	gpu, err := GetGPUOrchestrator()
	if err != nil {
		t.Fatalf("Failed to get global GPU orchestrator: %v", err)
	}

	if gpu == nil {
		t.Fatal("Global GPU orchestrator is nil")
	}

	// Should return same instance
	gpu2, _ := GetGPUOrchestrator()
	if gpu != gpu2 {
		t.Error("GetGPUOrchestrator should return same instance")
	}
}

func TestGPUOrchestrator_BLSOperations(t *testing.T) {
	gpu, err := NewGPUOrchestrator(DefaultGPUConfig())
	if err != nil {
		t.Fatalf("Failed to create GPU orchestrator: %v", err)
	}

	if !gpu.IsGPUEnabled() {
		t.Log("GPU not enabled, testing with CPU fallback")
	}

	// Test single hash
	data := []byte("test data for hashing")
	hash := gpu.SHA3_256(data)
	if len(hash) != 32 {
		t.Errorf("SHA3_256 hash length = %d, want 32", len(hash))
	}

	t.Logf("SHA3-256 hash: %x", hash)
}

func TestGPUOrchestrator_BatchHashing(t *testing.T) {
	gpu, err := NewGPUOrchestrator(DefaultGPUConfig())
	if err != nil {
		t.Fatalf("Failed to create GPU orchestrator: %v", err)
	}

	inputs := [][]byte{
		[]byte("input 1"),
		[]byte("input 2"),
		[]byte("input 3"),
		[]byte("input 4"),
	}

	outputs, err := gpu.BatchSHA3_256(inputs)
	if err != nil {
		t.Fatalf("BatchSHA3_256 failed: %v", err)
	}

	if len(outputs) != len(inputs) {
		t.Errorf("outputs count = %d, want %d", len(outputs), len(inputs))
	}

	for i, out := range outputs {
		if len(out) != 32 {
			t.Errorf("outputs[%d] length = %d, want 32", i, len(out))
		}
	}

	t.Logf("Batch hashed %d inputs", len(outputs))
}

func TestGPUOrchestrator_ThresholdContext(t *testing.T) {
	gpu, err := NewGPUOrchestrator(DefaultGPUConfig())
	if err != nil {
		t.Fatalf("Failed to create GPU orchestrator: %v", err)
	}

	threshold := uint32(3)
	total := uint32(5)

	ctx, err := gpu.NewThresholdContext(threshold, total)
	if err != nil {
		t.Fatalf("Failed to create threshold context: %v", err)
	}
	defer ctx.Close()

	// Generate shares
	shares, pk, err := ctx.Keygen(nil)
	if err != nil {
		t.Fatalf("Keygen failed: %v", err)
	}

	if uint32(len(shares)) != total {
		t.Errorf("shares count = %d, want %d", len(shares), total)
	}

	t.Logf("Generated %d shares, pk=%d bytes", len(shares), len(pk))
}

func TestGPUEnabled(t *testing.T) {
	enabled := GPUEnabled()
	t.Logf("GPU Enabled: %v", enabled)
}

func BenchmarkGPU_BatchSHA3_256(b *testing.B) {
	gpu, err := NewGPUOrchestrator(DefaultGPUConfig())
	if err != nil {
		b.Fatalf("Failed to create GPU orchestrator: %v", err)
	}

	inputs := make([][]byte, 100)
	for i := range inputs {
		inputs[i] = make([]byte, 1024)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = gpu.BatchSHA3_256(inputs)
	}
}

func BenchmarkGPU_SHA3_256(b *testing.B) {
	gpu, err := NewGPUOrchestrator(DefaultGPUConfig())
	if err != nil {
		b.Fatalf("Failed to create GPU orchestrator: %v", err)
	}

	data := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gpu.SHA3_256(data)
	}
}

// =============================================================================
// Ringtail GPU Tests
// =============================================================================

func TestGPUOrchestrator_RingtailEnabled(t *testing.T) {
	gpu, err := NewGPUOrchestrator(DefaultGPUConfig())
	if err != nil {
		t.Fatalf("Failed to create GPU orchestrator: %v", err)
	}
	defer gpu.Close()

	enabled := gpu.RingtailGPUEnabled()
	t.Logf("Ringtail GPU Enabled: %v", enabled)
}

func TestGPUOrchestrator_RingtailNTT(t *testing.T) {
	gpu, err := NewGPUOrchestrator(DefaultGPUConfig())
	if err != nil {
		t.Fatalf("Failed to create GPU orchestrator: %v", err)
	}
	defer gpu.Close()

	// Create test polynomials (N=256)
	polys := make([][]uint64, 2)
	for i := range polys {
		polys[i] = make([]uint64, 256)
		for j := 0; j < 256; j++ {
			polys[i][j] = uint64(j+i*256) % 8380417
		}
	}

	// Forward NTT
	nttPolys, err := gpu.RingtailNTTForward(polys)
	if err != nil {
		t.Fatalf("RingtailNTTForward failed: %v", err)
	}
	if len(nttPolys) != 2 {
		t.Errorf("expected 2 polynomials, got %d", len(nttPolys))
	}

	// Inverse NTT
	invPolys, err := gpu.RingtailNTTInverse(nttPolys)
	if err != nil {
		t.Fatalf("RingtailNTTInverse failed: %v", err)
	}
	if len(invPolys) != 2 {
		t.Errorf("expected 2 polynomials, got %d", len(invPolys))
	}

	t.Log("Ringtail NTT forward/inverse operations successful")
}

func TestGPUOrchestrator_RingtailPolyMul(t *testing.T) {
	gpu, err := NewGPUOrchestrator(DefaultGPUConfig())
	if err != nil {
		t.Fatalf("Failed to create GPU orchestrator: %v", err)
	}
	defer gpu.Close()

	n := 256
	aPolys := make([][]uint64, 2)
	bPolys := make([][]uint64, 2)
	for i := range aPolys {
		aPolys[i] = make([]uint64, n)
		bPolys[i] = make([]uint64, n)
		for j := 0; j < n; j++ {
			aPolys[i][j] = uint64(j+i) % 8380417
			bPolys[i][j] = uint64(n-j+i) % 8380417
		}
	}

	products, err := gpu.RingtailPolyMul(aPolys, bPolys)
	if err != nil {
		t.Fatalf("RingtailPolyMul failed: %v", err)
	}
	if len(products) != 2 {
		t.Errorf("expected 2 products, got %d", len(products))
	}

	t.Log("Ringtail polynomial multiplication successful")
}

func TestGPUOrchestrator_RingtailPolyOperations(t *testing.T) {
	gpu, err := NewGPUOrchestrator(DefaultGPUConfig())
	if err != nil {
		t.Fatalf("Failed to create GPU orchestrator: %v", err)
	}
	defer gpu.Close()

	n := 256
	a := make([]uint64, n)
	b := make([]uint64, n)
	for i := 0; i < n; i++ {
		a[i] = uint64(i) % 8380417
		b[i] = uint64(n-i) % 8380417
	}

	// Test PolyAdd
	sum, err := gpu.RingtailPolyAdd(a, b)
	if err != nil {
		t.Fatalf("RingtailPolyAdd failed: %v", err)
	}
	if len(sum) != n {
		t.Errorf("expected sum length %d, got %d", n, len(sum))
	}

	// Test PolySub
	diff, err := gpu.RingtailPolySub(a, b)
	if err != nil {
		t.Fatalf("RingtailPolySub failed: %v", err)
	}
	if len(diff) != n {
		t.Errorf("expected diff length %d, got %d", n, len(diff))
	}

	t.Log("Ringtail polynomial add/sub operations successful")
}

func TestGPUOrchestrator_RingtailSampling(t *testing.T) {
	gpu, err := NewGPUOrchestrator(DefaultGPUConfig())
	if err != nil {
		t.Fatalf("Failed to create GPU orchestrator: %v", err)
	}
	defer gpu.Close()

	seed := []byte("test seed for ringtail sampling!")

	// Test SampleUniform
	uniform, err := gpu.RingtailSampleUniform(seed)
	if err != nil {
		t.Fatalf("RingtailSampleUniform failed: %v", err)
	}
	if len(uniform) != 256 {
		t.Errorf("expected uniform length 256, got %d", len(uniform))
	}

	// Test SampleGaussian
	gaussian, err := gpu.RingtailSampleGaussian(3.2, seed)
	if err != nil {
		t.Fatalf("RingtailSampleGaussian failed: %v", err)
	}
	if len(gaussian) != 256 {
		t.Errorf("expected gaussian length 256, got %d", len(gaussian))
	}

	// Test SampleTernary
	ternary, err := gpu.RingtailSampleTernary(0.5, seed)
	if err != nil {
		t.Fatalf("RingtailSampleTernary failed: %v", err)
	}
	if len(ternary) != 256 {
		t.Errorf("expected ternary length 256, got %d", len(ternary))
	}

	t.Log("Ringtail sampling operations successful")
}

func TestGPUOrchestrator_RingtailMatrixVectorMul(t *testing.T) {
	gpu, err := NewGPUOrchestrator(DefaultGPUConfig())
	if err != nil {
		t.Fatalf("Failed to create GPU orchestrator: %v", err)
	}
	defer gpu.Close()

	n := 256

	// Create a 2x3 matrix and 3-element vector
	matrix := make([][][]uint64, 2)
	for i := range matrix {
		matrix[i] = make([][]uint64, 3)
		for j := range matrix[i] {
			matrix[i][j] = make([]uint64, n)
			for k := 0; k < n; k++ {
				matrix[i][j][k] = uint64(i*100+j*10+k) % 8380417
			}
		}
	}

	vector := make([][]uint64, 3)
	for i := range vector {
		vector[i] = make([]uint64, n)
		for j := 0; j < n; j++ {
			vector[i][j] = uint64(i*10+j) % 8380417
		}
	}

	result, err := gpu.RingtailMatrixVectorMul(matrix, vector)
	if err != nil {
		t.Fatalf("RingtailMatrixVectorMul failed: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected result length 2, got %d", len(result))
	}

	t.Log("Ringtail matrix-vector multiplication successful")
}

func BenchmarkGPU_RingtailNTT(b *testing.B) {
	gpu, err := NewGPUOrchestrator(DefaultGPUConfig())
	if err != nil {
		b.Fatalf("Failed to create GPU orchestrator: %v", err)
	}
	defer gpu.Close()

	polys := make([][]uint64, 16)
	for i := range polys {
		polys[i] = make([]uint64, 256)
		for j := 0; j < 256; j++ {
			polys[i][j] = uint64(j) % 8380417
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = gpu.RingtailNTTForward(polys)
	}
}

func BenchmarkGPU_RingtailPolyMul(b *testing.B) {
	gpu, err := NewGPUOrchestrator(DefaultGPUConfig())
	if err != nil {
		b.Fatalf("Failed to create GPU orchestrator: %v", err)
	}
	defer gpu.Close()

	n := 256
	aPolys := make([][]uint64, 16)
	bPolys := make([][]uint64, 16)
	for i := range aPolys {
		aPolys[i] = make([]uint64, n)
		bPolys[i] = make([]uint64, n)
		for j := 0; j < n; j++ {
			aPolys[i][j] = uint64(j) % 8380417
			bPolys[i][j] = uint64(n-j) % 8380417
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = gpu.RingtailPolyMul(aPolys, bPolys)
	}
}
