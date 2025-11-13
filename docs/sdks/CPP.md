# C++ SDK Guide

Modern C++ SDK for Lux Consensus with template optimizations and optional MLX GPU acceleration.

[![C++ Standard](https://img.shields.io/badge/C++-17-blue)](https://en.cppreference.com/w/cpp/17)
[![Test Coverage](https://img.shields.io/badge/coverage-95%25-yellow)](#test-coverage)
[![Status](https://img.shields.io/badge/status-beta-orange)](#status)

## Installation

```bash
cd /Users/z/work/lux/consensus/pkg/cpp
mkdir build && cd build
cmake ..
make
sudo make install
```

## Hello Consensus

```cpp
#include <lux/consensus.hpp>
#include <iostream>

int main() {
    // Create configuration
    lux::Config config;
    config.k = 21;
    config.alpha_preference = 15;
    config.alpha_confidence = 18;
    config.beta = 8;

    // Create consensus chain
    auto chain = lux::Chain::create(config);

    // Start engine
    chain->start();

    // Create block
    auto block = lux::Block{
        .id = lux::generate_id(),
        .parent_id = lux::GENESIS_ID,
        .height = 1,
        .payload = {'H', 'e', 'l', 'l', 'o', '!'}
    };

    // Add block
    chain->add(block);

    std::cout << "Block added with quantum finality!\\n";

    // Cleanup (RAII handles this automatically)
    return 0;
}
```

### Compile

```bash
g++ -std=c++17 -o hello hello.cpp -lluxconsensus
./hello
```

## Performance Benchmarks

| Operation | Time/Op | Throughput |
|-----------|---------|------------|
| **Single Block Add** | ~800 ns | ~1.25M blocks/sec |
| **Single Vote** | ~700 ns | ~1.4M votes/sec |
| **Batch (10,000 blocks)** | ~111 ns/block | 9M blocks/sec |

### MLX Acceleration (Apple Silicon Only)

With MLX enabled on M1/M2/M3 chips:
- **4.5x faster** than pure Go
- **~300 ns per decision** for AI consensus
- **GPU acceleration** for large batches

## Testing

```bash
cd /Users/z/work/lux/consensus/pkg/cpp/build
ctest --verbose

# Or manually
./tests/consensus_test
```

**Test Coverage**: 95% (beta status)

## Modern C++ Features

### RAII & Smart Pointers

```cpp
{
    auto chain = lux::Chain::create(config);
    // Automatically cleaned up on scope exit
}
```

### Move Semantics

```cpp
auto block = create_block();
chain->add(std::move(block)); // Zero-copy transfer
```

### Type Safety

```cpp
// Compile-time type checking
lux::BlockID id = chain->preference();  // OK
int invalid = chain->preference();      // Compiler error
```

## Resources

- **[CMake Build](../../pkg/cpp/CMakeLists.txt)**
- **[Examples](../../pkg/cpp/examples/)**
- **[Header Files](../../pkg/cpp/include/lux/)**

---

**Note**: C++ SDK is in **beta**. Production use recommended for Go, Python, Rust, or C SDKs.

**Need help?** [Open an issue](https://github.com/luxfi/consensus/issues/new).
