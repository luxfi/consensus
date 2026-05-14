cmake_minimum_required(VERSION 3.20)
project(lux_consensus_cpp VERSION 1.0.0 LANGUAGES CXX)

# Set C++ standard
set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

# Find required packages
find_package(Threads REQUIRED)

# Optional: Find ZeroMQ for network layer
find_package(PkgConfig QUIET)
if(PkgConfig_FOUND)
    pkg_check_modules(ZEROMQ QUIET libzmq)
endif()

# Optional: Find MLX for GPU acceleration. Upstream MLXConfig.cmake
# exports a lowercase imported target `mlx`, NOT `MLX::MLX`. We resolve
# whichever is actually exported and fall back to disabling the GPU
# path on hosts where MLX is partially installed (target absent).
# Homebrew installs MLXConfig.cmake under /usr/local/share/cmake/MLX
# (Intel) or /opt/homebrew/share/cmake/MLX (Apple Silicon); add both.
list(APPEND CMAKE_PREFIX_PATH /usr/local /opt/homebrew)
find_package(MLX QUIET)
set(MLX_TARGET)
if(TARGET MLX::MLX)
    set(MLX_TARGET MLX::MLX)
elseif(TARGET mlx)
    set(MLX_TARGET mlx)
endif()
if(MLX_FOUND AND NOT MLX_TARGET)
    set(MLX_FOUND FALSE)
    message(STATUS "MLX detected but no imported target found; GPU path disabled.")
endif()

# Library sources (matching Go implementation structure)
set(SOURCES
    src/consensus.cpp
    src/network.cpp
    src/vote_processor.cpp
)

# MLX sources (conditional)
if(MLX_FOUND)
    list(APPEND SOURCES src/mlx_consensus.cpp)
endif()

# Create library
add_library(lux_consensus STATIC ${SOURCES})

# Include directories
target_include_directories(lux_consensus
    PUBLIC
        $<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}/include>
        $<INSTALL_INTERFACE:include>
)

# Link libraries
target_link_libraries(lux_consensus
    PUBLIC
        Threads::Threads
)

# Link ZeroMQ if available (optional)
if(ZEROMQ_FOUND)
    target_include_directories(lux_consensus PRIVATE ${ZEROMQ_INCLUDE_DIRS})
    target_link_directories(lux_consensus INTERFACE ${ZEROMQ_LIBRARY_DIRS})
    target_link_libraries(lux_consensus INTERFACE ${ZEROMQ_LIBRARIES})
    target_compile_definitions(lux_consensus PRIVATE HAS_ZEROMQ)
endif()

# Compile options
target_compile_options(lux_consensus PRIVATE
    -Wall -Wextra -Wpedantic
    $<$<CONFIG:Release>:-O3 -march=native>
    $<$<CONFIG:Debug>:-g -fsanitize=address>
)

# MLX support
if(MLX_FOUND)
    target_compile_definitions(lux_consensus PUBLIC HAS_MLX)
    target_link_libraries(lux_consensus PRIVATE ${MLX_TARGET})
endif()

# Build examples (optional - may have linking issues)
# add_subdirectory(examples)

# Build tests
enable_testing()
add_subdirectory(tests)

# Installation
install(TARGETS lux_consensus
    EXPORT lux_consensus_targets
    LIBRARY DESTINATION lib
    ARCHIVE DESTINATION lib
)

install(DIRECTORY include/
    DESTINATION include
)

install(EXPORT lux_consensus_targets
    FILE lux_consensus_config.cmake
    NAMESPACE Lux::
    DESTINATION lib/cmake/lux_consensus
)