package quasar

type Bundle struct {
    Epoch   uint64
    Root    []byte
    BLSAgg  []byte
    PQBatch []byte
    Binding []byte
}

type Client interface {
    SubmitCheckpoint(epoch uint64, root []byte, attest []byte) error
    FetchBundle(epoch uint64) (Bundle, error)
    Verify(Bundle) bool
}
