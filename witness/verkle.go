package witness

// VerkleHints lets consensus request compact witnesses for a tx batch.
type VerkleHints interface {
	PrepareHints(keys [][]byte) (witnessBlob []byte, estSize int, err error)
}
