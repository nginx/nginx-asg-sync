package main

import "testing"

func TestValidateTime(t *testing.T) {
	time := "10"
	err := validateTime(time)

	if err != nil {
		t.Errorf("validateTime(%v) returned an error %v for valid input", time, err)
	}
}

func TestParseTime(t *testing.T) {
	var testsWithValidInput = []string{"1", "1m10s", "11 11", "5m 30s", "1s", "100m", "5w", "15m", "11M", "3h", "100y", "600"}
	var invalidInput = []string{"ss", "rM", "m0m", "s1s", "-5s", "", "1L"}
	for _, test := range testsWithValidInput {
		result, err := parseTime(test)
		if err != nil {
			t.Errorf("parseTime(%q) returned an error for valid input: %v", test, err)
		}
		if test != result {
			t.Errorf("parseTime(%q) returned %q expected %q", test, result, test)
		}
	}
	for _, test := range invalidInput {
		result, err := parseTime(test)
		if err == nil {
			t.Errorf("parseTime(%q) didn't return error. Returned: %q", test, result)
		}
	}
}
