package lexer_test

import (
	"testing"

	"github.com/recolabs/gnata/internal/lexer"
)

// A string literal without escapes is the source between its quotes: the
// lexer returns it without building a copy.
func TestStringLiteralWithoutEscapesDoesNotAllocate(t *testing.T) {
	src := `"a plain literal without any escape sequence"`
	tok, err := lexer.NewLexer(src).Next(false)
	if err != nil || tok.Type != lexer.TokenString || tok.Value != src[1:len(src)-1] {
		t.Fatalf("token %+v, %v", tok, err)
	}
	allocs := testing.AllocsPerRun(50, func() {
		l := lexer.NewLexer(src)
		if _, err := l.Next(false); err != nil {
			t.Fatal(err)
		}
	})
	if allocs > 1 {
		t.Fatalf("lexing a literal without escapes allocates %.0f times, want at most 1 (the lexer)", allocs)
	}
	escaped := `"line\nbreak \"quoted\""`
	tok, err = lexer.NewLexer(escaped).Next(false)
	if err != nil || tok.Value != "line\nbreak \"quoted\"" {
		t.Fatalf("escaped literal: %+v, %v", tok, err)
	}
}
