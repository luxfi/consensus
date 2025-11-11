// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// Extended test suite to verify actual consensus behavior

#include <lux_consensus.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <assert.h>

#define COLOR_RESET   "\033[0m"
#define COLOR_RED     "\033[0;31m"
#define COLOR_GREEN   "\033[0;32m"
#define COLOR_YELLOW  "\033[1;33m"
#define COLOR_BLUE    "\033[0;34m"

typedef struct {
    int passed;
    int failed;
    const char* current_test;
} test_state_t;

static test_state_t state = {0, 0, NULL};

#define TEST(name) do { \
    state.current_test = name; \
    printf("  Testing: %s... ", name); \
} while(0)

#define PASS() do { \
    printf("%s[PASS]%s\n", COLOR_GREEN, COLOR_RESET); \
    state.passed++; \
} while(0)

#define FAIL(msg) do { \
    printf("%s[FAIL]%s - %s\n", COLOR_RED, COLOR_RESET, msg); \
    state.failed++; \
} while(0)

// Test 1: Verify voting actually changes block state
int test_voting_changes_state() {
    printf("\n%s=== TEST: Voting Changes Block State ===%s\n", COLOR_YELLOW, COLOR_RESET);

    lux_consensus_init();

    // Create engine with low thresholds for easy testing
    lux_consensus_config_t config = {
        .k = 5,
        .alpha_preference = 2,
        .alpha_confidence = 2,
        .beta = 3,  // Need 3 votes to accept
        .concurrent_polls = 1,
        .optimal_processing = 1,
        .max_outstanding_items = 1024,
        .max_item_processing_time_ns = 2000000000,
        .engine_type = LUX_ENGINE_CHAIN
    };

    lux_consensus_engine_t* engine = NULL;
    lux_consensus_engine_create(&engine, &config);

    // Add a test block
    lux_block_t block = {0};
    memset(block.id, 0xAB, 32);
    memset(block.parent_id, 0, 32);
    block.height = 1;
    block.timestamp = time(NULL);
    lux_consensus_add_block(engine, &block);

    // Check initial state - should NOT be accepted
    bool is_accepted = false;
    lux_consensus_is_accepted(engine, block.id, &is_accepted);

    TEST("Block not accepted initially");
    if (!is_accepted) {
        PASS();
    } else {
        FAIL("Block shouldn't be accepted without votes");
    }

    // Add 2 votes (below threshold)
    for (int i = 0; i < 2; i++) {
        lux_vote_t vote = {0};
        memset(vote.voter_id, i+1, 32);
        memcpy(vote.block_id, block.id, 32);
        vote.is_preference = false;  // confidence vote
        lux_consensus_process_vote(engine, &vote);
    }

    lux_consensus_is_accepted(engine, block.id, &is_accepted);
    TEST("Block not accepted with 2 votes (below beta=3)");
    if (!is_accepted) {
        PASS();
    } else {
        FAIL("Block accepted too early");
    }

    // Add 1 more vote to reach threshold
    lux_vote_t vote = {0};
    memset(vote.voter_id, 0x99, 32);
    memcpy(vote.block_id, block.id, 32);
    vote.is_preference = false;
    lux_consensus_process_vote(engine, &vote);

    lux_consensus_is_accepted(engine, block.id, &is_accepted);
    TEST("Block accepted with 3 votes (reached beta)");
    if (is_accepted) {
        PASS();
    } else {
        FAIL("Block should be accepted after reaching threshold");
    }

    // Get stats to verify votes were counted
    lux_consensus_stats_t stats;
    lux_consensus_get_stats(engine, &stats);

    TEST("Stats show 3 votes processed");
    if (stats.votes_processed == 3) {
        PASS();
    } else {
        char msg[100];
        snprintf(msg, sizeof(msg), "Expected 3 votes, got %llu", stats.votes_processed);
        FAIL(msg);
    }

    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
    return 0;
}

