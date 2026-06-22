package semantic

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeVectorAndDot(t *testing.T) {
	t.Parallel()
	v := []float32{3, 4}
	if ok := NormalizeVector(v); !ok {
		t.Fatalf("expected non-zero vector normalization")
	}
	d, err := Dot(v, v)
	if err != nil {
		t.Fatal(err)
	}
	if d < 0.999 || d > 1.001 {
		t.Fatalf("self dot should be ~1, got %f", d)
	}
}

func TestLoadAndWriteF32File(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	path := filepath.Join(td, "vectors.f32")
	in := [][]float32{{1, 2, 3}, {4, 5, 6}}
	if err := WriteF32File(path, in, 3); err != nil {
		t.Fatal(err)
	}
	out, err := LoadF32File(path, 3, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || len(out[0]) != 3 {
		t.Fatalf("unexpected shape: %d x %d", len(out), len(out[0]))
	}
	if out[1][2] != 6 {
		t.Fatalf("unexpected value: %f", out[1][2])
	}
}

func TestLoadF32FileDetectsCorruptLength(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	path := filepath.Join(td, "bad.f32")
	if err := os.WriteFile(path, []byte{1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadF32File(path, 2, -1); err == nil {
		t.Fatalf("expected corrupt file error")
	}
}
