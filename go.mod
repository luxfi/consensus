module github.com/luxfi/consensus

go 1.25.5

exclude github.com/ethereum/go-ethereum v1.10.26

exclude github.com/ethereum/go-ethereum v1.16.3

require (
	github.com/luxfi/accel v0.0.0-00010101000000-000000000000
	github.com/luxfi/crypto v1.17.37
	github.com/luxfi/database v1.17.38
	github.com/luxfi/geth v1.16.69
	github.com/luxfi/ids v1.2.8
	github.com/luxfi/log v1.3.1
	github.com/luxfi/math v1.2.2
	github.com/luxfi/metric v1.4.9
	github.com/luxfi/p2p v1.18.4
	github.com/luxfi/ringtail v0.2.0
	github.com/luxfi/vm v1.0.2
	github.com/luxfi/warp v1.18.2
	github.com/prometheus/client_golang v1.23.2
	github.com/prometheus/client_model v0.6.2
	github.com/stretchr/testify v1.11.1
	go.uber.org/mock v0.6.0
)

require (
	github.com/ALTree/bigfloat v0.2.0 // indirect
	github.com/ProjectZKM/Ziren/crates/go-runtime/zkvm_runtime v0.0.0-20251230134950-44c893854e3f // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bits-and-blooms/bitset v1.24.4 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudflare/circl v1.6.2 // indirect
	github.com/consensys/gnark-crypto v0.19.2 // indirect
	github.com/crate-crypto/go-eth-kzg v1.4.0 // indirect
	github.com/crate-crypto/go-ipa v0.0.0-20240724233137-53bbb0ceb27a // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.0 // indirect
	github.com/dgraph-io/badger/v4 v4.9.0 // indirect
	github.com/dgraph-io/ristretto/v2 v2.3.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/ethereum/c-kzg-4844/v2 v2.1.5 // indirect
	github.com/ethereum/go-verkle v0.2.2 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/snappy v1.0.0 // indirect
	github.com/google/flatbuffers v25.12.19+incompatible // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/holiman/uint256 v1.3.2 // indirect
	github.com/klauspost/compress v1.18.2 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/luxfi/cache v1.1.0 // indirect
	github.com/luxfi/compress v0.0.2 // indirect
	github.com/luxfi/concurrent v0.0.2 // indirect
	github.com/luxfi/lattice/v7 v7.0.0 // indirect
	github.com/luxfi/math/big v0.1.0 // indirect
	github.com/luxfi/sampler v1.0.0 // indirect
	github.com/luxfi/utils v1.1.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/montanaflynn/stats v0.7.1 // indirect
	github.com/mr-tron/base58 v1.2.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/common v0.67.5 // indirect
	github.com/prometheus/procfs v0.19.2 // indirect
	github.com/supranational/blst v0.3.16 // indirect
	github.com/syndtr/goleveldb v1.0.1-0.20220614013038-64ee5596c38a // indirect
	github.com/zeebo/assert v1.3.0 // indirect
	github.com/zeebo/blake3 v0.2.4 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.39.0 // indirect
	go.opentelemetry.io/otel/metric v1.39.0 // indirect
	go.opentelemetry.io/otel/trace v1.39.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.1 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	golang.org/x/crypto v0.46.0 // indirect
	golang.org/x/exp v0.0.0-20251219203646-944ab1f22d93 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	gonum.org/v1/gonum v0.16.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

exclude google.golang.org/genproto v0.0.0-20230410155749-daa745c078e1

exclude google.golang.org/genproto/googleapis/rpc v0.0.0-20250908214217-97024824d090

replace github.com/luxfi/accel => ../accel