// Test 2: Verify preference tracking works
int test_preference_tracking() {
    printf("\n%s=== TEST: Preference Tracking ===%s\n", COLOR_YELLOW, COLOR_RESET);

    lux_consensus_init();

    lux_consensus_config_t config = {
        .k = 5,
        .alpha_preference = 2,
        .alpha_confidence = 2,
        .beta = 3,
        .concurrent_polls = 1,
        .optimal_processing = 1,
        .max_outstanding_items = 1024,
        .max_item_processing_time_ns = 2000000000,
        .engine_type = LUX_ENGINE_DAG
    };

    lux_consensus_engine_t* engine = NULL;
    lux_consensus_engine_create(&engine, &config);

    // Add two competing blocks at same height
    lux_block_t block_a = {0};
    memset(block_a.id, 0xAA, 32);
    memset(block_a.parent_id, 0, 32);
    block_a.height = 1;
    block_a.timestamp = time(NULL);
    lux_consensus_add_block(engine, &block_a);

    lux_block_t block_b = {0};
    memset(block_b.id, 0xBB, 32);
    memset(block_b.parent_id, 0, 32);
    block_b.height = 1;
    block_b.timestamp = time(NULL) + 1;
    lux_consensus_add_block(engine, &block_b);

    // Vote for block A with preference votes
    for (int i = 0; i < 2; i++) {
        lux_vote_t vote = {0};
        memset(vote.voter_id, i+10, 32);
        memcpy(vote.block_id, block_a.id, 32);
        vote.is_preference = true;  // preference vote
        lux_consensus_process_vote(engine, &vote);
    }

    // Get current preference
    uint8_t pref[32];
    lux_consensus_get_preference(engine, pref);

    TEST("Preference updated after preference votes");
    // Check if preference changed from genesis (all zeros)
    bool changed = false;
    for (int i = 0; i < 32; i++) {
        if (pref[i] != 0) {
            changed = true;
            break;
        }
    }
    if (changed) {
        PASS();
    } else {
        FAIL("Preference didn't change from genesis");
    }

    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
    return 0;
}

// Test 3: Test different engine types have same API
int test_engine_types_api() {
    printf("\n%s=== TEST: All Engine Types Support Same API ===%s\n", COLOR_YELLOW, COLOR_RESET);

    lux_consensus_init();

    lux_engine_type_t types[] = {LUX_ENGINE_CHAIN, LUX_ENGINE_DAG, LUX_ENGINE_PQ};
    const char* names[] = {"Chain", "DAG", "PQ"};

    for (int t = 0; t < 3; t++) {
        lux_consensus_config_t config = {
            .k = 5,
            .alpha_preference = 2,
            .alpha_confidence = 2,
            .beta = 3,
            .concurrent_polls = 1,
            .optimal_processing = 1,
            .max_outstanding_items = 1024,
            .max_item_processing_time_ns = 2000000000,
            .engine_type = types[t]
        };

        lux_consensus_engine_t* engine = NULL;
        lux_error_t err = lux_consensus_engine_create(&engine, &config);

        char test_name[100];
        snprintf(test_name, sizeof(test_name), "%s engine created", names[t]);
        TEST(test_name);
        if (err == LUX_SUCCESS && engine != NULL) {
            PASS();
        } else {
            FAIL("Failed to create engine");
            continue;
        }

        // Test add block
        lux_block_t block = {0};
        block.id[0] = t;
        block.height = 1;
        block.timestamp = time(NULL);
        err = lux_consensus_add_block(engine, &block);

        snprintf(test_name, sizeof(test_name), "%s supports add_block", names[t]);
        TEST(test_name);
        if (err == LUX_SUCCESS) {
            PASS();
        } else {
            FAIL("add_block failed");
        }

        // Test process vote
        lux_vote_t vote = {0};
        vote.voter_id[0] = 1;
        vote.block_id[0] = t;
        err = lux_consensus_process_vote(engine, &vote);

        snprintf(test_name, sizeof(test_name), "%s supports process_vote", names[t]);
        TEST(test_name);
        if (err == LUX_SUCCESS) {
            PASS();
        } else {
            FAIL("process_vote failed");
        }

        // Test is_accepted
        bool accepted;
        err = lux_consensus_is_accepted(engine, block.id, &accepted);

        snprintf(test_name, sizeof(test_name), "%s supports is_accepted", names[t]);
        TEST(test_name);
        if (err == LUX_SUCCESS) {
            PASS();
        } else {
            FAIL("is_accepted failed");
        }

        // Test get_preference
        uint8_t pref[32];
        err = lux_consensus_get_preference(engine, pref);

        snprintf(test_name, sizeof(test_name), "%s supports get_preference", names[t]);
        TEST(test_name);
        if (err == LUX_SUCCESS) {
            PASS();
        } else {
            FAIL("get_preference failed");
        }

        lux_consensus_engine_destroy(engine);
    }

    lux_consensus_cleanup();
    return 0;
}

