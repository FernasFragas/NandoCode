package semantic

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
)

func NormalizeVector(vec []float32) bool {
	if len(vec) == 0 {
		return false
	}
	var sum float64
	for _, v := range vec {
		sum += float64(v * v)
	}
	if sum <= 0 {
		return false
	}
	norm := float32(math.Sqrt(sum))
	for i := range vec {
		vec[i] /= norm
	}
	return true
}

func Dot(a, b []float32) (float32, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("dot: dimension mismatch %d != %d", len(a), len(b))
	}
	var out float32
	for i := range a {
		out += a[i] * b[i]
	}
	return out, nil
}

func ValidateVectorSet(vs VectorSet, expectedDimensions int, expectedCount int) error {
	if vs.Dimensions <= 0 {
		return fmt.Errorf("vector set dimensions must be > 0")
	}
	if expectedDimensions > 0 && vs.Dimensions != expectedDimensions {
		return fmt.Errorf("vector set dimensions mismatch: got %d want %d", vs.Dimensions, expectedDimensions)
	}
	if expectedCount >= 0 && len(vs.Vectors) != expectedCount {
		return fmt.Errorf("vector set count mismatch: got %d want %d", len(vs.Vectors), expectedCount)
	}
	for i, vec := range vs.Vectors {
		if len(vec) != vs.Dimensions {
			return fmt.Errorf("vector %d has dimension %d, want %d", i, len(vec), vs.Dimensions)
		}
	}
	return nil
}

func WriteF32File(path string, vectors [][]float32, dimensions int) error {
	if dimensions <= 0 {
		return fmt.Errorf("dimensions must be > 0")
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	for i := range vectors {
		if len(vectors[i]) != dimensions {
			return fmt.Errorf("vector %d has dimension %d, want %d", i, len(vectors[i]), dimensions)
		}
		if err := binary.Write(f, binary.LittleEndian, vectors[i]); err != nil {
			return err
		}
	}
	return nil
}

func LoadF32File(path string, dimensions int, expectedCount int) ([][]float32, error) {
	if dimensions <= 0 {
		return nil, fmt.Errorf("dimensions must be > 0")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	stats, err := f.Stat()
	if err != nil {
		return nil, err
	}
	elemSize := int64(4)
	perVector := int64(dimensions) * elemSize
	if perVector <= 0 {
		return nil, fmt.Errorf("invalid dimensions")
	}
	if stats.Size()%perVector != 0 {
		return nil, fmt.Errorf("%w: vectors file has invalid byte length", ErrCorruptIndex)
	}

	count := int(stats.Size() / perVector)
	if expectedCount >= 0 && count != expectedCount {
		return nil, fmt.Errorf("%w: vectors count mismatch got %d want %d", ErrCorruptIndex, count, expectedCount)
	}

	out := make([][]float32, count)
	buf := make([]float32, dimensions)
	for i := 0; i < count; i++ {
		if err := binary.Read(f, binary.LittleEndian, buf); err != nil {
			if err == io.EOF {
				return nil, fmt.Errorf("%w: unexpected EOF while reading vectors", ErrCorruptIndex)
			}
			return nil, err
		}
		vec := make([]float32, dimensions)
		copy(vec, buf)
		out[i] = vec
	}
	return out, nil
}
