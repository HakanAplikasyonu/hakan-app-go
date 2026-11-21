package netgsm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestParseReportShapes(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want []ReportItem
	}{
		{
			name: "jobs list with numeric status",
			raw:  `{"code":"00","description":"ok","jobs":[{"jobid":"111","telno":"5321234567","status":1,"deliveredDate":"20.11.2026 14:05:09","errorCode":0},{"jobid":"222","telno":"5329998877","status":3,"errorCode":"3"}]}`,
			want: []ReportItem{
				{JobID: "111", Phone: "5321234567", Status: "1", Delivery: DeliveryDelivered},
				{JobID: "222", Phone: "5329998877", Status: "3", Delivery: DeliveryFailed, ErrorCode: "3"},
			},
		},
		{
			name: "bare array with camelCase keys",
			raw:  `[{"jobId":"333","status":"0"}]`,
			want: []ReportItem{{JobID: "333", Status: "0", Delivery: DeliveryWaiting}},
		},
		{
			name: "nested under data",
			raw:  `{"code":"00","data":{"jobs":[{"bulkId":"444","status":"2"}]}}`,
			want: []ReportItem{{JobID: "444", Status: "2", Delivery: DeliveryFailed}},
		},
		{
			name: "unknown status stays waiting, missing job id skipped",
			raw:  `{"jobs":[{"jobid":"555","status":"77"},{"status":"1"}]}`,
			want: []ReportItem{{JobID: "555", Status: "77", Delivery: DeliveryWaiting}},
		},
		{name: "empty", raw: ``, want: []ReportItem{}},
		{name: "no list", raw: `{"code":"00","description":"ok"}`, want: []ReportItem{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseReport([]byte(tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
			for i := range got {
				g, w := got[i], tc.want[i]
				if g.JobID != w.JobID || g.Phone != w.Phone || g.Status != w.Status || g.Delivery != w.Delivery || g.ErrorCode != w.ErrorCode {
					t.Fatalf("item %d = %+v, want %+v", i, g, w)
				}
			}
		})
	}
}

func TestParseReportDeliveredTimeIsTurkishLocal(t *testing.T) {
	got, err := ParseReport([]byte(`{"jobs":[{"jobid":"1","status":1,"deliveredDate":"20.11.2026 14:05:09"}]}`))
	if err != nil || len(got) != 1 || got[0].DeliveredAt == nil {
		t.Fatalf("got %+v, %v", got, err)
	}
	want := time.Date(2026, 11, 20, 11, 5, 9, 0, time.UTC)
	if !got[0].DeliveredAt.Equal(want) {
		t.Fatalf("delivered at %v, want %v", got[0].DeliveredAt.UTC(), want)
	}
	for _, s := range []string{"2026-11-20 14:05:09", "2026-11-20T11:05:09Z"} {
		raw, _ := json.Marshal(map[string]any{"jobs": []any{map[string]any{"jobid": "1", "status": 1, "deliveredDate": s}}})
		got, _ := ParseReport(raw)
		if got[0].DeliveredAt == nil || !got[0].DeliveredAt.Equal(want) {
			t.Fatalf("%q -> %v", s, got[0].DeliveredAt)
		}
	}
}

func TestParseReportRejectsNonJSON(t *testing.T) {
	if _, err := ParseReport([]byte("<html>")); err == nil {
		t.Fatal("non-JSON accepted")
	}
}

func TestReportCall(t *testing.T) {
	var body reportBody
	c := fake(t, func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != cred.Usercode || p != cred.Password || r.URL.Path != "/sms/rest/v2/report" || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = io.WriteString(w, `{"code":"00","jobs":[{"jobid":"17723","telno":"5321234567","status":"1","deliveredDate":"21.11.2026 09:00:00"}]}`)
	})
	got, err := c.Report(context.Background(), cred, []string{"17723", " ", "17724"})
	if err != nil || len(got) != 1 || got[0].Delivery != DeliveryDelivered {
		t.Fatalf("report = %+v, %v", got, err)
	}
	if len(body.BulkIDs) != 2 || body.BulkIDs[0] != "17723" {
		t.Fatalf("body = %+v", body)
	}
	_, err = c.Report(context.Background(), Credentials{Usercode: "1", Password: "x"}, []string{"1"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Code != "30" {
		t.Fatalf("wrong password = %v", err)
	}
}

func TestReportNoRecordAndLimits(t *testing.T) {
	calls := 0
	c := fake(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = io.WriteString(w, `{"code":"60","description":"no record"}`)
	})
	got, err := c.Report(context.Background(), cred, []string{"1"})
	if err != nil || len(got) != 0 {
		t.Fatalf("no record = %+v, %v", got, err)
	}
	if got, err := c.Report(context.Background(), cred, nil); err != nil || len(got) != 0 || calls != 1 {
		t.Fatalf("empty ids must not call NetGSM: %v %v calls=%d", got, err, calls)
	}
	many := make([]string, MaxReportJobs+1)
	for i := range many {
		many[i] = "x"
	}
	if _, err := c.Report(context.Background(), cred, many); err == nil {
		t.Fatal("too many job ids accepted")
	}
}

func TestStatusMessage(t *testing.T) {
	if StatusMessage("3") == "" || StatusMessage("999") == "" || StatusMessage("") == "" {
		t.Fatal("empty message")
	}
}