// Test 4: Verify block hierarchy tracking
int test_block_hierarchy() {
    printf("\n%s=== TEST: Block Parent-Child Relationships ===%s\n", COLOR_YELLOW, COLOR_RESET);

    lux_consensus_init();

    lux_consensus_config_t config = {
        .k = 5,
        .alpha_preference = 1,
        .alpha_confidence = 1,
        .beta = 1,
        .concurrent_polls = 1,
        .optimal_processing = 1,
        .max_outstanding_items = 1024,
        .max_item_processing_time_ns = 2000000000,
        .engine_type = LUX_ENGINE_CHAIN
    };

    lux_consensus_engine_t* engine = NULL;
    lux_consensus_engine_create(&engine, &config);

    // Create a chain of blocks
    uint8_t parent_id[32] = {0};  // Genesis

    for (int i = 1; i <= 5; i++) {
        lux_block_t block = {0};
        memset(block.id, i, 32);
        memcpy(block.parent_id, parent_id, 32);
        block.height = i;
        block.timestamp = time(NULL) + i;

        lux_error_t err = lux_consensus_add_block(engine, &block);

        char test_name[50];
        snprintf(test_name, sizeof(test_name), "Added block at height %d", i);
        TEST(test_name);
        if (err == LUX_SUCCESS) {
            PASS();
        } else {
            FAIL("Failed to add block");
        }

        // Next block's parent is this block
        memcpy(parent_id, block.id, 32);
    }

    lux_consensus_engine_destroy(engine);
    lux_consensus_cleanup();
    return 0;
}

int main(void) {
    printf("%s========================================%s\n", COLOR_BLUE, COLOR_RESET);
    printf("%s   EXTENDED CONSENSUS BEHAVIOR TESTS    %s\n", COLOR_BLUE, COLOR_RESET);
    printf("%s========================================%s\n", COLOR_BLUE, COLOR_RESET);

    test_voting_changes_state();
    test_preference_tracking();
    test_engine_types_api();
    test_block_hierarchy();

    printf("\n%s========================================%s\n", COLOR_BLUE, COLOR_RESET);
    printf("%s               SUMMARY                  %s\n", COLOR_BLUE, COLOR_RESET);
    printf("%s========================================%s\n", COLOR_BLUE, COLOR_RESET);
    printf("Total Tests: %d\n", state.passed + state.failed);
    printf("%sPassed: %d%s\n", COLOR_GREEN, state.passed, COLOR_RESET);
    printf("%sFailed: %d%s\n", COLOR_RED, state.failed, COLOR_RESET);

    if (state.failed == 0) {
        printf("\n%s✅ All extended tests passed!%s\n", COLOR_GREEN, COLOR_RESET);
        return 0;
    } else {
        printf("\n%s❌ Some tests failed%s\n", COLOR_RED, COLOR_RESET);
        return 1;
    }
}