package shell

import "testing"

func TestParsePipesRedirectsAndConditionals(t *testing.T) {
	statements, err := Parse(`printf 'hello' | grep hello > /tmp/out && cat /tmp/out || echo failed`)
	if err != nil {
		t.Fatal(err)
	}
	if len(statements) != 3 || len(statements[0].Pipeline.Commands) != 2 {
		t.Fatalf("unexpected first statement: %#v", statements)
	}
	if got := statements[0].Pipeline.Commands[1].Redirections[0].Operator; got != ">" {
		t.Fatalf("redirect operator = %q", got)
	}
	if statements[0].Previous != OpNone {
		t.Fatalf("first connector = %q", statements[0].Previous)
	}
	if _, err := Parse(`echo "unterminated`); err == nil {
		t.Fatal("unterminated quote was accepted")
	}
}
