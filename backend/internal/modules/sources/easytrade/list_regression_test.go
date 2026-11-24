package easytrade

// Regression (#780): an expense entered in Easytrade's mobile app today did
// not reach the office. The expense list was asked with date_from /
// date_to, which Easytrade's list endpoints do not accept (only search,
// sort, direction, page, per_page and filters[...]); the manual sync read
// only the picked period (usually the previous month). These tests use an
// httptest fake that behaves like Easytrade's documented lists: unknown
// parameters ignored, filters[<date>][start|end] compared against a
// date-time, X-Branch-ID scoping, meta paging. All data is made up.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeExpense is one made-up expense of the fake.
type fakeExpense struct {
	id                   int
	branch               string
	expenseDate, created string // RFC 3339, like Laravel's date-time casts
	amount               string
}

// expenseFake lists /company/expenses like Easytrade: X-Branch-ID scopes
// the list; filters[expense_date][start|end] (Y-m-d) compare against the
// full date-time (end = that day's 00:00, as a plain whereBetween would);
// sort=created_at / direction work; date_from / date_to are ignored.
// pageCap > 0 serves fewer rows than per_page; noMeta leaves meta out.
type expenseFake struct {
	mu       sync.Mutex
	rows     []fakeExpense
	branches []string
	pageCap  int
	noMeta   bool
	queries  []string
}

