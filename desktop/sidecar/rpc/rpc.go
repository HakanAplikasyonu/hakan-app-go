// Package rpc, Tauri kabuğu ile Go sidecar arasındaki satır tabanlı JSON
// protokolüdür (stdin/stdout).
//
// Her satır tek bir JSON nesnesidir:
//
//	istek:  {"id":1,"method":"ping","params":{}}
//	yanıt:  {"id":1,"result":{...}}  ya da  {"id":1,"error":{"code":"...","message":"..."}}
//	olay:   {"event":"ready","data":{...}}   (id yok; sidecar kendiliğinden yollar)
//
// İstekler eşzamanlı işlenir; yanıtların sırası isteklerin sırasını
// izlemeyebilir, kabuk yanıtı id ile eşler. stdout yalnızca protokol içindir;
// günlükler stderr'e yazılır.
package rpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// MaxLine bir protokol satırının üst sınırıdır.
const MaxLine = 4 << 20

// Hata kodları. Kabuk bunları kullanıcı mesajına çevirir.
const (
	CodeParse          = "PARSE_ERROR"
	CodeUnknownMethod  = "UNKNOWN_METHOD"
	CodeInvalidParams  = "INVALID_PARAMS"
	CodeInternal       = "INTERNAL_ERROR"
	CodeUnavailable    = "UNAVAILABLE"
	CodeNotImplemented = "NOT_IMPLEMENTED"
)

type Request struct {
	ID     uint64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Errorf, kodlu bir protokol hatası üretir.
func Errorf(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

type response struct {
	ID     uint64 `json:"id"`
	Result any    `json:"result,omitempty"`
	Error  *Error `json:"error,omitempty"`
}

type event struct {
	Event string `json:"event"`
	Data  any    `json:"data,omitempty"`
}

// Handler bir yöntemi karşılar. Dönen hata *Error değilse INTERNAL_ERROR olur.
type Handler func(ctx context.Context, params json.RawMessage) (any, error)

type Server struct {
	handlers map[string]Handler
	mu       sync.Mutex
	enc      *json.Encoder
}

func NewServer(out io.Writer) *Server {
	return &Server{handlers: map[string]Handler{}, enc: json.NewEncoder(out)}
}

func (s *Server) Handle(method string, h Handler) { s.handlers[method] = h }

// Methods kayıtlı yöntem adlarını döner.
func (s *Server) Methods() []string {
	out := make([]string, 0, len(s.handlers))
	for m := range s.handlers {
		out = append(out, m)
	}
	return out
}

func (s *Server) write(v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enc.Encode(v)
}

// Emit kabuğa kendiliğinden bir olay yollar.
func (s *Server) Emit(name string, data any) error {
	return s.write(event{Event: name, Data: data})
}

// Serve, girdi kapanana (kabuk süreci bitene) kadar istekleri okur. Girdi
// kapanınca bekleyen isteklerin bitmesini bekler ve nil döner.
func (s *Server) Serve(ctx context.Context, in io.Reader) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64<<10), MaxLine)
	var wg sync.WaitGroup
	defer wg.Wait()
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			_ = s.write(response{Error: Errorf(CodeParse, "geçersiz JSON: %v", err)})
			continue
		}
		h, ok := s.handlers[req.Method]
		if !ok {
			_ = s.write(response{ID: req.ID, Error: Errorf(CodeUnknownMethod, "bilinmeyen yöntem: %q", req.Method)})
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.dispatch(ctx, req, h)
		}()
	}
	return sc.Err()
}

func (s *Server) dispatch(ctx context.Context, req Request, h Handler) {
	defer func() {
		if r := recover(); r != nil {
			_ = s.write(response{ID: req.ID, Error: Errorf(CodeInternal, "beklenmeyen hata: %v", r)})
		}
	}()
	res, err := h(ctx, req.Params)
	if err != nil {
		var pe *Error
		if !errors.As(err, &pe) {
			pe = Errorf(CodeInternal, "%v", err)
		}
		_ = s.write(response{ID: req.ID, Error: pe})
		return
	}
	if res == nil {
		res = struct{}{}
	}
	_ = s.write(response{ID: req.ID, Result: res})
}

// Decode params'ı hedefe çözer; hata INVALID_PARAMS olur.
func Decode(params json.RawMessage, dst any) error {
	if len(params) == 0 {
		params = []byte("{}")
	}
	if err := json.Unmarshal(params, dst); err != nil {
		return Errorf(CodeInvalidParams, "parametreler çözülemedi: %v", err)
	}
	return nil
}
