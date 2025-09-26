// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

#ifndef LUX_CONSENSUS_H
#define LUX_CONSENSUS_H

#include <stdint.h>
#include <stdbool.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

// Version information
#define LUX_CONSENSUS_VERSION_MAJOR 1
#define LUX_CONSENSUS_VERSION_MINOR 0
#define LUX_CONSENSUS_VERSION_PATCH 0

// Error codes
typedef enum {
    LUX_SUCCESS = 0,
    LUX_ERROR_INVALID_PARAMS = -1,
    LUX_ERROR_OUT_OF_MEMORY = -2,
    LUX_ERROR_INVALID_STATE = -3,
    LUX_ERROR_CONSENSUS_FAILED = -4,
    LUX_ERROR_NOT_IMPLEMENTED = -5,
} lux_error_t;

// Consensus engine types
typedef enum {
    LUX_ENGINE_CHAIN = 0,
    LUX_ENGINE_DAG = 1,
    LUX_ENGINE_PQ = 2,
} lux_engine_type_t;

// Forward declarations
typedef struct lux_consensus_config lux_consensus_config_t;
typedef struct lux_consensus_engine lux_consensus_engine_t;
typedef struct lux_vote lux_vote_t;
typedef struct lux_block lux_block_t;

// Consensus configuration
struct lux_consensus_config {
    uint32_t k;                      // Sample size
    uint32_t alpha_preference;       // Preference quorum size
    uint32_t alpha_confidence;       // Confidence quorum size
    uint32_t beta;                   // Decision threshold
    uint32_t concurrent_polls;       // Number of concurrent polls
    uint32_t optimal_processing;     // Optimal processing
    uint32_t max_outstanding_items;  // Max outstanding items
    uint64_t max_item_processing_time_ns; // Max processing time in nanoseconds
    lux_engine_type_t engine_type;   // Engine type
};

// Block structure
struct lux_block {
    uint8_t id[32];        // Block ID (32 bytes)
    uint8_t parent_id[32]; // Parent block ID
    uint64_t height;       // Block height
    uint64_t timestamp;    // Unix timestamp
    void* data;            // Block data
    size_t data_size;      // Size of block data
};

// Vote structure
struct lux_vote {
    uint8_t voter_id[32];  // Voter node ID
    uint8_t block_id[32];  // Block being voted for
    bool is_preference;    // Is this a preference vote?
};

// Callback function types
typedef void (*lux_callback_decision)(const uint8_t* block_id, void* user_data);
typedef bool (*lux_callback_verify)(const lux_block_t* block, void* user_data);
typedef void (*lux_callback_notify)(const char* event, void* user_data);

// Core API functions

// Initialize the consensus library
lux_error_t lux_consensus_init(void);

// Cleanup the consensus library
lux_error_t lux_consensus_cleanup(void);

// Create a new consensus engine
lux_error_t lux_consensus_engine_create(
    lux_consensus_engine_t** engine,
    const lux_consensus_config_t* config
);

// Destroy a consensus engine
lux_error_t lux_consensus_engine_destroy(lux_consensus_engine_t* engine);

// Add a new block to the consensus engine
lux_error_t lux_consensus_add_block(
    lux_consensus_engine_t* engine,
    const lux_block_t* block
);

// Process a vote
lux_error_t lux_consensus_process_vote(
    lux_consensus_engine_t* engine,
    const lux_vote_t* vote
);

// Check if a block is accepted
lux_error_t lux_consensus_is_accepted(
    lux_consensus_engine_t* engine,
    const uint8_t* block_id,
    bool* is_accepted
);

// Get preference
lux_error_t lux_consensus_get_preference(
    lux_consensus_engine_t* engine,
    uint8_t* block_id
);

// Poll for consensus
lux_error_t lux_consensus_poll(
    lux_consensus_engine_t* engine,
    uint32_t num_validators,
    const uint8_t** validator_ids
);

// Register callbacks
lux_error_t lux_consensus_register_decision_callback(
    lux_consensus_engine_t* engine,
    lux_callback_decision callback,
    void* user_data
);

lux_error_t lux_consensus_register_verify_callback(
    lux_consensus_engine_t* engine,
    lux_callback_verify callback,
    void* user_data
);

lux_error_t lux_consensus_register_notify_callback(
    lux_consensus_engine_t* engine,
    lux_callback_notify callback,
    void* user_data
);

// Statistics and metrics
typedef struct {
    uint64_t blocks_accepted;
    uint64_t blocks_rejected;
    uint64_t polls_completed;
    uint64_t votes_processed;
    double average_decision_time_ms;
} lux_consensus_stats_t;

lux_error_t lux_consensus_get_stats(
    lux_consensus_engine_t* engine,
    lux_consensus_stats_t* stats
);

// Utility functions
const char* lux_error_string(lux_error_t error);
const char* lux_engine_type_string(lux_engine_type_t type);

#ifdef __cplusplus
}
#endif

#endif // LUX_CONSENSUS_H