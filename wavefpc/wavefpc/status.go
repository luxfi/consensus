package wavefpc

type Status uint8

const (
    Pending Status = iota
    Executable
    Final
)

type Proof struct {
    // Optional classical + PQ proofs, when you wire them:
    BLSAgg      []byte // aggregate BLS on VotePrefix||epoch||tx (compact)
    VoterBitmap []byte // bitmap of voters included in BLSAgg
    PQCert      []byte // Ringtail (or other PQ) threshold signature (optional)
}
