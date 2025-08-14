// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInitializeRuntime(t *testing.T) {
	tests := []struct {
		name    string
		network string
		wantErr bool
	}{
		{
			name:    "mainnet",
			network: "mainnet",
			wantErr: false,
		},
		{
			name:    "testnet",
			network: "testnet",
			wantErr: false,
		},
		{
			name:    "test",
			network: "test",
			wantErr: false,
		},
		{
			name:    "local",
			network: "local",
			wantErr: false,
		},
		{
			name:    "invalid",
			network: "invalid-network",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)
			
			// Reset initialized state
			runtimeMu.Lock()
			initialized = false
			runtimeMu.Unlock()
			
			err := InitializeRuntime(tt.network)
			if tt.wantErr {
				require.Error(err)
			} else {
				require.NoError(err)
				
				// Verify initialization
				params := GetRuntime()
				require.NotNil(params)
				require.Greater(params.K, 0)
			}
		})
	}
}

func TestGetRuntime(t *testing.T) {
	require := require.New(t)

	// Reset state
	runtimeMu.Lock()
	initialized = false
	runtimeMu.Unlock()

	// Test uninitialized state - should return TestParameters
	params := GetRuntime()
	require.Equal(TestParameters, params)

	// Initialize with mainnet
	err := InitializeRuntime("mainnet")
	require.NoError(err)

	// Should now return mainnet parameters
	params = GetRuntime()
	require.Equal(MainnetParameters, params)
}

func TestSetRuntime(t *testing.T) {
	require := require.New(t)

	customParams := Parameters{
		K:                     30,
		AlphaPreference:       20,
		AlphaConfidence:       25,
		Beta:                  10,
		ConcurrentPolls:    5,
		OptimalProcessing:     15,
		MaxOutstandingItems:   200,
		MaxItemProcessingTime: 10 * time.Second,
		MinRoundInterval:      200 * time.Millisecond,
	}

	SetRuntime(customParams)

	params := GetRuntime()
	require.Equal(customParams, params)
}

func TestUpdateRuntimeParameter(t *testing.T) {
	require := require.New(t)

	// Currently disabled, should return error
	err := UpdateRuntimeParameter("K", 50)
	require.Error(err)
	require.Contains(err.Error(), "temporarily disabled")
}

func TestLoadRuntimeFromFile(t *testing.T) {
	require := require.New(t)

	// Create temporary directory
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test-config.json")

	// Create test config
	testConfig := Config{
		K:                     25,
		AlphaPreference:       15,
		AlphaConfidence:       20,
		Beta:                  8,
		ConcurrentPolls:    4,
		OptimalProcessing:     12,
		MaxOutstandingItems:   150,
		MaxItemProcessingTime: 7 * time.Second,
		MinRoundInterval:      150 * time.Millisecond,
	}

	// Write config to file
	data, err := json.MarshalIndent(testConfig, "", "  ")
	require.NoError(err)
	err = os.WriteFile(configPath, data, 0644)
	require.NoError(err)

	// Load from file
	err = LoadRuntimeFromFile(configPath)
	require.NoError(err)

	// Verify loaded parameters
	params := GetRuntime()
	require.Equal(testConfig.K, params.K)
	require.Equal(testConfig.AlphaPreference, params.AlphaPreference)
	require.Equal(testConfig.AlphaConfidence, params.AlphaConfidence)
	require.Equal(testConfig.Beta, params.Beta)
	require.Equal(testConfig.ConcurrentPolls, params.ConcurrentPolls)
	require.Equal(testConfig.OptimalProcessing, params.OptimalProcessing)
	require.Equal(testConfig.MaxOutstandingItems, params.MaxOutstandingItems)
	require.Equal(testConfig.MaxItemProcessingTime, params.MaxItemProcessingTime)
	require.Equal(testConfig.MinRoundInterval, params.MinRoundInterval)
}

func TestLoadRuntimeFromFileErrors(t *testing.T) {
	require := require.New(t)

	// Test non-existent file
	err := LoadRuntimeFromFile("/non/existent/file.json")
	require.Error(err)
	require.Contains(err.Error(), "failed to read")

	// Test invalid JSON
	tempDir := t.TempDir()
	invalidPath := filepath.Join(tempDir, "invalid.json")
	err = os.WriteFile(invalidPath, []byte("invalid json"), 0644)
	require.NoError(err)

	err = LoadRuntimeFromFile(invalidPath)
	require.Error(err)
	require.Contains(err.Error(), "failed to parse")
}

