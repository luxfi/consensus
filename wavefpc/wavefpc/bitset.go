package wavefpc

import "math/bits"

// bitset stores validator indices 0..N-1.
type bitset struct {
	words []uint64
	count int
}

func newBitset(n int) *bitset {
	words := (n + 63) / 64
	return &bitset{words: make([]uint64, words)}
}

func (b *bitset) Set(i int) bool {
	w, off := i/64, uint(i%64)
	mask := uint64(1) << off
	if b.words[w]&mask != 0 {
		return false
	}
	b.words[w] |= mask
	b.count++
	return true
}

func (b *bitset) Count() int { return b.count }

func (b *bitset) Bitmap() []byte {
	out := make([]byte, 0, len(b.words)*8)
	for _, w := range b.words {
		for i := 0; i < 8; i++ {
			out = append(out, byte(w>>(8*i)))
		}
	}
	return out
}

func (b *bitset) ForEach(f func(idx int) bool) {
	for wi, w := range b.words {
		for w != 0 {
			t := w & -w
			i := bits.TrailingZeros64(w)
			if !f(wi*64 + i) {
				return
			}
			w &= ^t
		}
	}
}
