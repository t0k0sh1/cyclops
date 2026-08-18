package arithmetic

import "testing"

func TestAdd(t *testing.T) {
	if got, want := Add(1, 2), 3; got != want {
		t.Fatalf("Add(1, 2) = %d, want %d", got, want)
	}
}
