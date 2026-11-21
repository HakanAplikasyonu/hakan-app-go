package netgsm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// MaxReportJobs is the most job ids one report request carries.
const MaxReportJobs = 100

// Delivery is the delivery state NetGSM reports for one message.
type Delivery int

const (
	// DeliveryWaiting: not delivered yet (the operator has not answered).
	DeliveryWaiting Delivery = iota
	// DeliveryDelivered: the operator delivered the message to the phone.
	DeliveryDelivered
	// DeliveryFailed: the message will not be delivered (timeout, wrong
	// number, operator refused, …).
	DeliveryFailed
)

// ReportItem is one message of a report answer.
type ReportItem struct {
	JobID string
	Phone string
	// Status is NetGSM's raw status code ("" when the answer had none).
	Status   string
	Delivery Delivery
	// DeliveredAt is the operator's delivery time when NetGSM gave one.
	DeliveredAt *time.Time
	// ErrorCode is NetGSM's error code for a failed delivery ("" if none).
	ErrorCode string
}

// StatusMessage explains a report status code in Turkish.
func StatusMessage(status string) string {
	switch status {
	case "0":
		return "Operatörden iletim bilgisi bekleniyor."
	case "1":
		return "Mesaj telefona iletildi."
	case "2":
		return "Zaman aşımı: mesaj geçerlilik süresi içinde telefona iletilemedi."
	case "3":
		return "Hatalı ya da kullanılmayan numara."
	case "4":
		return "Mesaj operatöre gönderilemedi."
	case "11":
		return "Operatör mesajı kabul etmedi."
	case "12":
		return "Gönderim hatası."
	case "13":
		return "Mükerrer gönderim, mesaj iletilmedi."
	case "103":
		return "Gönderim görevi başarısız."
	}
	if status == "" {
		return "İletilemedi."
	}
	return "İletilemedi (NetGSM durum kodu " + status + ")."
}

// deliveryOf maps NetGSM's report status to a delivery state. Unknown
// codes stay waiting: a later poll may know more, and after the polling
// window the message simply stays "sent".
func deliveryOf(status string) Delivery {
	switch status {
	case "1":
		return DeliveryDelivered
	case "2", "3", "4", "11", "12", "13", "103":
		return DeliveryFailed
	}
	return DeliveryWaiting
}

type reportBody struct {
	BulkIDs []string `json:"bulkIds"`
}

// Report asks NetGSM for the delivery state of the given job ids (at most
// MaxReportJobs). It only reads; a job NetGSM does not know is simply
// missing from the answer.
func (c *Client) Report(ctx context.Context, cred Credentials, jobIDs []string) ([]ReportItem, error) {
	ids := make([]string, 0, len(jobIDs))
	for _, id := range jobIDs {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return []ReportItem{}, nil
	}
	if len(ids) > MaxReportJobs {
		return nil, fmt.Errorf("netgsm: at most %d job ids per report", MaxReportJobs)
	}
	raw, err := json.Marshal(reportBody{BulkIDs: ids})
	if err != nil {
		return nil, err
	}
	a, err := c.do(ctx, cred, http.MethodPost, "/sms/rest/v2/report", raw)
	if err != nil {
		return nil, err
	}
	switch a.Code {
	case "", "00":
	case "60":
		// "No record for these jobs": nothing to report yet.
		return []ReportItem{}, nil
	default:
		return nil, &APIError{Code: a.Code, Description: a.Description}
	}
	return ParseReport(a.Raw)
}

// ParseReport reads a report answer defensively: the job list may be the
// body itself or under "jobs" / "data" / "reports" / "list" (possibly one
// level deeper), keys may be camelCase or lower case, the status a number
// or a string, the delivery time Turkish local time in several layouts.
func ParseReport(raw []byte) ([]ReportItem, error) {
	raw = bytes.TrimSpace(raw)
	out := []ReportItem{}
	if len(raw) == 0 {
		return out, nil
	}
	var root any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil {
		return nil, fmt.Errorf("netgsm: report answer is not JSON: %w", err)
	}
	for _, o := range jobList(root, 0) {
		it := ReportItem{
			JobID:     pick(o, "jobid", "jobId", "job_id", "bulkid", "bulkId", "bulk_id", "id"),
			Phone:     pick(o, "telno", "gsmno", "phone", "no", "number"),
			Status:    pick(o, "status", "state", "durum"),
			ErrorCode: pick(o, "errorCode", "errorcode", "error_code", "error"),
		}
		if it.ErrorCode == "0" {
			it.ErrorCode = ""
		}
		if it.JobID == "" {
			continue
		}
		it.Delivery = deliveryOf(it.Status)
		if t, ok := parseTime(pick(o, "deliveredDate", "delivereddate", "deliveryDate", "deliverydate", "delivered_at", "deliveredAt", "deliverDate")); ok {
			it.DeliveredAt = &t
		}
		out = append(out, it)
	}
	return out, nil
}

// jobList finds the list of job objects in the answer.
func jobList(v any, depth int) []map[string]any {
	switch x := v.(type) {
	case []any:
		objs := make([]map[string]any, 0, len(x))
		for _, e := range x {
			if o, ok := e.(map[string]any); ok {
				objs = append(objs, o)
			}
		}
		return objs
	case map[string]any:
		if depth > 2 {
			return nil
		}
		for _, k := range []string{"jobs", "Jobs", "data", "reports", "report", "list", "items", "result"} {
			if inner, ok := x[k]; ok {
				if objs := jobList(inner, depth+1); len(objs) > 0 {
					return objs
				}
			}
		}
		// A single job object.
		if pick(x, "jobid", "jobId", "bulkid", "bulkId") != "" {
			return []map[string]any{x}
		}
	}
	return nil
}

// pick returns the first key present as a trimmed string.
func pick(o map[string]any, keys ...string) string {
	for _, k := range keys {
		v, ok := o[k]
		if !ok || v == nil {
			continue
		}
		var s string
		switch x := v.(type) {
		case string:
			s = x
		case json.Number:
			s = x.String()
		case float64:
			s = strconv.FormatFloat(x, 'f', -1, 64)
		case bool:
			s = strconv.FormatBool(x)
		default:
			continue
		}
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return ""
}

// turkey is NetGSM's clock: Türkiye has been UTC+3 all year since 2016.
var turkey = time.FixedZone("TRT", 3*60*60)

var timeLayouts = []string{
	"02.01.2006 15:04:05",
	"02.01.2006 15:04",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"02/01/2006 15:04:05",
	"020120061504",
	"02012006150405",
}

func parseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	for _, l := range timeLayouts {
		if t, err := time.ParseInLocation(l, s, turkey); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
