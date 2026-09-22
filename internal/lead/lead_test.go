package lead

import "testing"

func TestPlausiblePhone(t *testing.T) {
	good := []string{
		"9822041190", "+91 98220 41190", "+919822041190",
		"98220-41190", "91 7020533449", "6123456789",
	}
	for _, p := range good {
		if !plausiblePhone(p) {
			t.Errorf("%q should be accepted", p)
		}
	}

	bad := []string{
		"", "12345", "98220411901234",
		"5123456789",  // Indian mobiles do not start below 6
		"0822041190",  // leading zero
		"abcdefghij",
		"+1 415 555 0132", // ten digits but not an Indian mobile prefix
	}
	for _, p := range bad {
		if plausiblePhone(p) {
			t.Errorf("%q should be rejected", p)
		}
	}
}







func TestValidEnquiryStatus(t *testing.T) {
	for _, s := range []string{"new", "contacted", "visit_scheduled", "converted", "lost"} {
		if !validEnquiryStatus(s) {
			t.Errorf("%q should be valid", s)
		}
	}
	for _, s := range []string{"", "NEW", "won", "closed"} {
		if validEnquiryStatus(s) {
			t.Errorf("%q should not be valid", s)
		}
	}
}
