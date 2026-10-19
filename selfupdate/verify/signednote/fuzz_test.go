package signednote

import "testing"

// FuzzParseStatement: ParseStatement never panics, and a statement it
// accepts renders back to exactly the text it read (0025-MADR D10).
func FuzzParseStatement(f *testing.F) {
	f.Add(fixtureText)
	f.Add("selfupdate-release/v1\n")
	f.Add(fixtureText + "\n")
	f.Fuzz(func(t *testing.T, text string) {
		s, err := ParseStatement(text)
		if err != nil {
			return
		}
		got, err := s.Text()
		if err != nil || got != text {
			t.Fatalf("accepted %q, renders %q, %v", text, got, err)
		}
	})
}
