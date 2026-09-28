package addmissingunittests

import "testing"

func TestClampTable(t *testing.T) {
	tests := []struct {
		name  string
		value int
		min   int
		max   int
		want  int
	}{
		{name: "below min", value: 0, min: 1, max: 5, want: 1},
		{name: "within bounds", value: 3, min: 1, max: 5, want: 3},
		{name: "above max", value: 8, min: 1, max: 5, want: 5},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Clamp(tc.value, tc.min, tc.max); got != tc.want {
				t.Fatalf("got %d want %d", got, tc.want)
			}
		})
	}
}
