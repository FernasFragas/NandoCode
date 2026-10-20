package main

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"testing"
)

func TestDiffIdenticalAndChanged(t *testing.T) {
	baseline := image.NewRGBA(image.Rect(0, 0, 2, 2))
	actual := image.NewRGBA(image.Rect(0, 0, 2, 2))
	if got, err := diff(baseline, actual); err != nil || got != 0 {
		t.Fatalf("identical: diff = %v, err = %v; want 0", got, err)
	}

	actual.Set(1, 1, color.RGBA{R: 255, A: 255})
	if got, err := diff(baseline, actual); err != nil || got != 0.25 {
		t.Fatalf("one changed pixel: diff = %v, err = %v; want 0.25", got, err)
	}

	if _, err := diff(baseline, image.NewRGBA(image.Rect(0, 0, 1, 2))); err == nil {
		t.Fatal("dimension mismatch: want an error")
	}
}

func TestExitCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"ok", nil, 0},
		{"chrome missing", fmt.Errorf("wrapped: %w", errChromeMissing), 2},
		{"over threshold", errOverThreshold, 1},
		{"missing baseline", errors.New("baseline x: no such file"), 1},
	}
	for _, c := range cases {
		if got := exitCode(c.err); got != c.want {
			t.Errorf("%s: exitCode = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestCropToViewport(t *testing.T) {
	tall := image.NewRGBA(image.Rect(0, 0, 4, 6))
	got, err := cropTo(tall, 4, 4)
	if err != nil || got.Bounds().Dx() != 4 || got.Bounds().Dy() != 4 {
		t.Fatalf("taller screenshot: bounds = %v, err = %v; want 4x4", got.Bounds(), err)
	}
	if _, err := cropTo(image.NewRGBA(image.Rect(0, 0, 4, 3)), 4, 4); err == nil {
		t.Fatal("shorter screenshot: want an error naming both sizes")
	}
}
