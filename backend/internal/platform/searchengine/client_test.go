package searchengine

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/config"
)

// fakeMeili answers the Meilisearch HTTP calls the client makes and records
// each request as "METHOD path?query body" (body JSON with sorted keys).
type fakeMeili struct {
	mu   sync.Mutex
	reqs []string
}

func (f *fakeMeili) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	body := ""
	if len(raw) > 0 {
		var v any
		if json.Unmarshal(raw, &v) == nil {
			b, _ := json.Marshal(v) // map keys are sorted
			body = " " + string(b)
		} else {
			body = " " + string(raw)
		}
	}
	line := r.Method + " " + r.URL.Path
	if r.URL.RawQuery != "" {
		line += "?" + r.URL.RawQuery
	}
	f.mu.Lock()
	f.reqs = append(f.reqs, line+body)
	f.mu.Unlock()

	task := `{"taskUid":1,"indexUid":"hakan_users","status":"enqueued","type":"documentAdditionOrUpdate","enqueuedAt":"2026-11-16T00:00:00Z"}`
	hit := `{"id":"u1","spec":"users","title":"Ayşe Demir","subtitle":"ayse@example.test","href":"/platform/users/u1"}`
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/health":
		_, _ = io.WriteString(w, `{"status":"available"}`)
	case strings.HasPrefix(r.URL.Path, "/tasks/"):
		_, _ = io.WriteString(w, `{"uid":1,"indexUid":"hakan_users","status":"succeeded","type":"documentAdditionOrUpdate","enqueuedAt":"2026-11-16T00:00:00Z"}`)
	case r.URL.Path == "/multi-search":
		_, _ = io.WriteString(w, `{"results":[{"indexUid":"hakan_users","hits":[`+hit+`],"query":"ay","processingTimeMs":1,"limit":5,"offset":0,"estimatedTotalHits":1}]}`)
	case strings.HasSuffix(r.URL.Path, "/search"):
		_, _ = io.WriteString(w, `{"hits":[`+hit+`],"query":"ay","processingTimeMs":1,"limit":5,"offset":0,"estimatedTotalHits":1}`)
	case strings.HasSuffix(r.URL.Path, "/stats"):
		_, _ = io.WriteString(w, `{"numberOfDocuments":3,"isIndexing":false,"fieldDistribution":{}}`)
	default:
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, task)
	}
}

// TestClientRequests pins the HTTP requests the client sends for each
// operation (#223: meilisearch-go 0.31 → 0.37 renamed the *WithContext
// methods; the requests must stay the same). The expected list was taken
// from the client on meilisearch-go 0.31.
func TestClientRequests(t *testing.T) {
	f := &fakeMeili{}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := NewClient(config.SearchConfig{Enabled: true, Driver: "meilisearch", MeiliHost: srv.URL, MeiliKey: "k", IndexPrefix: "hakan"}, nil)
	ctx := context.Background()

	if err := c.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if err := c.EnsureIndex(ctx, Spec{ID: "users", Searchable: []string{"title", "subtitle"}, Filterable: []string{"role"}}); err != nil {
		t.Fatalf("ensure index: %v", err)
	}
	if err := c.UpsertDocuments(ctx, "users", []Document{{ID: "u1", Spec: "users", Title: "Ayşe Demir", Href: "/platform/users/u1"}}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := c.DeleteDocument(ctx, "users", "u1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	one, err := c.Search(ctx, []string{"users"}, "ay", 5)
	if err != nil || len(one) != 1 || one[0].ID != "u1" || one[0].Spec != "users" || one[0].Title != "Ayşe Demir" {
		t.Fatalf("search one spec: %+v %v", one, err)
	}
	many, err := c.Search(ctx, []string{"users", "roles"}, "ay", 5)
	if err != nil || len(many) != 1 || many[0].Href != "/platform/users/u1" {
		t.Fatalf("multi search: %+v %v", many, err)
	}
	n, err := c.Stats(ctx, "users")
	if err != nil || n != 3 {
		t.Fatalf("stats: %d %v", n, err)
	}

	if got, want := strings.Join(f.reqs, "\n"), strings.Join(wantRequests, "\n"); got != want {
		t.Errorf("requests changed:\n got:\n%s\nwant:\n%s", got, want)
	}
}

var wantRequests = []string{
	"GET /health",
	"POST /indexes {\"primaryKey\":\"id\",\"uid\":\"hakan_users\"}",
	"GET /tasks/1",
	"PATCH /indexes/hakan_users/settings {\"displayedAttributes\":[\"id\",\"spec\",\"title\",\"subtitle\",\"keywords\",\"href\",\"icon\"],\"filterableAttributes\":[\"spec\",\"role\"],\"searchableAttributes\":[\"title\",\"subtitle\"]}",
	"GET /tasks/1",
	"POST /indexes/hakan_users/documents [{\"href\":\"/platform/users/u1\",\"id\":\"u1\",\"spec\":\"users\",\"title\":\"Ayşe Demir\"}]",
	"GET /tasks/1",
	"DELETE /indexes/hakan_users/documents/u1",
	"GET /tasks/1",
	"POST /indexes/hakan_users/search {\"hybrid\":null,\"limit\":5,\"q\":\"ay\"}",
	"POST /multi-search {\"queries\":[{\"hybrid\":null,\"indexUid\":\"hakan_users\",\"limit\":5,\"q\":\"ay\"},{\"hybrid\":null,\"indexUid\":\"hakan_roles\",\"limit\":5,\"q\":\"ay\"}]}",
	"GET /indexes/hakan_users/stats",
}
