package shell

import (
	"fmt"
	"strings"
)

type Operator string

const (
	OpNone     Operator = ""
	OpSequence Operator = ";"
	OpAnd      Operator = "&&"
	OpOr       Operator = "||"
)

type Redirection struct {
	FD       int
	Operator string
	Target   string
}
type Command struct {
	Words        []string
	Redirections []Redirection
}
type Pipeline struct{ Commands []Command }
type Statement struct {
	Previous Operator
	Pipeline Pipeline
}

func Parse(input string) ([]Statement, error) {
	tokens, err := lex(input)
	if err != nil {
		return nil, err
	}
	var statements []Statement
	current := Statement{}
	var pipeline Pipeline
	var command Command
	flushCommand := func() error {
		if len(command.Words) == 0 {
			return fmt.Errorf("shell: empty command")
		}
		pipeline.Commands = append(pipeline.Commands, command)
		command = Command{}
		return nil
	}
	flushPipeline := func() error {
		if len(command.Words) > 0 {
			if err := flushCommand(); err != nil {
				return err
			}
		}
		if len(pipeline.Commands) == 0 {
			return fmt.Errorf("shell: empty pipeline")
		}
		current.Pipeline = pipeline
		statements = append(statements, current)
		pipeline = Pipeline{}
		current = Statement{Previous: OpSequence}
		return nil
	}
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		switch t {
		case "|":
			if err := flushCommand(); err != nil {
				return nil, err
			}
		case ";", "&&", "||":
			if err := flushPipeline(); err != nil {
				return nil, err
			}
			current.Previous = Operator(t)
		case ">", ">>", "<", "2>", "2>&1":
			if i+1 >= len(tokens) {
				return nil, fmt.Errorf("shell: redirection %s requires a target", t)
			}
			i++
			fd := 1
			if strings.HasPrefix(t, "2") {
				fd = 2
			}
			command.Redirections = append(command.Redirections, Redirection{FD: fd, Operator: t, Target: tokens[i]})
		default:
			command.Words = append(command.Words, t)
		}
	}
	if len(command.Words) > 0 || len(pipeline.Commands) > 0 {
		if err := flushPipeline(); err != nil {
			return nil, err
		}
	}
	if len(statements) == 0 {
		return nil, nil
	}
	statements[0].Previous = OpNone
	return statements, nil
}

func lex(input string) ([]string, error) {
	var out []string
	var word strings.Builder
	wordStarted := false
	quote := rune(0)
	escaped := false
	flush := func() {
		if wordStarted {
			out = append(out, word.String())
			word.Reset()
			wordStarted = false
		}
	}
	for _, r := range input {
		if escaped {
			word.WriteRune(r)
			wordStarted = true
			escaped = false
			continue
		}
		if quote == '\'' {
			if r == '\'' {
				quote = 0
			} else {
				word.WriteRune(r)
				wordStarted = true
			}
			continue
		}
		if quote == '"' {
			if r == '"' {
				quote = 0
			} else if r == '\\' {
				escaped = true
			} else {
				word.WriteRune(r)
				wordStarted = true
			}
			continue
		}
		switch r {
		case '\\':
			escaped = true
			wordStarted = true
		case '\'', '"':
			quote = r
			wordStarted = true
		case ' ', '\t', '\r', '\n':
			flush()
		case '|', ';', '>', '<', '&':
			flush() // operators are assembled from their textual forms below
			// A small look-behind over the original input is unnecessary for the
			// supported operators; the lexer emits adjacent punctuation and the
			// normalizer below combines it.
			out = append(out, string(r))
		default:
			word.WriteRune(r)
			wordStarted = true
		}
	}
	if escaped {
		return nil, fmt.Errorf("shell: trailing escape")
	}
	if quote != 0 {
		return nil, fmt.Errorf("shell: unterminated quote")
	}
	flush()
	return normalizeOperators(out), nil
}

func normalizeOperators(tokens []string) []string {
	var out []string
	for i := 0; i < len(tokens); i++ {
		if tokens[i] == "|" && i+1 < len(tokens) && tokens[i+1] == "|" {
			out = append(out, "||")
			i++
			continue
		}
		if tokens[i] == "&" && i+1 < len(tokens) && tokens[i+1] == "&" {
			out = append(out, "&&")
			i++
			continue
		}
		if tokens[i] == "2" && i+3 < len(tokens) && tokens[i+1] == ">" && tokens[i+2] == "&" && tokens[i+3] == "1" {
			out = append(out, "2>&1")
			i += 3
			continue
		}
		if tokens[i] == "2" && i+1 < len(tokens) && tokens[i+1] == ">" {
			out = append(out, "2>")
			i++
			continue
		}
		if tokens[i] == ">" && i+1 < len(tokens) && tokens[i+1] == ">" {
			out = append(out, ">>")
			i++
			continue
		}
		out = append(out, tokens[i])
	}
	return out
}