func (f *expenseFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	js := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	page = max(page, 1)
	per, _ := strconv.Atoi(q.Get("per_page"))
	if per <= 0 || per > 100 {
		per = 15
	}
	if f.pageCap > 0 {
		per = min(per, f.pageCap)
	}
	switch {
	case r.URL.Path == "/company/branches":
		var all []map[string]any
		for _, b := range f.branches {
			n, _ := strconv.Atoi(b)
			all = append(all, map[string]any{"id": n, "name": "Şube " + b})
		}
		lo, hi := min((page-1)*per, len(all)), min(page*per, len(all))
		js(map[string]any{"data": all[lo:hi], "meta": map[string]any{"current_page": page, "last_page": max((len(all)+per-1)/per, 1), "per_page": per, "total": len(all)}})
	case r.URL.Path == "/company/expenses":
		f.queries = append(f.queries, r.URL.RawQuery)
		var rows []fakeExpense
		start, end := q.Get("filters[expense_date][start]"), q.Get("filters[expense_date][end]")
		for _, e := range f.rows {
			if e.branch != r.Header.Get("X-Branch-ID") {
				continue
			}
			at, _ := time.Parse(time.RFC3339Nano, e.expenseDate)
			if s, err := time.Parse(time.DateOnly, start); err == nil && at.Before(s) {
				continue
			}
			if en, err := time.Parse(time.DateOnly, end); err == nil && at.After(en) {
				continue
			}
			rows = append(rows, e)
		}
		key := func(e fakeExpense) string { return e.expenseDate }
		if q.Get("sort") == "created_at" {
			key = func(e fakeExpense) string { return e.created }
		}
		desc := q.Get("direction") != "asc"
		sort.Slice(rows, func(i, j int) bool {
			if key(rows[i]) != key(rows[j]) {
				return (key(rows[i]) > key(rows[j])) == desc
			}
			return (rows[i].id > rows[j].id) == desc
		})
		lo, hi := min((page-1)*per, len(rows)), min(page*per, len(rows))
		data := []map[string]any{}
		for _, e := range rows[lo:hi] {
			data = append(data, map[string]any{"id": e.id, "expense_date": e.expenseDate, "created_at": e.created})
		}
		out := map[string]any{"success": true, "data": data}
		if !f.noMeta {
			out["meta"] = map[string]any{"current_page": page, "last_page": max((len(rows)+per-1)/per, 1), "per_page": per, "total": len(rows)}
		}
		js(out)
	case strings.HasPrefix(r.URL.Path, "/company/expenses/"):
		id, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/company/expenses/"))
		for _, e := range f.rows {
			if e.id == id {
				js(map[string]any{"data": map[string]any{"id": e.id, "expense_date": e.expenseDate, "amount": e.amount,
					"tax_rate": "20_kdv", "status": "pending", "recurrence_type": "none", "is_recurring": false, "is_main_expense": true,
					"description": "market fişi", "branch_id": e.branch}})
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func fakeClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, srv.Client(), "42", Token{Value: "tok", ExpiresAt: time.Now().Add(time.Hour)})
	c.Interval = 0
	return c
}

// oldExpenses are n made-up expenses of 2025 on branch 1.
func oldExpenses(n int) []fakeExpense {
	var out []fakeExpense
	base := time.Date(2025, 3, 1, 9, 0, 0, 0, time.UTC)
	for i := range n {
		at := base.Add(time.Duration(i) * 25 * time.Hour).Format(time.RFC3339Nano)
		out = append(out, fakeExpense{id: 1000 + i, branch: "1", expenseDate: at, created: at, amount: "10.00"})
	}
	return out
}

// An expense entered today at 14:42 Istanbul (11:42 UTC) is read by today's
// scheduled window, even though the end filter compares against midnight
// and 250 older expenses are in the list too.
func TestExpenseCreatedTodayIsRead(t *testing.T) {
	now := time.Date(2026, 11, 24, 15, 0, 0, 0, istanbul)
	today := fakeExpense{id: 7001, branch: "1", expenseDate: "2026-11-24T11:42:00.000000Z", created: "2026-11-24T11:43:10.000000Z", amount: "318.40"}
	// Entered at 00:30 on 1 November Istanbul: 31 October in UTC.
	edge := fakeExpense{id: 7002, branch: "1", expenseDate: "2026-10-31T21:30:00.000000Z", created: "2026-10-31T21:31:00.000000Z", amount: "55.00"}
	f := &expenseFake{rows: append(oldExpenses(250), today, edge), branches: []string{"1"}}
	c := fakeClient(t, f)
	from, to := openPeriods(now)
	ws, _ := spans([2]time.Time{from, to})
	if len(ws) != 2 || !ws[1].From.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) || !ws[1].To.Equal(time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("windows %+v", ws)
	}
	nov := ws[1]
	b, read, err := c.ReadItemStats(context.Background(), ItemExpenses, []Branch{{ID: "1"}}, nov.From, nov.To.AddDate(0, 0, -1))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, e := range b.Expenses {
		got[e.SourceID] = e.Amount.StringFixed(2) + "@" + e.Date.Format(time.DateOnly)
	}
	if got["7001"] != "318.40@2026-11-24" {
		t.Fatalf("today's expense not read: %v (read %+v)", got, read)
	}
	if got["7002"] != "55.00@2026-11-01" {
		t.Fatalf("00:30 on the 1st belongs to November in Istanbul: %v", got)
	}
	if len(b.Expenses) != 2 || read.Read != 2 {
		t.Fatalf("expenses %v, read %+v", got, read)
	}
	for _, q := range f.queries {
		if strings.Contains(q, "date_from") || strings.Contains(q, "date_to") || !strings.Contains(q, "filters%5Bexpense_date%5D%5Bend%5D=2026-11-25") {
			t.Fatalf("query %s", q)
		}
	}
}

// Every page is read: with paging meta, with a server that serves fewer
// rows than per_page, and with no meta at all (the walk used to stop after
// a short first page).
func TestListReadsAllPages(t *testing.T) {
	for name, f := range map[string]*expenseFake{
		"meta":            {},
		"capped page":     {pageCap: 15},
		"capped, no meta": {pageCap: 15, noMeta: true},
	} {
		t.Run(name, func(t *testing.T) {
			f.branches = []string{"1"}
			for i := range 230 {
				at := time.Date(2026, 11, 1+i%20, 8, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Minute).Format(time.RFC3339Nano)
				f.rows = append(f.rows, fakeExpense{id: 1 + i, branch: "1", expenseDate: at, created: at, amount: "1.00"})
			}
			c := fakeClient(t, f)
			c.BranchID = "1"
			from, to := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
			ids, err := c.listIDs(context.Background(), "/company/expenses", "expense_date", from, to)
			if err != nil {
				t.Fatal(err)
			}
			if len(ids) != 230 {
				t.Fatalf("%d of 230 ids over %d requests", len(ids), len(f.queries))
			}
		})
	}
}

// A server that ignores page and repeats it stops the walk instead of
// looping.
func TestListStopsOnRepeatedPage(t *testing.T) {
	n := 0
	c := fakeClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"id": 1, "expense_date": "2026-11-03"}}})
	}))
	from, to := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	ids, err := c.listIDs(context.Background(), "/company/expenses", "expense_date", from, to)
	if err != nil || len(ids) != 1 || n != 2 {
		t.Fatalf("ids %v err %v requests %d", ids, err, n)
	}
}

