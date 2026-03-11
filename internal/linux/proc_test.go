package linux

import "testing"

func TestBaseBlockDevice(t *testing.T) {
	cases := map[string]string{
		"/dev/sda":  "/dev/sda",
		"/dev/sda1": "/dev/sda",
		"/dev/dm-0": "",
	}
	for in, want := range cases {
		if got := baseBlockDevice(in); got != want {
			t.Fatalf("baseBlockDevice(%q) = %q, want %q", in, got, want)
		}
	}
}
