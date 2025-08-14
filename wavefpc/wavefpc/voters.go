package wavefpc

func votersFromBitset(bs *bitset) []ValidatorIndex {
    out := make([]ValidatorIndex, 0, bs.Count())
    bs.ForEach(func(i int) bool {
        out = append(out, ValidatorIndex(i))
        return true
    })
    return out
}
