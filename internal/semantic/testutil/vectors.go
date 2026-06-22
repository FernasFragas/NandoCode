package testutil

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
)

// DeterministicVector derives a stable vector from input text for tests.
func DeterministicVector(text string, dimensions int) []float32 {
	if dimensions <= 0 {
		return nil
	}
	out := make([]float32, dimensions)
	seed := sha256.Sum256([]byte(text))
	state := binary.LittleEndian.Uint64(seed[:8]) ^ 0x9e3779b97f4a7c15
	for i := 0; i < dimensions; i++ {
		state ^= state << 13
		state ^= state >> 7
		state ^= state << 17
		v := float64(state%1000000)/500000.0 - 1.0
		out[i] = float32(math.Max(-1, math.Min(1, v)))
	}
	return out
}
