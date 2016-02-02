package testassert

import "testing"

func AssertEqual(t *testing.T, a interface{}, b interface{}, message string) {
	if a == b {
		t.Error(message)

	}
}
func AssertNotEqual(t *testing.T, a interface{}, b interface{}, message string) {
	if a != b {
		t.Error(message)

	}
}
