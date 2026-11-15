package rpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type line struct {
	ID     uint64          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *Error          `json:"error"`
	Event  string          `json:"event"`
}

func newPiped() (*Server, *io.PipeReader, *io.PipeWriter) {
	pr, pw := io.Pipe()
	return NewServer(pw), pr, pw
}

func TestDispatch(t *testing.T) {
	s, pr, pw := newPiped()
	s.Handle("echo", func(_ context.Context, p json.RawMessage) (any, error) {
		var v struct{ X int }
		if err := Decode(p, &v); err != nil {
			return nil, err
		}
		return map[string]int{"x": v.X}, nil
	})
	s.Handle("slow", func(context.Context, json.RawMessage) (any, error) {
		time.Sleep(50 * time.Millisecond)
		return "slow", nil
	})
	s.Handle("fail", func(context.Context, json.RawMessage) (any, error) { return nil, errors.New("boom") })
	s.Handle("coded", func(context.Context, json.RawMessage) (any, error) { return nil, Errorf("NOT_UBL", "x") })
	s.Handle("panic", func(context.Context, json.RawMessage) (any, error) { panic("oops") })

	input := strings.Join([]string{
		`{"id":1,"method":"slow"}`,
		`{"id":2,"method":"echo","params":{"X":7}}`,
		`{"id":3,"method":"nope"}`,
		`not json`,
		``,
		`{"id":4,"method":"fail"}`,
		`{"id":5,"method":"coded"}`,
		`{"id":6,"method":"panic"}`,
		`{"id":7,"method":"echo","params":"bad"}`,
	}, "\n")

	got := map[uint64]line{}
	var order []uint64
	done := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(pr)
		for sc.Scan() {
			var l line
			if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
				t.Errorf("çıktı JSON değil: %q", sc.Text())
			}
			got[l.ID] = l
			order = append(order, l.ID)
		}
		close(done)
	}()
	if err := s.Serve(context.Background(), strings.NewReader(input)); err != nil {
		t.Fatal(err)
	}
	_ = pw.Close()
	<-done

	if string(got[2].Result) != `{"x":7}` {
		t.Errorf("echo: %s", got[2].Result)
	}
	if string(got[1].Result) != `"slow"` {
		t.Errorf("slow: %s", got[1].Result)
	}
	// Eşzamanlı: yavaş istek sonrakileri bekletmez.
	if order[len(order)-1] != 1 {
		t.Errorf("yavaş yanıt en sonda olmalıydı, sıra: %v", order)
	}
	checks := map[uint64]string{3: CodeUnknownMethod, 0: CodeParse, 4: CodeInternal, 5: "NOT_UBL", 6: CodeInternal, 7: CodeInvalidParams}
	for id, code := range checks {
		if got[id].Error == nil || got[id].Error.Code != code {
			t.Errorf("id %d: hata kodu %v, beklenen %s", id, got[id].Error, code)
		}
	}
}

func TestEmit(t *testing.T) {
	var b strings.Builder
	s := NewServer(&b)
	if err := s.Emit("ready", map[string]string{"v": "1"}); err != nil {
		t.Fatal(err)
	}
	if b.String() != `{"event":"ready","data":{"v":"1"}}`+"\n" {
		t.Errorf("olay: %q", b.String())
	}
}
