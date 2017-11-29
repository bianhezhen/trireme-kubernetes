package utils

import (
	"strings"
	"testing"

	"github.com/aporeto-inc/trireme-lib/enforcer/utils/tokens"
)

var nodeNameGenTests = []struct {
	in string
}{
	{"a"},
	{""},
	{"00"},
	{"TriremeNode"},
	{"supersuperlongnamethatislongerthantriremetokenbyquitealot"},
}

func TestNodeNameGen(t *testing.T) {
	for _, tt := range nodeNameGenTests {
		s := GenerateNodeName(tt.in)

		if len(s) > tokens.MaxServerName {
			t.Errorf("GenerateNodeName(%q) => %q, Should be smaller than maxtoken", tt.in, s)
		}

		if !strings.HasPrefix(s, "trireme-") {
			t.Errorf("GenerateNodeName(%q) => %q, Should have trireme- as prefix", tt.in, s)
		}
	}
}
