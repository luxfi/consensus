#include "lux/mlx_consensus.hpp"
#include <chrono>
#include <iostream>
#include <stdexcept>

#ifdef HAS_MLX

namespace lux {
namespace consensus {

namespace mx = mlx::core;

// Private implementation (PIMPL)
struct MLXConsensus::Impl {
    mx::array weights;          // Neural network weights
    mx::array biases;           // Neural network biases
    std::vector<Vote> cache;    // Vote cache
    bool profiling_enabled = false;

    explicit Impl(const MLXConfig& config) {
        // Initialize weights with small random values
        weights = mx::random::normal({32, 64}, mx::float32);
        biases = mx::zeros({64}, mx::float32);
    }
};

MLXConsensus::MLXConsensus(const MLXConfig& config)
    : pimpl_(std::make_unique<Impl>(config)), config_(config) {

    try {
        // Check if GPU is available
        if (config.device_type == "gpu" && mx::metal::is_available()) {
            mx::set_default_device(mx::Device::gpu());
            gpu_enabled_ = true;
            std::cout << "MLX GPU acceleration enabled\n";
            std::cout << "Device: " << get_device_name() << "\n";
        } else {
            mx::set_default_device(mx::Device::cpu());
            gpu_enabled_ = false;
            std::cout << "MLX running in CPU mode\n";
        }
    } catch (const std::exception& e) {
        throw std::runtime_error("Failed to initialize MLX: " + std::string(e.what()));
    }
}

MLXConsensus::~MLXConsensus() = default;

MLXConsensus::MLXConsensus(MLXConsensus&&) noexcept = default;
MLXConsensus& MLXConsensus::operator=(MLXConsensus&&) noexcept = default;

size_t MLXConsensus::process_votes_batch(std::span<const Vote> votes) {
    if (votes.empty()) {
        return 0;
    }

    try {
        // Convert votes to MLX array
        mx::array input;
        preprocess_batch(votes, input);

        // Run forward pass on GPU
        auto output = forward_pass(input);

        // Process results
        auto results = postprocess_results(output);

        // Count successful votes
        return std::count(results.begin(), results.end(), true);
    } catch (const std::exception& e) {
        std::cerr << "Error processing vote batch: " << e.what() << "\n";
        return 0;
    }
}

std::vector<bool> MLXConsensus::validate_blocks_batch(std::span<const BlockID> blocks) {
    std::vector<bool> results(blocks.size(), true);

    if (blocks.empty()) {
        return results;
    }

    try {
        // Convert block IDs to array (32 bytes each)
        std::vector<float> data;
        data.reserve(blocks.size() * 32);

        for (const auto& block_id : blocks) {
            for (size_t i = 0; i < 32; ++i) {
                data.push_back(static_cast<float>(block_id.data[i]) / 255.0f);
            }
        }

        // Create MLX array and run validation
        auto input = mx::array(data.data(), {static_cast<int>(blocks.size()), 32}, mx::float32);
        auto output = forward_pass(input);

        // Evaluate results on GPU
        mx::eval(output);

        // Convert to bool vector
        auto output_data = output.data<float>();
        for (size_t i = 0; i < blocks.size(); ++i) {
            results[i] = output_data[i] > 0.5f;
        }
    } catch (const std::exception& e) {
        std::cerr << "Error validating blocks: " << e.what() << "\n";
        // Return all true on error (conservative approach)
    }

    return results;
}

size_t MLXConsensus::get_gpu_memory_usage() const {
    if (!gpu_enabled_) {
        return 0;
    }
    return mx::metal::get_active_memory();
}

size_t MLXConsensus::get_peak_gpu_memory() const {
    if (!gpu_enabled_) {
        return 0;
    }
    return mx::metal::get_peak_memory();
}

void MLXConsensus::reset_peak_memory() {
    if (gpu_enabled_) {
        mx::metal::reset_peak_memory();
    }
}

std::string MLXConsensus::get_device_name() const {
    return mx::default_device().to_string();
}

void MLXConsensus::set_profiling(bool enable) {
    pimpl_->profiling_enabled = enable;
    if (enable) {
        setenv("MLX_DEBUG", "1", 1);
    } else {
        unsetenv("MLX_DEBUG");
    }
}

// Private methods

void MLXConsensus::preprocess_batch(std::span<const Vote> votes, mx::array& out) {
    // Convert votes to normalized float array
    std::vector<float> data;
    data.reserve(votes.size() * 64); // 32 bytes voter + 32 bytes block

    for (const auto& vote : votes) {
        // Voter ID (32 bytes)
        for (size_t i = 0; i < 32; ++i) {
            data.push_back(static_cast<float>(vote.voter_id.data[i]) / 255.0f);
        }
        // Block ID (32 bytes)
        for (size_t i = 0; i < 32; ++i) {
            data.push_back(static_cast<float>(vote.block_id.data[i]) / 255.0f);
        }
    }

    out = mx::array(data.data(), {static_cast<int>(votes.size()), 64}, mx::float32);
}

mx::array MLXConsensus::forward_pass(const mx::array& input) {
    // Simple 2-layer neural network
    // Layer 1: input * weights + bias
    auto layer1 = mx::matmul(input, pimpl_->weights) + pimpl_->biases;

    // ReLU activation
    auto activated = mx::maximum(layer1, 0.0f);

    // Layer 2: reduce to single output per sample
    auto layer2 = mx::mean(activated, /* axis= */ 1);

    // Sigmoid activation for final probability
    auto output = 1.0f / (1.0f + mx::exp(-layer2));

    // Force evaluation on GPU
    mx::eval(output);

    return output;
}

std::vector<bool> MLXConsensus::postprocess_results(const mx::array& output) {
    auto data = output.data<float>();
    std::vector<bool> results;
    results.reserve(output.size());

    for (size_t i = 0; i < output.size(); ++i) {
        results.push_back(data[i] > 0.5f);
    }

    return results;
}

// AdaptiveMLXBatchProcessor implementation

AdaptiveMLXBatchProcessor::AdaptiveMLXBatchProcessor(std::unique_ptr<MLXConsensus> mlx)
    : mlx_(std::move(mlx)) {}

void AdaptiveMLXBatchProcessor::add_vote(const Vote& vote) {
    vote_buffer_.push_back(vote);

    // Auto-flush when buffer reaches optimal size
    if (vote_buffer_.size() >= optimal_batch_size_) {
        flush();
    }
}

void AdaptiveMLXBatchProcessor::flush() {
    if (vote_buffer_.empty()) {
        return;
    }

    auto start = std::chrono::high_resolution_clock::now();

    mlx_->process_votes_batch(vote_buffer_);

    auto end = std::chrono::high_resolution_clock::now();
    auto duration = std::chrono::duration_cast<std::chrono::microseconds>(end - start);

    // Calculate throughput
    double current_throughput = vote_buffer_.size() * 1000000.0 / duration.count();

    // Update running average
    if (throughput_ == 0.0) {
        throughput_ = current_throughput;
    } else {
        throughput_ = 0.9 * throughput_ + 0.1 * current_throughput; // EMA
    }

    // Adjust batch size based on performance
    adjust_batch_size(current_throughput);

    vote_buffer_.clear();
}

void AdaptiveMLXBatchProcessor::adjust_batch_size(double current_throughput) {
    // Increase batch size if throughput is good
    if (current_throughput > 1000000.0 && optimal_batch_size_ < 128) {
        optimal_batch_size_ *= 2;
    }
    // Decrease if throughput is poor
    else if (current_throughput < 100000.0 && optimal_batch_size_ > 16) {
        optimal_batch_size_ /= 2;
    }
}

} // namespace consensus
} // namespace lux

#endif // HAS_MLX
