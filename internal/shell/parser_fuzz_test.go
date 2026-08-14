package shell

import "testing"

func FuzzParseNeverPanics(f *testing.F) {
	f.Add("echo hello | grep hello && pwd")
	f.Add("printf 'unterminated")
	f.Fuzz(func(t *testing.T, input string) { _, _ = Parse(input) })
}
