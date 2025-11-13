// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

#include <lux_consensus.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <assert.h>

// ANSI color codes
#define COLOR_RESET   "\033[0m"
#define COLOR_RED     "\033[0;31m"
#define COLOR_GREEN   "\033[0;32m"
#define COLOR_YELLOW  "\033[1;33m"

// Test tracking
static int tests_passed = 0;
static int tests_failed = 0;

#define ASSERT_TEST(condition, test_name) do { \
    if (condition) { \
        printf("%s[PASS]%s %s\n", COLOR_GREEN, COLOR_RESET, test_name); \
        tests_passed++; \
    } else { \
        printf("%s[FAIL]%s %s\n", COLOR_RED, COLOR_RESET, test_name); \
        tests_failed++; \
        return 1; \
    } \
} while(0)

int main(void) {
    printf("\n%s=== Lux Consensus C SDK Tests (v1.22.0) ===%s\n", COLOR_YELLOW, COLOR_RESET);
    
    // Test 1: Library initialization
    printf("\n%s--- Test 1: Library Init/Cleanup ---%s\n", COLOR_YELLOW, COLOR_RESET);
    lux_error_t err = lux_consensus_init();
    ASSERT_TEST(err == LUX_SUCCESS, "Initialize library");
    
    err = lux_consensus_cleanup();
    ASSERT_TEST(err == LUX_SUCCESS, "Cleanup library");
    
    // Re-init for remaining tests
    lux_consensus_init();
    
    // Test 2: Create chain with default config
    printf("\n%s--- Test 2: Chain Creation ---%s\n", COLOR_YELLOW, COLOR_RESET);
    lux_chain_t* chain = lux_chain_new_default();
    ASSERT_TEST(chain != NULL, "Create chain with default config");
    
    // Test 3: Start chain
    printf("\n%s--- Test 3: Chain Lifecycle ---%s\n", COLOR_YELLOW, COLOR_RESET);
    err = lux_chain_start(chain);
    ASSERT_TEST(err == LUX_SUCCESS, "Start chain");
    
    // Test 4: Create and add block
    printf("\n%s--- Test 4: Block Operations ---%s\n", COLOR_YELLOW, COLOR_RESET);
    lux_block_t block;
    memset(&block, 0, sizeof(block));
    
    // Set block ID
    for (int i = 0; i < 32; i++) {
        block.id[i] = (uint8_t)(i + 1);
    }
    
    // Set genesis parent
    memset(block.parent_id, 0, 32);
    
    block.height = 1;
    block.timestamp = 1700000000;
    
    const char* test_data = "Test Block";
    block.data = (void*)test_data;
    block.data_size = strlen(test_data);
    
    err = lux_chain_add_block(chain, &block);
    ASSERT_TEST(err == LUX_SUCCESS, "Add block to chain");
    
    // Test 5: Create chain with custom config
    printf("\n%s--- Test 5: Custom Configuration ---%s\n", COLOR_YELLOW, COLOR_RESET);
    lux_config_t config = {
        .node_count = 5,
        .k = 3,
        .alpha = 3,
        .beta = 4
    };
    
    lux_chain_t* custom_chain = lux_chain_new(&config);
    ASSERT_TEST(custom_chain != NULL, "Create chain with custom config");
    
    err = lux_chain_start(custom_chain);
    ASSERT_TEST(err == LUX_SUCCESS, "Start custom chain");
    
    // Test 6: Cleanup
    printf("\n%s--- Test 6: Cleanup ---%s\n", COLOR_YELLOW, COLOR_RESET);
    lux_chain_stop(chain);
    lux_chain_destroy(chain);
    ASSERT_TEST(1, "Stop and destroy first chain");
    
    lux_chain_stop(custom_chain);
    lux_chain_destroy(custom_chain);
    ASSERT_TEST(1, "Stop and destroy custom chain");
    
    lux_consensus_cleanup();
    ASSERT_TEST(1, "Final cleanup");
    
    // Print summary
    printf("\n%s=== Test Results ===%s\n", COLOR_YELLOW, COLOR_RESET);
    printf("Passed: %s%d%s\n", COLOR_GREEN, tests_passed, COLOR_RESET);
    printf("Failed: %s%d%s\n", tests_failed > 0 ? COLOR_RED : COLOR_GREEN, 
           tests_failed, COLOR_RESET);
    
    if (tests_failed == 0) {
        printf("\n%s✅ All tests passed!%s\n\n", COLOR_GREEN, COLOR_RESET);
        return 0;
    } else {
        printf("\n%s❌ Some tests failed!%s\n\n", COLOR_RED, COLOR_RESET);
        return 1;
    }
}