func TestSaveRuntimeToFile(t *testing.T) {
	require := require.New(t)

	// Set custom runtime
	customParams := Parameters{
		K:                     35,
		AlphaPreference:       25,
		AlphaConfidence:       30,
		Beta:                  12,
		ConcurrentPolls:    6,
		OptimalProcessing:     20,
		MaxOutstandingItems:   300,
		MaxItemProcessingTime: 15 * time.Second,
		MinRoundInterval:      250 * time.Millisecond,
	}
	SetRuntime(customParams)

	// Save to file
	tempDir := t.TempDir()
	savePath := filepath.Join(tempDir, "saved-config.json")
	
	err := SaveRuntimeToFile(savePath)
	require.NoError(err)

	// Read and verify file contents
	data, err := os.ReadFile(savePath)
	require.NoError(err)

	var savedData map[string]interface{}
	err = json.Unmarshal(data, &savedData)
	require.NoError(err)

	require.Equal(float64(35), savedData["k"])
	require.Equal(float64(25), savedData["alphaPreference"])
	require.Equal(float64(30), savedData["alphaConfidence"])
	require.Equal(float64(12), savedData["beta"])
	require.Equal(float64(6), savedData["concurrentPolls"])
	require.Equal(float64(20), savedData["optimalProcessing"])
	require.Equal(float64(300), savedData["maxOutstandingItems"])
}

func TestGetRuntimeOverrides(t *testing.T) {
	require := require.New(t)

	// Currently overrides are empty since UpdateRuntimeParameter is disabled
	overrides := GetRuntimeOverrides()
	require.NotNil(overrides)
	require.Empty(overrides)
}

func TestToInt(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  int
		ok    bool
	}{
		{"int", 42, 42, true},
		{"int64", int64(100), 100, true},
		{"float64", float64(75.0), 75, true},
		{"string valid", "123", 123, true},
		{"string invalid", "abc", 0, false},
		{"other type", []int{1, 2, 3}, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)
			got, ok := toInt(tt.input)
			require.Equal(tt.ok, ok)
			if ok {
				require.Equal(tt.want, got)
			}
		})
	}
}

func TestToDuration(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  time.Duration
		ok    bool
	}{
		{"duration", 5 * time.Second, 5 * time.Second, true},
		{"string valid", "10s", 10 * time.Second, true},
		{"string invalid", "invalid", 0, false},
		{"int64", int64(1000000000), 1 * time.Second, true}, // nanoseconds
		{"other type", 42, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require := require.New(t)
			got, ok := toDuration(tt.input)
			require.Equal(tt.ok, ok)
			if ok {
				require.Equal(tt.want, got)
			}
		})
	}
}

func TestConcurrentRuntimeAccess(t *testing.T) {
	require := require.New(t)

	// Set initial parameters
	err := InitializeRuntime("testnet")
	require.NoError(err)

	// Concurrent reads and writes
	var wg sync.WaitGroup
	done := make(chan bool)

	// Multiple readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					params := GetRuntime()
					require.Greater(params.K, 0)
				}
			}
		}()
	}

	// Multiple writers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				select {
				case <-done:
					return
				default:
					params := Parameters{
						K:                     20 + idx,
						AlphaPreference:       10 + idx,
						AlphaConfidence:       15 + idx,
						Beta:                  5 + idx,
						ConcurrentPolls:    3,
						OptimalProcessing:     10,
						MaxOutstandingItems:   100,
						MaxItemProcessingTime: 5 * time.Second,
						MinRoundInterval:      100 * time.Millisecond,
					}
					SetRuntime(params)
				}
			}
		}(i)
	}

	// Let it run for a bit
	time.Sleep(100 * time.Millisecond)
	close(done)
	wg.Wait()

	// Should still have valid parameters
	finalParams := GetRuntime()
	require.Greater(finalParams.K, 0)
	require.Greater(finalParams.AlphaPreference, 0)
	require.Greater(finalParams.AlphaConfidence, 0)
	require.Greater(finalParams.Beta, 0)
}

func BenchmarkGetRuntime(b *testing.B) {
	_ = InitializeRuntime("mainnet")
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GetRuntime()
	}
}

func BenchmarkSetRuntime(b *testing.B) {
	params := MainnetParameters
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SetRuntime(params)
	}
}

func BenchmarkLoadRuntimeFromFile(b *testing.B) {
	// Create test config file
	tempDir := b.TempDir()
	configPath := filepath.Join(tempDir, "bench-config.json")
	
	testConfig := Config{
		K:                     21,
		AlphaPreference:       13,
		AlphaConfidence:       18,
		Beta:                  8,
		ConcurrentPolls:    4,
		OptimalProcessing:     10,
		MaxOutstandingItems:   100,
		MaxItemProcessingTime: 5 * time.Second,
		MinRoundInterval:      100 * time.Millisecond,
	}
	
	data, _ := json.MarshalIndent(testConfig, "", "  ")
	_ = os.WriteFile(configPath, data, 0644)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = LoadRuntimeFromFile(configPath)
	}
}