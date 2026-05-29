package input

import "testing"

func TestKeyBytes(t *testing.T) {
	got, err := KeyBytes("enter")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "\r" {
		t.Fatalf("enter = %q", string(got))
	}
	got, err = KeyBytes("q")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "q" {
		t.Fatalf("q = %q", string(got))
	}
}
