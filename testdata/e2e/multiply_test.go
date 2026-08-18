package arithmetic

import "testing"

func TestMultiply(t *testing.T) {
	if got, want := Multiply(2, 3), 6; got != want {
		t.Fatalf("Multiply(2, 3) = %d, want %d", got, want)
	}
}
