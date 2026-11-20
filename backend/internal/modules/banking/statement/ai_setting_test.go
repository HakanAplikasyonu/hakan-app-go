package statement

import "testing"

func TestStatementAIEnabled(t *testing.T) {
	cases := map[string]bool{
		``:                                 true,
		`{}`:                               true,
		`{"bank_statement_ai":true}`:       true,
		`{"bank_statement_ai":false}`:      false,
		`{"bank_statement_ai":"no","x":1}`: true,
		`not json`:                         true,
	}
	for in, want := range cases {
		if got := StatementAIEnabled([]byte(in)); got != want {
			t.Errorf("%q: %v, want %v", in, got, want)
		}
	}
}