// Each branch is read with its X-Branch-ID and the branch list is paged
// like every list, so an expense entered on the second branch (on the
// branch list's second page) is read. Behaviour: Easytrade scopes the
// expense list by X-Branch-ID; every branch the company lists is walked and
// a record is read once even when several branches list it.
func TestOtherBranchNotMissed(t *testing.T) {
	f := &expenseFake{branches: []string{"1", "2"}, pageCap: 1, rows: []fakeExpense{
		{id: 501, branch: "1", expenseDate: "2026-11-20T09:00:00.000000Z", created: "2026-11-20T09:00:00.000000Z", amount: "12.00"},
		{id: 502, branch: "2", expenseDate: "2026-11-24T11:42:00.000000Z", created: "2026-11-24T11:42:00.000000Z", amount: "318.40"},
	}}
	c := fakeClient(t, f)
	brs, err := c.ActiveBranches(context.Background())
	if err != nil || len(brs) != 2 || brs[1].ID != "2" {
		t.Fatalf("branches %+v %v", brs, err)
	}
	from, to := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	b, read, err := c.ReadItemStats(context.Background(), ItemExpenses, brs, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Expenses) != 2 || read.Read != 2 || read.Listed != 2 {
		t.Fatalf("expenses %+v read %+v", b.Expenses, read)
	}
	if c.BranchID != "" {
		t.Fatalf("branch header left set: %q", c.BranchID)
	}
}

// The manual sync reads the picked period and the open periods; disjoint
// ranges stay apart, overlapping ones merge.
func TestManualSyncSpans(t *testing.T) {
	now := time.Date(2026, 11, 24, 15, 0, 0, 0, istanbul)
	of, ot := openPeriods(now)
	d := func(m time.Month, day int) time.Time { return time.Date(2026, m, day, 0, 0, 0, 0, time.UTC) }
	ws, days := spans([2]time.Time{d(10, 1), d(10, 31)}, [2]time.Time{of, ot})
	if len(ws) != 2 || len(days) != 1 || days[0] != (DayRange{From: "2026-10-01", To: "2026-11-24"}) {
		t.Fatalf("october: %+v %+v", ws, days)
	}
	ws, days = spans([2]time.Time{d(3, 1), d(3, 31)}, [2]time.Time{of, ot})
	if len(ws) != 3 || len(days) != 2 || days[0] != (DayRange{From: "2026-03-01", To: "2026-03-31"}) || days[1].To != "2026-11-24" {
		t.Fatalf("march: %+v %+v", ws, days)
	}
}

func TestParseDay(t *testing.T) {
	for in, want := range map[string]string{
		"2026-11-24":                  "2026-11-24",
		"2026-11-24 14:30:00":         "2026-11-24",
		"2026-11-24T11:42:00.000000Z": "2026-11-24",
		"2026-11-24T21:30:00.000000Z": "2026-11-25", // 00:30 Istanbul
		"2026-11-24T00:00:00+03:00":   "2026-11-24",
		"24.11.2026":                  "2026-11-24",
	} {
		got, ok := parseDay(in)
		if !ok || got.Format(time.DateOnly) != want {
			t.Errorf("%q → %v %v, want %s", in, got, ok, want)
		}
	}
	if _, ok := parseDay("dün"); ok {
		t.Error("garbage parsed")
	}
}

// A one-off expense is not a recurring template even when Easytrade marks
// it as its own main record; a rejected one is excluded, a pending one is
// not.
func TestExpenseExclusions(t *testing.T) {
	for name, c := range map[string]struct {
		body, skip string
	}{
		"one-off main":        {`{"id":1,"expense_date":"2026-11-24","amount":"10","is_main_expense":true,"recurrence_type":"none","is_recurring":false}`, ""},
		"recurring main":      {`{"id":2,"expense_date":"2026-11-24","amount":"10","is_main_expense":true,"recurrence_type":"monthly","is_recurring":true}`, "recurring_template"},
		"main, no recurrence": {`{"id":3,"expense_date":"2026-11-24","amount":"10","is_main_expense":true}`, "recurring_template"},
		"pending":             {`{"id":4,"expense_date":"2026-11-24","amount":"10","status":"pending","status_label":"Onay bekliyor"}`, ""},
		"rejected":            {`{"id":5,"expense_date":"2026-11-24","amount":"10","status":"rejected"}`, "rejected"},
		"rejected label":      {`{"id":6,"expense_date":"2026-11-24","amount":"10","status":"x","status_label":"Reddedildi"}`, "rejected"},
	} {
		if _, skip := mapExpense(payload(t, c.body)); skip != c.skip {
			t.Errorf("%s: skip %q, want %q", name, skip, c.skip)
		}
	}
}
