# C SDK Guide

High-performance C library for Lux Consensus with minimal dependencies and optimized data structures.

[![C Standard](https://img.shields.io/badge/C-C11-blue)](https://en.cppreference.com/w/c/11)
[![Test Coverage](https://img.shields.io/badge/coverage-100%25-brightgreen)](#test-coverage)

## Installation

```bash
cd /Users/z/work/lux/consensus/pkg/c
make clean && make all
sudo make install
```

## Hello Consensus

```c
#include <luxconsensus/consensus.h>
#include <stdio.h>

int main(void) {
    lux_config_t config = {
        .k = 21,
        .alpha_preference = 15,
        .alpha_confidence = 18,
        .beta = 8,
        .engine_type = LUX_ENGINE_CHAIN,
    };

    lux_engine_t *engine = lux_engine_create(&config);
    if (!engine) return 1;

    lux_id_t block_id;
    lux_id_generate(&block_id);

    lux_engine_add_block(
        engine,
        &block_id,
        &LUX_GENESIS_ID,
        1,
        (uint8_t*)"Hello, Lux!",
        11
    );

    printf("Block added with quantum finality!\\n");
    lux_engine_destroy(engine);
    return 0;
}
```

### Compile

```bash
gcc -o hello hello.c -lluxconsensus
./hello
```

## Performance Benchmarks

| Operation | Time/Op | Throughput |
|-----------|---------|------------|
| **Single Block Add** | 8.97 µs | 111K blocks/sec |
| **Single Vote** | 46.4 µs | 21K votes/sec |
| **Finalization Check** | 320 ns | 3.1M checks/sec |
| **Get Preference** | 157 ns | 6.3M ops/sec |

### Memory Usage

- **Empty engine**: 29 KB
- **100 blocks**: 206 KB (~197 bytes/block)
- **1,000 blocks**: 1.88 MB
- **10,000 blocks**: 1.97 MB

## Testing

```bash
cd /Users/z/work/lux/consensus/pkg/c
make test          # Run all tests
make test-valgrind # Memory leak detection
make benchmark     # Run benchmarks
```

**Test Coverage**: 100% across all modules

## Resources

- **[API Headers](../../pkg/c/include/)**
- **[Examples](../../pkg/c/examples/)**
- **[Test Results](../../pkg/c/TEST_RESULTS.md)**
- **[Benchmarks](../../pkg/c/BENCHMARK_RESULTS.md)**

---

**Need help?** [Open an issue](https://github.com/luxfi/consensus/issues/new).
