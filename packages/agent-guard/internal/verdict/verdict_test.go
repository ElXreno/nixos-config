package verdict

import "testing"

func TestParseLevelAcceptsLegacyDigits(t *testing.T) {
	cases := map[string]Level{
		"silent": Silent, "0": Silent,
		"advise": Advise, "1": Advise,
		"ask":  Ask,
		"deny": Deny, "2": Deny,
	}
	for in, want := range cases {
		got, err := ParseLevel(in)
		if err != nil || got != want {
			t.Errorf("ParseLevel(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseLevel("3"); err == nil {
		t.Error("an unknown level must be an error")
	}
}

func TestLevelTextRoundTrip(t *testing.T) {
	for _, l := range []Level{Silent, Advise, Ask, Deny} {
		b, err := l.MarshalText()
		if err != nil {
			t.Fatal(err)
		}
		var back Level
		if err := back.UnmarshalText(b); err != nil || back != l {
			t.Errorf("%v: round trip gave %v, %v", l, back, err)
		}
	}
}
