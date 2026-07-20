package index

import (
	"math"
	"testing"
)

func TestCosineIsSane(t *testing.T) {
	cases := []struct {
		name string
		a, b []float32
		want float64
	}{
		{"identical", []float32{1, 0}, []float32{1, 0}, 1},
		{"orthogonal", []float32{1, 0}, []float32{0, 1}, 0},
		{"mismatched dims must not panic", []float32{1}, []float32{1, 2}, 0},
		{"zero vector must not divide by zero", []float32{0, 0}, []float32{1, 1}, 0},
	}
	for _, c := range cases {
		if got := cosine(c.a, c.b); math.Abs(got-c.want) > 1e-6 {
			t.Errorf("%s: cosine = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestBlobRoundTrip(t *testing.T) {
	in := []float32{0.5, -1.25, 3.75, 0}
	out := blobToFloats(floatsToBlob(in))
	if len(out) != len(in) {
		t.Fatalf("len = %d, want %d", len(out), len(in))
	}
	for i := range in {
		if in[i] != out[i] {
			t.Errorf("index %d: got %v, want %v", i, out[i], in[i])
		}
	}
}
