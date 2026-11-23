package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/config"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/database"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/rls"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/platform/storage"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testdb"
	"github.com/HakanAplikasyonu/hakan-app-go/backend/internal/testfakes"
)

// Write sweep (issue #302). #277 and #281 were 500s on mükellef write paths
// that only manual testing found: the request's row level security scope
// refused a row the handler had to write. This test calls every write
// operation of the mükellef panel (/v1/taxpayer/*) and of the office panel
// (/v1/tenant/*) once, through the real server on a real Postgres with the
// demo office, in the production request scope (DB_RLS_REQUEST_SCOPE), and
// fails on any 5xx.
//
// Every write operation in backend/docs/openapi.yaml under those prefixes
// must have a case below (TestModuleRoutesDocumented keeps the spec in step
// with the router): a new endpoint fails this test until it is listed.
//
// A case gives the body (JSON, or a multipart file) worth sending; path
// parameters are filled from the demo data at call time (paramValue), so a
// later case sees what an earlier one created. Operations run in spec order;
// `late` ones run after them and `last` ones (ending the engagement,
// leaving the office) at the very end. Every 5xx fails: the external
// services (KolayBi, Paraşüt, Easytrade, Vakıfbank, the AI provider) are
// internal/testfakes servers, so their endpoints run their real path (#318).
type sweepCase struct {
	body     string // JSON body; empty sends {}
	file     string // multipart field name; the part is fileName / fileBody
	fileName string
	fileBody string
	fields   map[string]string // extra multipart form fields
	late     bool              // after the others (needs rows they create, or closes one)
	last     bool              // after the late ones (ends the engagement, leaves the office)
}

// receiptPNG is a small valid PNG: the AI fake reads it as a fuel receipt.
// The office's upload is another image, or it would be the taxpayer's
// receipt again (status duplicate) and approve / reject would not run.
var receiptPNG, officeReceiptPNG = pngOf(0xff), pngOf(0xf0)

func pngOf(shade byte) string {
	img := image.NewRGBA(image.Rect(0, 0, 32, 48))
	for i := range img.Pix {
		img.Pix[i] = shade
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.String()
}

var taxpayerWrites = map[string]sweepCase{
	"POST /v1/taxpayer/onboarding-requests/{uuid}/approve": {body: `{"privacy_notice_version":1,"privacy_notice_language":"tr"}`},
	"POST /v1/taxpayer/onboarding-requests/{uuid}/reject":  {},
	"POST /v1/taxpayer/history-shares":                     {body: `{"consent_version":1}`},
	"DELETE /v1/taxpayer/history-shares/{uuid}":            {},
	"POST /v1/taxpayer/receipts":                           {file: "image", fileName: "fis.png", fileBody: receiptPNG},
	"POST /v1/taxpayer/foreign-invoices":                   {file: "file", fileName: "google-ads.pdf", fileBody: "%PDF-1.4\n% yabanci fatura sweep\n", fields: map[string]string{"note": "Ekim reklam faturası"}},
	"POST /v1/taxpayer/receipts/{uuid}/corrections":        {body: `{"corrections":[{"field":"total_amount","value":"1300,00"}]}`},
	"POST /v1/taxpayer/uploads":                            {file: "file", fileName: "atlas-03-tevkifat.xml", fileBody: "@ubl:atlas-03-tevkifat.xml"},
	"POST /v1/taxpayer/document-tasks/{taskUuid}/uploads":  {file: "file", fileName: "fatura.pdf", fileBody: "%PDF-1.4\n% sweep\n"},
	"POST /v1/taxpayer/notifications/read-all":             {},
	"POST /v1/taxpayer/notifications/{uuid}/read":          {},
}

// sweepRange is the sync window of the source connections.
const sweepRange = `{"from":"2026-10-01","to":"2026-10-31"}`

var tenantWrites = map[string]sweepCase{
	"POST /v1/tenant/roles":                                                                 {body: `{"key":"sweep_role","name":"Tarama","permissions":["tenant.engagements.read"]}`},
	"PATCH /v1/tenant/roles/{roleKey}":                                                      {body: `{"name":"Tarama 2"}`},
	"DELETE /v1/tenant/roles/{roleKey}":                                                     {},
	"PATCH /v1/tenant/members/{userUuid}":                                                   {body: `{"role_key":"member"}`},
	"DELETE /v1/tenant/members/{userUuid}":                                                  {late: true},
	"POST /v1/tenant/taxpayer-requests":                                                     {body: `{"kind":"legal","tax_id":"1234567890","title":"Tarama A.Ş.","contact_email":"tarama@example.test"}`},
	"DELETE /v1/tenant/taxpayer-requests/{uuid}":                                            {},
	"POST /v1/tenant/tax-certificates/read":                                                 {file: "file", fileName: "vergi-levhasi.png", fileBody: pngOf(0xe0)},
	"POST /v1/tenant/engagements/{engagementUuid}/invitations":                              {body: `{"email":"calisan@example.test","role":"employee"}`},
	"DELETE /v1/tenant/engagements/{engagementUuid}/invitations/{uuid}":                     {},
	"POST /v1/tenant/engagements/{engagementUuid}/invitations/{uuid}/resend":                {late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/petitions/preview":                        {body: `{}`},
	"POST /v1/tenant/engagements/{engagementUuid}/petitions":                                {body: `{}`},
	"POST /v1/tenant/engagements/{engagementUuid}/end":                                      {last: true},
	"PUT /v1/tenant/engagements/{engagementUuid}/assignments/{userUuid}":                    {},
	"DELETE /v1/tenant/engagements/{engagementUuid}/assignments/{userUuid}":                 {},
	"POST /v1/tenant/leave":                                                                 {last: true},
	"POST /v1/tenant/invitations":                                                           {body: `{"email":"yeni.uye@example.test","role_key":"member"}`},
	"POST /v1/tenant/notifications/read-all":                                                {},
	"POST /v1/tenant/notifications/{uuid}/read":                                             {},
	"DELETE /v1/tenant/invitations/{uuid}":                                                  {},
	"PATCH /v1/tenant/settings":                                                             {body: `{"name":"Demir Mali Müşavirlik (demo)"}`},
	"PUT /v1/tenant/setup":                                                                  {body: `{"steps":{"office":{"status":"done"}},"accounting_program":"luca"}`},
	"POST /v1/tenant/taxpayer-imports":                                                      {file: "file", fileName: "liste.csv", fileBody: "VKN;Unvan;E-posta\n1234567890;Tarama A.Ş.;liste@example.test\n"},
	"POST /v1/tenant/taxpayer-imports/{uuid}/send":                                          {},
	"PUT /v1/tenant/warning-rules/{code}":                                                   {body: `{"enabled":false}`},
	"DELETE /v1/tenant/warning-rules/{code}":                                                {},
	"PUT /v1/tenant/engagements/{engagementUuid}/warning-rules/{code}":                      {body: `{"enabled":false}`},
	"DELETE /v1/tenant/engagements/{engagementUuid}/warning-rules/{code}":                   {},
	"POST /v1/tenant/engagements/{engagementUuid}/warnings/{warningUuid}/acknowledge":       {body: `{"note":"tarama"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/warnings/{warningUuid}/resolve":           {body: `{"note":"tarama"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/warnings/acknowledge-bulk":                {body: `{"code":"TAX_TOTAL_MISMATCH","period":"2026-10","note":"tarama"}`, late: true},
	"POST /v1/tenant/warnings/acknowledge-bulk":                                             {body: `{"code":"TAX_TOTAL_MISMATCH","note":"tarama"}`, late: true},
	"PUT /v1/tenant/review/settings":                                                        {body: `{}`},
	"DELETE /v1/tenant/review/settings/{kind}":                                              {},
	"POST /v1/tenant/engagements/{engagementUuid}/suggestions/{suggestionUuid}/accept":      {late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/suggestions/{suggestionUuid}/reject":      {body: `{"reason":"tarama"}`, late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/suggestions/{suggestionUuid}/correct":     {body: `{"value":{"account_code":"600"},"reason":"tarama"}`, late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/suggestions/{suggestionUuid}/revert":      {body: `{"reason":"tarama"}`, late: true},
	"PUT /v1/tenant/engagements/{engagementUuid}/review-settings":                           {body: `{}`},
	"DELETE /v1/tenant/engagements/{engagementUuid}/review-settings/{kind}":                 {},
	"POST /v1/tenant/engagements/{engagementUuid}/excel-imports":                            {file: "file", fileName: "satis.csv", fileBody: "Tarih;Tutar\n01.10.2026;100\n"},
	"PUT /v1/tenant/engagements/{engagementUuid}/excel-imports/{importUuid}/mapping":        {body: `{"data_type":"outgoing_invoices"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/excel-imports/{importUuid}/validate":      {},
	"PATCH /v1/tenant/engagements/{engagementUuid}/excel-imports/{importUuid}/rows/{rowNo}": {body: `{}`},
	"POST /v1/tenant/engagements/{engagementUuid}/excel-imports/{importUuid}/commit":        {},
	"POST /v1/tenant/engagements/{engagementUuid}/excel-imports/{importUuid}/rollback":      {},
	"POST /v1/tenant/bank-statement-templates":                                              {body: `{"format":"csv","name":"Tarama","columns":[{"header":"Gün","field":"date"},{"header":"Tutar","field":"amount"}]}`},
	"POST /v1/tenant/bank-statement-templates/analyze":                                      {file: "file", fileName: "ekstre.csv", fileBody: "Tarih;Açıklama;Tutar;Bakiye\n05.10.2026;Ödeme;-100,00;900,00\n"},
	"DELETE /v1/tenant/bank-statement-templates/{templateUuid}":                             {},
	"DELETE /v1/tenant/excel-import-templates/{templateUuid}":                               {},
	"POST /v1/tenant/engagements/{engagementUuid}/receipts":                                 {file: "image", fileName: "fis.png", fileBody: officeReceiptPNG},
	// WhatsApp Web uzantısı (#388).
	"POST /v1/tenant/whatsapp/pairings":                                                   {},
	"DELETE /v1/tenant/whatsapp/chat-links/{linkUuid}":                                    {late: true},
	"PUT /v1/tenant/engagements/{engagementUuid}/whatsapp/chat-link":                      {body: `{"chat_key":"905551112233@c.us","label":"Atlas Muhasebe"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/whatsapp/captures":                      {file: "file", fileName: "belge.pdf", fileBody: "%PDF-1.4\n% whatsapp sweep\n", fields: map[string]string{"target": "document", "chat_key": "905551112233@c.us"}},
	"PUT /v1/tenant/document-upload-settings":                                             {body: `{"enabled":true}`},
	"POST /v1/tenant/engagements/{engagementUuid}/document-uploads":                       {file: "file", fileName: "evrak.pdf", fileBody: "%PDF-1.4\n% evrak sweep\n", fields: map[string]string{"doc_type": "other", "period": "2026-11", "lock_choice": "correction"}},
	"PUT /v1/tenant/engagements/{engagementUuid}/whatsapp/captures/{captureUuid}/task":    {body: `{"task_uuid":"00000000-0000-0000-0000-000000000001"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/receipts/{receiptUuid}/approve":         {},
	"POST /v1/tenant/engagements/{engagementUuid}/receipts/{receiptUuid}/reject":          {},
	"PUT /v1/tenant/expense-categories":                                                   {body: `{"code":"tarama","name":"Tarama","account_code":"770"}`},
	"DELETE /v1/tenant/expense-categories/{categoryUuid}":                                 {},
	"PUT /v1/tenant/engagements/{engagementUuid}/expense-categories":                      {body: `{"code":"tarama_mukellef","name":"Tarama","account_code":"770"}`},
	"DELETE /v1/tenant/engagements/{engagementUuid}/expense-categories/{categoryUuid}":    {},
	"PATCH /v1/tenant/engagements/{engagementUuid}/expenses/{expenseUuid}":                {body: `{"category_code":"yemek"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/expenses/{expenseUuid}/approve":         {late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/expenses/{expenseUuid}/reject":          {late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/retail-sales/continuity":                {},
	"PATCH /v1/tenant/engagements/{engagementUuid}/retail-sales/{retailSaleUuid}":         {body: `{"receipt_count":12}`},
	"POST /v1/tenant/engagements/{engagementUuid}/retail-sales/{retailSaleUuid}/approve":  {late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/retail-sales/{retailSaleUuid}/reject":   {late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/expenses/import":                        {file: "file", fileName: "masraf.csv", fileBody: "Tarih;Satıcı;Toplam\n01.10.2026;Tarama Kırtasiye;120,00\n"},
	"POST /v1/tenant/engagements/{engagementUuid}/retail-sales/import":                    {file: "file", fileName: "z.csv", fileBody: "Tarih;ÖKC Seri No;Z No;Toplam\n01.10.2026;TR0000001;0001;500,00\n"},
	"POST /v1/tenant/engagements/{engagementUuid}/vehicles":                               {body: `{"plate":"34ABC123"}`},
	"DELETE /v1/tenant/engagements/{engagementUuid}/vehicles/{vehicleUuid}":               {},
	"POST /v1/tenant/engagements/{engagementUuid}/controls/run":                           {body: `{"period":"2026-10"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/report/deliveries":                      {body: `{"period":"2026-10","recipient":"rapor@demo.hakan.local"}`},
	"PUT /v1/tenant/engagements/{engagementUuid}/report/schedule":                         {body: `{"enabled":true,"day_of_month":1,"recipient":"rapor@demo.hakan.local"}`},
	"POST /v1/tenant/report-deliveries":                                                   {body: `{"period":"2026-10","engagement_uuids":["{engagementUuid}"]}`},
	"POST /v1/tenant/copilot/plans":                                                       {body: `{"command":"Atlas Gıda ekim fişlerini Luca'ya aktar","period":"2026-10"}`},
	"POST /v1/tenant/copilot/plans/{planUuid}/steps/{stepIndex}":                          {body: `{"status":"skipped"}`},
	"POST /v1/tenant/copilot/plans/{planUuid}/cancel":                                     {},
	"POST /v1/tenant/engagements/{engagementUuid}/period-summaries":                       {body: `{"period":"2026-10"}`},
	"PUT /v1/tenant/engagements/{engagementUuid}/period-summaries/{summaryUuid}":          {body: `{"body":"Ekim dönemi özeti (tarama)."}`},
	"POST /v1/tenant/engagements/{engagementUuid}/period-summaries/{summaryUuid}/approve": {},
	"POST /v1/tenant/agents":                       {body: `{"machine_id":"sweep-machine-01","name":"Tarama bilgisayarı","platform":"windows","version":"1.0.0"}`},
	"DELETE /v1/tenant/agents/{agentUuid}":         {},
	"POST /v1/tenant/agents/{agentUuid}/heartbeat": {body: `{}`},
	"POST /v1/tenant/agents/{agentUuid}/documents": {file: "file", fileName: "mizan-2026-10.csv", fields: map[string]string{"folder": `C:\Luca\Mizan`},
		fileBody: "Hesap Kodu;Hesap Adı;Borç;Alacak;Borç Bakiye;Alacak Bakiye\n100;KASA;10,00;4,00;6,00;0,00\n320;SATICILAR;0,00;5,00;0,00;5,00\n600;SATIŞLAR;0,00;1,00;0,00;1,00\n"},
	"POST /v1/tenant/agent-documents/{agentDocumentUuid}/resolve":                                          {late: true, body: `{"engagement_uuid":"{engagementUuid}","kind":"trial_balance","period":"2026-10"}`},
	"POST /v1/tenant/agent-documents/{agentDocumentUuid}/dismiss":                                          {last: true},
	"POST /v1/tenant/legal/dpa/accept":                                                                     {body: `{"version":1}`},
	"POST /v1/tenant/engagements/{engagementUuid}/ubl-uploads":                                             {file: "file", fileName: "atlas-04-istisna.xml", fileBody: "@ubl:atlas-04-istisna.xml"},
	"POST /v1/tenant/engagements/{engagementUuid}/journal-drafts":                                          {body: `{"entry_date":"2026-10-15T00:00:00Z","description":"Tarama fişi","lines":[{"account":"100","debit":"100.00","credit":"0"},{"account":"600","debit":"0","credit":"100.00"}]}`},
	"PATCH /v1/tenant/engagements/{engagementUuid}/journal-drafts/{draftUuid}/lines/{lineNo}":              {body: `{"account":"770.01"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/journal-drafts/{draftUuid}/approve":                      {},
	"POST /v1/tenant/engagements/{engagementUuid}/journal-drafts/{draftUuid}/reverse":                      {body: `{"reason":"tarama"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/journal-drafts/{draftUuid}/unapprove":                    {body: `{"reason":"tarama"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/journal-exports":                                         {body: `{"driver":"luca","period":"2026-10"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/journal-exports/check":                                   {body: `{"driver":"luca","period":"2026-10"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/journal-exports/{batchUuid}/transfers":                   {},
	"POST /v1/tenant/engagements/{engagementUuid}/journal-transfers/{transferUuid}/claim":                  {body: `{"agent_uuid":"00000000-0000-0000-0000-000000000000"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/journal-transfers/{transferUuid}/delivery":               {body: `{"agent_uuid":"00000000-0000-0000-0000-000000000000","outcome":"failed","error_code":"FOLDER_MISSING"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/journal-transfers/{transferUuid}/complete":               {body: `{"outcome":"imported"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/journal-transfers/{transferUuid}/cancel":                 {},
	"POST /v1/tenant/engagements/{engagementUuid}/journal-transfers/{transferUuid}/retry":                  {},
	"POST /v1/tenant/engagements/{engagementUuid}/privacy-requests":                                        {body: `{"kind":"export","subject_type":"taxpayer","reason":"tarama"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/privacy-requests/{requestUuid}/approve":                  {},
	"POST /v1/tenant/engagements/{engagementUuid}/privacy-requests/{requestUuid}/reject":                   {body: `{"note":"tarama"}`, late: true},
	"PUT /v1/tenant/engagements/{engagementUuid}/journal-export-settings":                                  {body: `{}`},
	"PUT /v1/tenant/journal-export-settings":                                                               {body: `{"luca_pull_integrators":["isnet"]}`},
	"POST /v1/tenant/engagements/{engagementUuid}/bank-matching":                                           {body: `{"period":"2026-10"}`},
	"PUT /v1/tenant/engagements/{engagementUuid}/bank-transactions/{transactionUuid}/matches":              {body: `{"invoice_uuids":["00000000-0000-0000-0000-000000000001"]}`},
	"DELETE /v1/tenant/engagements/{engagementUuid}/bank-transactions/{transactionUuid}/matches":           {},
	"PUT /v1/tenant/engagements/{engagementUuid}/bank-transactions/{transactionUuid}/account":              {body: `{"account_code":"120.01.001"}`},
	"PUT /v1/tenant/engagements/{engagementUuid}/bank-transactions/{transactionUuid}/exclusion":            {body: `{"excluded":true}`},
	"PUT /v1/tenant/engagements/{engagementUuid}/bank-transactions/{transactionUuid}/fx-rate":              {body: `{"rate":"35.00"}`},
	"PUT /v1/tenant/engagements/{engagementUuid}/bank-transactions/{transactionUuid}/transfer":             {body: `{"counterpart_uuid":"00000000-0000-0000-0000-000000000001"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/easytrade-connections":                                   {body: `{"email":"muhasebe@atlas.example.test","password":"` + testfakes.EasytradePassword + `","company_id":"` + testfakes.EasytradeCompanyID + `"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/easytrade-connections/test":                              {body: `{"email":"muhasebe@atlas.example.test","password":"` + testfakes.EasytradePassword + `","company_id":"` + testfakes.EasytradeCompanyID + `"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/easytrade-connections/{connectionUuid}/sync":             {body: sweepRange},
	"PATCH /v1/tenant/engagements/{engagementUuid}/easytrade-connections/{connectionUuid}":                 {body: `{"sync_interval_minutes":60}`},
	"POST /v1/tenant/engagements/{engagementUuid}/kolaybi-connections":                                     {body: `{"api_key":"` + testfakes.KolayBiAPIKey + `"}`},
	"PUT /v1/tenant/engagements/{engagementUuid}/kolaybi-connections/{connectionUuid}/company":             {body: `{"company_id":"` + testfakes.KolayBiCompanyID + `"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/kolaybi-connections/{connectionUuid}/sync":               {body: sweepRange},
	"POST /v1/tenant/engagements/{engagementUuid}/parasut-connections":                                     {body: `{}`},
	"POST /v1/tenant/engagements/{engagementUuid}/parasut-connections/{connectionUuid}/authorize":          {body: `{"code":"sweep-auth-code"}`},
	"PUT /v1/tenant/engagements/{engagementUuid}/parasut-connections/{connectionUuid}/company":             {body: `{"company_id":"` + testfakes.ParasutCompanyID + `"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/parasut-connections/{connectionUuid}/sync":               {body: sweepRange},
	"POST /v1/tenant/engagements/{engagementUuid}/einvoice-connections":                                    {body: `{"integrator_id":"edm","environment":"test"}`},
	"PUT /v1/tenant/engagements/{engagementUuid}/einvoice-connections/{connectionUuid}/credentials":        {body: `{"values":{"username":"` + testfakes.EDMUsername + `","password":"` + testfakes.EDMPassword + `"}}`, late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/einvoice-connections/{connectionUuid}/sync":              {late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/einvoice-connections/{connectionUuid}/test":              {late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/bank-connections":                                        {body: `{"bank_id":"vakifbank","environment":"test"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/bank-connections/{connectionUuid}/submitted":             {},
	"PUT /v1/tenant/engagements/{engagementUuid}/bank-connections/{connectionUuid}/credentials":            {body: `{"values":{"customer_no":"123456789","username":"atlas_ws","password":"sweep-pw"}}`},
	"POST /v1/tenant/engagements/{engagementUuid}/bank-connections/{connectionUuid}/sync":                  {body: `{"from":"2026-11-03","to":"2026-11-03"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/bank-statements":                                         {file: "file", fileName: "ekstre.csv", fileBody: "Tarih;Açıklama;Tutar;Bakiye\n05.10.2026;Ödeme;-100,00;900,00\n"},
	"POST /v1/tenant/engagements/{engagementUuid}/bank-statement-exports":                                  {body: `{"cash_box_uuid":"00000000-0000-0000-0000-000000000000","date_from":"2026-10-01","date_to":"2026-10-31"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/bank-counterparty-aliases":                               {body: `{"match_type":"name","match_value":"Tarama Yazılım A.Ş.","account_code":"120.01.001"}`},
	"PATCH /v1/tenant/engagements/{engagementUuid}/bank-counterparty-aliases/{aliasUuid}":                  {body: `{"account_code":"120.01.002"}`},
	"DELETE /v1/tenant/engagements/{engagementUuid}/bank-counterparty-aliases/{aliasUuid}":                 {},
	"POST /v1/tenant/engagements/{engagementUuid}/bank-counterparty-aliases/{aliasUuid}/promote":           {},
	"POST /v1/tenant/engagements/{engagementUuid}/bank-postings/suggest":                                   {body: `{"period":"2026-10"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/bank-postings/approve":                                   {body: `{"transaction_uuids":["00000000-0000-0000-0000-000000000001"]}`},
	"POST /v1/tenant/engagements/{engagementUuid}/bank-postings/drafts":                                    {body: `{"period":"2026-10"}`},
	"PUT /v1/tenant/engagements/{engagementUuid}/bank-postings/{transactionUuid}":                          {body: `{"account_code":"120.01.001"}`},
	"PUT /v1/tenant/engagements/{engagementUuid}/bank-postings/{transactionUuid}/commission":               {body: `{"amount":"12.50","account_code":"653"}`},
	"PUT /v1/tenant/engagements/{engagementUuid}/bank-ledger-accounts/{cashBoxUuid}":                       {body: `{"account_code":"102.01"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/bank-ledger-difference/resolutions":                      {body: `{"cash_box_uuid":"00000000-0000-0000-0000-000000000000","period":"2026-10","reason":"unexplained","action":"explained","explanation":"Tarama"}`},
	"DELETE /v1/tenant/engagements/{engagementUuid}/bank-ledger-difference/resolutions/{resolutionUuid}":   {},
	"POST /v1/tenant/engagements/{engagementUuid}/cash-boxes":                                              {body: `{"type":"cash","name":"Tarama kasası"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/payroll/employees":                                       {body: `{"full_name":"Tarama Personel","job_title":"Muhasebe"}`},
	"PATCH /v1/tenant/engagements/{engagementUuid}/payroll/employees/{employeeUuid}":                       {body: `{"department":"Finans"}`, late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/payroll/imports":                                         {file: "file", fileName: "bordro.csv", fileBody: "Dönem: 2026/10\n\nAdı Soyadı;TC Kimlik No;Brüt Ücret;SGK İşçi Payı;İşsizlik İşçi Payı;Gelir Vergisi;Damga Vergisi;Net Ücret;SGK İşveren Payı;İşsizlik İşveren Payı\nTarama Kişi;10000000146;30000,00;4200,00;300,00;2000,00;100,00;23400,00;4650,00;600,00\n"},
	"DELETE /v1/tenant/engagements/{engagementUuid}/payroll/runs/{runUuid}":                                {last: true},
	"POST /v1/tenant/engagements/{engagementUuid}/payroll/runs/{runUuid}/journal-draft":                    {late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/payroll/bank-match":                                      {body: `{"period":"2026-10"}`, late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/payroll/sgk-premiums":                                    {body: `{"period":"2026-09","accrual_amount":"1000.00"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/reconciliation/runs":                                     {body: `{"period":"2026-10"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/reconciliation/items/{itemUuid}/review":                  {body: `{"note":"Tarama incelemesi"}`, late: true},
	"PATCH /v1/tenant/engagements/{engagementUuid}/reconciliation/items/{itemUuid}":                        {body: `{"status":"explained","note":"Tarama açıklaması"}`, late: true},
	"PATCH /v1/tenant/engagements/{engagementUuid}/reconciliation/counterparties/{counterpartyUuid}":       {body: `{"status":"explained","note":"Tarama"}`, late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/reconciliation/counterparties/{counterpartyUuid}/review": {body: `{}`, late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/reconciliation/babs":                                     {body: `{"period":"2026-10","form_type":"ba","counterparty_tax_id":"1234567890","counterparty_name":"Tarama","document_count":1,"amount":"6000"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/reconciliation/babs/imports":                             {file: "file", fileName: "ba.csv", fileBody: "VKN;Unvan;Belge Sayısı;Tutar\n1234567891;Tarama İki;2;7.500\n", fields: map[string]string{"period": "2026-10", "form": "ba"}},
	"DELETE /v1/tenant/engagements/{engagementUuid}/reconciliation/babs/{declarationUuid}":                 {last: true},
	"POST /v1/tenant/engagements/{engagementUuid}/reconciliation/letters":                                  {body: `{"counterparty_uuid":"00000000-0000-0000-0000-000000000000"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/reconciliation/letters/{letterUuid}/send":                {body: `{"email":"tarama@example.com"}`, late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/reconciliation/letters/{letterUuid}/response":            {body: `{"response":"agreed"}`, last: true},
	"PATCH /v1/tenant/engagements/{engagementUuid}/payroll/sgk-premiums/{premiumUuid}":                     {body: `{"paid_amount":"1000.00","paid_at":"2026-10-28"}`, late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/payroll/documents":                                       {file: "file", fileName: "bordro.pdf", fileBody: "%PDF-1.4\n% bordro taramasi\n"},
	"DELETE /v1/tenant/engagements/{engagementUuid}/payroll/documents/{documentUuid}":                      {last: true},
	"POST /v1/tenant/engagements/{engagementUuid}/archive-documents": {file: "file", fileName: "isg-egitim.pdf", fileBody: "%PDF-1.4\n% isg taramasi\n",
		fields: map[string]string{"category": "isg", "isg_kind": "isg_training", "valid_until": "2027-01-31"}},
	"PATCH /v1/tenant/engagements/{engagementUuid}/archive-documents/{archiveDocumentUuid}":                                {body: `{"important_kind":"tax_certificate","visible_to_taxpayer":true}`, late: true},
	"DELETE /v1/tenant/engagements/{engagementUuid}/archive-documents/{archiveDocumentUuid}":                               {last: true},
	"PATCH /v1/tenant/engagements/{engagementUuid}/cash-boxes/{cashBoxUuid}":                                               {body: `{"name":"Tarama kasası 2"}`},
	"DELETE /v1/tenant/engagements/{engagementUuid}/bank-statements/{importId}":                                            {},
	"POST /v1/tenant/engagements/{engagementUuid}/account-plan/imports":                                                    {file: "file", fileName: "plan.csv", fileBody: "Hesap Kodu;Hesap Adı\n100;KASA\n"},
	"POST /v1/tenant/engagements/{engagementUuid}/account-plan/check":                                                      {body: `{}`},
	"POST /v1/tenant/engagements/{engagementUuid}/account-plan/missing-template":                                           {body: `{}`},
	"POST /v1/tenant/engagements/{engagementUuid}/account-plan/subaccounts":                                                {body: `{"main_code":"120","customer_uuid":"00000000-0000-0000-0000-000000000000"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/account-plan/subaccounts/generate":                                       {body: `{}`},
	"POST /v1/tenant/engagements/{engagementUuid}/account-plan/luca-files":                                                 {body: `{"kind":"full"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/periods/{period}/lock":                                                   {body: `{}`},
	"POST /v1/tenant/engagements/{engagementUuid}/periods/{period}/unlock":                                                 {body: `{"reason":"tarama"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/change-requests/{changeRequestUuid}/apply":                               {},
	"POST /v1/tenant/engagements/{engagementUuid}/change-requests/{changeRequestUuid}/reject":                              {},
	"POST /v1/tenant/engagements/{engagementUuid}/late-documents/{lateDocumentUuid}/decision":                              {body: `{"status":"ignored","note":"tarama"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/foreign-invoices":                                                        {body: `{"period":"2026-10","vendor":"Tarama Ltd.","country":"IE","amount":"100.00","currency":"USD","exchange_rate":"34.50","service_kind":"yazilim_lisans"}`},
	"PATCH /v1/tenant/engagements/{engagementUuid}/foreign-invoices/{foreignInvoiceUuid}":                                  {body: `{"note":"tarama"}`, late: true},
	"DELETE /v1/tenant/engagements/{engagementUuid}/foreign-invoices/{foreignInvoiceUuid}":                                 {last: true},
	"POST /v1/tenant/engagements/{engagementUuid}/foreign-invoices/{foreignInvoiceUuid}/confirm":                           {late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/foreign-invoices/{foreignInvoiceUuid}/unconfirm":                         {late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/foreign-invoices/journal":                                                {late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/foreign-invoices/{foreignInvoiceUuid}/journal":                           {late: true},
	"DELETE /v1/tenant/engagements/{engagementUuid}/foreign-invoices/{foreignInvoiceUuid}/journal":                         {late: true},
	"PUT /v1/tenant/foreign-service-kinds":                                                                                 {body: `{"enabled":true,"items":[{"service_kind":"reklam","treatment":"kdv2","stopaj_rate":"0","gross_up":false,"vat_rate":"20","enabled":true}]}`},
	"PUT /v1/tenant/exchange-rate-settings":                                                                                {body: `{"rate_kind":"forex_buying","auto_fill":true}`},
	"POST /v1/tenant/engagements/{engagementUuid}/document-tasks":                                                          {body: `{"title":"Kira sözleşmesi","note":"Ekim kira sözleşmesi","due_days":7}`},
	"POST /v1/tenant/engagements/{engagementUuid}/document-tasks/detect":                                                   {body: `{"period":"2026-10"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/document-tasks/generate":                                                 {body: `{"period":"2026-10"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/document-tasks/missing":                                                  {body: `{"period":"2026-10","items":[{"kind":"z_reports"}],"sms":true}`},
	"POST /v1/tenant/engagements/{engagementUuid}/document-tasks/{taskUuid}/cancel":                                        {body: `{"reason":"tarama"}`, late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/document-tasks/{taskUuid}/complete":                                      {late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/document-tasks/{taskUuid}/rerequest":                                     {body: `{"note":"Faturanın ikinci sayfası eksik"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/document-tasks/{taskUuid}/send":                                          {body: `{"channels":["push"],"reminder":true}`},
	"POST /v1/tenant/account-rules":                                                                                        {body: `{"role":"line","name":"Tarama kuralı","account_code":"770.01","conditions":{}}`},
	"POST /v1/tenant/account-rules/preview":                                                                                {body: `{"role":"line","account_code":"770.01","conditions":{"item_name_contains":["hizmet"]}}`},
	"PUT /v1/tenant/sms-settings":                                                                                          {body: `{"enabled":false,"test_mode":true,"usercode":"8501234567","msgheader":"OFISADI","encoding":"TR","appname":""}`},
	"POST /v1/tenant/sms-settings/test":                                                                                    {},
	"POST /v1/tenant/sms-messages/test":                                                                                    {body: `{"phone":"5321234567","text":"Deneme"}`},
	"POST /v1/tenant/communication/groups":                                                                                 {body: `{"name":"Tarama grubu","description":"","engagement_uuids":["{engagementUuid}"]}`},
	"PUT /v1/tenant/communication/groups/{groupUuid}":                                                                      {body: `{"name":"Tarama grubu 2","description":"","engagement_uuids":["{engagementUuid}"]}`, late: true},
	"DELETE /v1/tenant/communication/groups/{groupUuid}":                                                                   {last: true},
	"POST /v1/tenant/communication/bulk-sends/preview":                                                                     {body: `{"channel":"mail","subject":"Konu","body":"Sayın :mukellef","target":{"kind":"selection","engagement_uuids":["{engagementUuid}"]}}`},
	"POST /v1/tenant/communication/bulk-sends":                                                                             {body: `{"channel":"mail","subject":"Konu","body":"Sayın :mukellef","target":{"kind":"selection","engagement_uuids":["{engagementUuid}"]}}`},
	"POST /v1/tenant/automations":                                                                                          {body: `{"name":"Tarama kuralı","trigger_kind":"calendar","trigger":{"mode":"days","days":[15],"hour":9,"before_due_days":0,"tax":"","current_period":false,"event":"","delay_min_hours":0,"delay_max_hours":0,"resend_after_days":0,"payment_date":"","payment_amount":"","payment_days_before":0},"target_kind":"all","channels":["sms"],"body":"Sayın :mukellef, bilgi.","approval_mode":"auto","active":false}`},
	"PUT /v1/tenant/automations/{automationUuid}":                                                                          {body: `{"name":"Tarama kuralı","trigger_kind":"calendar","trigger":{"mode":"days","days":[15],"hour":9,"before_due_days":0,"tax":"","current_period":false,"event":"","delay_min_hours":0,"delay_max_hours":0,"resend_after_days":0,"payment_date":"","payment_amount":"","payment_days_before":0},"target_kind":"all","channels":["sms"],"body":"Sayın :mukellef, bilgi.","approval_mode":"auto","active":false}`},
	"DELETE /v1/tenant/automations/{automationUuid}":                                                                       {},
	"PUT /v1/tenant/automations/{automationUuid}/active":                                                                   {body: `{"active":false}`},
	"PUT /v1/tenant/automation-settings":                                                                                   {body: `{"enabled":true}`},
	"POST /v1/tenant/automation-runs/decide":                                                                               {body: `{"uuids":["00000000-0000-0000-0000-000000000000"],"approve":false}`},
	"PUT /v1/tenant/support-settings":                                                                                      {body: `{"clickfix_widget_id":"ofis-destek"}`},
	"PUT /v1/tenant/account-rule-settings":                                                                                 {body: `{"rule_candidate_min_count":4,"rule_candidate_decisions":"corrections_only"}`},
	"PUT /v1/tenant/tax-return-settings":                                                                                   {body: `{"final_check_required":true}`},
	"PUT /v1/tenant/z-report-parts-settings":                                                                               {body: `{"enabled":true}`},
	"PUT /v1/tenant/important-document-settings":                                                                           {body: `{"enabled":true}`},
	"PUT /v1/tenant/account-rules/{ruleKey}":                                                                               {body: `{"role":"line","name":"Tarama kuralı","account_code":"770.02","conditions":{}}`},
	"DELETE /v1/tenant/account-rules/{ruleKey}":                                                                            {},
	"POST /v1/tenant/engagements/{engagementUuid}/account-rules":                                                           {body: `{"role":"line","name":"Tarama kuralı","account_code":"770.01","conditions":{}}`},
	"POST /v1/tenant/engagements/{engagementUuid}/account-rules/preview":                                                   {body: `{"role":"line","account_code":"770.01","conditions":{"direction":"outgoing"}}`},
	"POST /v1/tenant/engagements/{engagementUuid}/account-rules/apply-template":                                            {body: `{}`},
	"PUT /v1/tenant/engagements/{engagementUuid}/account-rules/{ruleKey}":                                                  {body: `{"role":"line","name":"Tarama kuralı","account_code":"770.02","conditions":{}}`},
	"DELETE /v1/tenant/engagements/{engagementUuid}/account-rules/{ruleKey}":                                               {},
	"POST /v1/tenant/engagements/{engagementUuid}/account-rule-candidates/promote":                                         {body: `{}`},
	"POST /v1/tenant/engagements/{engagementUuid}/invoices/{invoiceUuid}/account-suggestions":                              {body: `{}`},
	"PUT /v1/tenant/engagements/{engagementUuid}/invoices/{invoiceUuid}/exchange-rate":                                     {body: `{"rate":"38.5"}`},
	"PUT /v1/tenant/engagements/{engagementUuid}/invoices/{invoiceUuid}/counterparty-tax-id":                               {body: `{"tax_id":"9000000017"}`},
	"PUT /v1/tenant/engagements/{engagementUuid}/invoices/{invoiceUuid}/vat-deduction-period":                              {body: `{"period":"2027-01"}`},
	"DELETE /v1/tenant/engagements/{engagementUuid}/invoices/{invoiceUuid}/vat-deduction-period":                           {},
	"POST /v1/tenant/engagements/{engagementUuid}/invoices/{invoiceUuid}/journal-draft":                                    {},
	"POST /v1/tenant/engagements/{engagementUuid}/invoice-postings":                                                        {body: `{"period":"2026-10"}`},
	"POST /v1/tenant/engagements/{engagementUuid}/vat-forecast/evaluate":                                                   {body: `{}`},
	"PUT /v1/tenant/engagements/{engagementUuid}/vat-forecast/openings/{period}":                                           {body: `{"amount":"100.00"}`},
	"DELETE /v1/tenant/engagements/{engagementUuid}/vat-forecast/openings/{period}":                                        {},
	"PUT /v1/tenant/engagements/{engagementUuid}/tax-returns/{returnKind}/{period}":                                        {body: `{"adjustments":{"additional_vat":"0.00"},"acknowledged":["CARRY_UNKNOWN"]}`},
	"POST /v1/tenant/engagements/{engagementUuid}/tax-returns/{returnKind}/{period}/prepare":                               {body: `{"acknowledged":["PENDING_EXPENSES","PENDING_Z_REPORTS","Z_WITHOUT_BREAKDOWN","HEADER_ESTIMATE","MISSING_EXCHANGE_RATE"]}`, late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/tax-returns/{returnKind}/{period}/check":                                 {late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/tax-returns/{returnKind}/{period}/file":                                  {body: `{"filed_on":"2026-11-10","accrual_number":"TARAMA-1"}`, late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/tax-returns/{returnKind}/{period}/reopen":                                {late: true},
	"PUT /v1/tenant/engagements/{engagementUuid}/tax-returns/{returnKind}/{period}/accrual-document":                       {file: "file", fileName: "tahakkuk.pdf", fileBody: "%PDF-1.4\n% tahakkuk taramasi\n", late: true},
	"PUT /v1/tenant/engagements/{engagementUuid}/tax-returns/{returnKind}/{period}/payment":                                {body: `{"paid_on":"2026-11-20"}`, late: true},
	"PUT /v1/tenant/engagements/{engagementUuid}/tax-returns/{returnKind}/{period}/rent-withholdings/{transactionUuid}":    {body: `{"rate":"20"}`},
	"DELETE /v1/tenant/engagements/{engagementUuid}/tax-returns/{returnKind}/{period}/rent-withholdings/{transactionUuid}": {late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/tax-returns/{returnKind}/{period}/amend":                                 {late: true},
	"PUT /v1/tenant/vat-tracking-settings":                                                                                 {body: `{"report_enabled":true,"report_day":29,"watch_email":false,"payable_limit":"10000.00"}`},
	// Defter türü ve işletme hesabı (#780): engagement işletme defterine
	// geçer; kategori ve mal mevcudu ondan sonra yazılır.
	"PUT /v1/tenant/engagements/{engagementUuid}/ledger-type":                  {body: `{"ledger_type":"isletme"}`, late: true},
	"PUT /v1/tenant/engagements/{engagementUuid}/operating-account/categories": {body: `{"source":"invoice","record_uuid":"00000000-0000-4000-8000-000000000780","category":"general_expense"}`, late: true},
	"PUT /v1/tenant/engagements/{engagementUuid}/operating-account/inventory":  {body: `{"year":2026,"opening":"1000.00","closing":"1500.00"}`, late: true},
	"POST /v1/tenant/engagements/{engagementUuid}/vat-forecast/notices":        {body: `{}`},
	"POST /v1/tenant/engagements/{engagementUuid}/vat-forecast/watch":          {body: `{}`},
	"PUT /v1/tenant/vat-report-rules":                                          {body: `{"items":[{"trigger_kind":"day","day":29,"audience":"office","scope":"all","engagement_uuids":[],"enabled":true}]}`},
	"POST /v1/tenant/vat-report-rules/{rule}/run":                              {body: `{}`},

	// Aktarılmış fişin tamamen iptali (#710).
	"POST /v1/tenant/engagements/{engagementUuid}/journal-drafts/{draftUuid}/cancel-export": {body: `{"reason":"tarama"}`, late: true},

	// Fişe elle bağlanan kayıt (#710).
	"PUT /v1/tenant/engagements/{engagementUuid}/journal-drafts/{draftUuid}/links/invoices/{invoiceUuid}":                 {},
	"DELETE /v1/tenant/engagements/{engagementUuid}/journal-drafts/{draftUuid}/links/invoices/{invoiceUuid}":              {late: true},
	"PUT /v1/tenant/engagements/{engagementUuid}/journal-drafts/{draftUuid}/links/bank-transactions/{transactionUuid}":    {},
	"DELETE /v1/tenant/engagements/{engagementUuid}/journal-drafts/{draftUuid}/links/bank-transactions/{transactionUuid}": {late: true},

	// Fişe elle bağlanan cari kartı (#710).
	"PUT /v1/tenant/engagements/{engagementUuid}/journal-drafts/{draftUuid}/links/customers/{customerUuid}":    {},
	"DELETE /v1/tenant/engagements/{engagementUuid}/journal-drafts/{draftUuid}/links/customers/{customerUuid}": {late: true},

	// Mükellef defterleri (#780): Luca işletme defteri dökümü.
	"POST /v1/tenant/engagements/{engagementUuid}/ledgers": {file: "file", fileName: "isletme-defteri.csv",
		fileBody: "Tarih;Açıklama;Satın Alınan Mallar;Giderler;Satılan Mallar\n05.10.2026;Mal alışı;1.000,00;;\n12.10.2026;Satış;;;2.500,00\n",
		fields:   map[string]string{"period": "2026-10", "kind": "isletme_defteri"}},
	"PUT /v1/tenant/engagements/{engagementUuid}/ledgers/settings":        {body: `{"period_kind":"quarterly"}`},
	"DELETE /v1/tenant/engagements/{engagementUuid}/ledgers/{ledgerUuid}": {last: true},
}

type sweepEnv struct {
	d         *testdb.DB
	h         http.Handler
	orgID     int64
	engID     int64
	tpID      int64
	ownerTok  string
	tpTok     string
	tpUUID    string
	backend   string
	createdBy map[string]bool
	fakes     sweepFakes
}

// sweepFakes are the external services of the sweep (internal/testfakes).
type sweepFakes struct {
	ai        *testfakes.AI
	kolaybi   *testfakes.KolayBi
	parasut   *testfakes.Parasut
	easytrade *testfakes.Easytrade
	bank      *testfakes.Vakifbank
	edm       *testfakes.EDM
}

func TestWriteEndpointsNo5xx(t *testing.T) {
	ops := specWriteOps(t)
	check := func(prefix string, cases map[string]sweepCase) {
		listed := map[string]bool{}
		for _, op := range ops {
			if !strings.HasPrefix(op.path, prefix) {
				continue
			}
			key := op.method + " " + op.path
			listed[key] = true
			if _, ok := cases[key]; !ok {
				t.Errorf("%s is a write operation without a sweep case: add it to the table in write_sweep_db_test.go", key)
			}
		}
		for key := range cases {
			if !listed[key] {
				t.Errorf("sweep case %s is not a write operation in openapi.yaml any more: remove it", key)
			}
		}
	}
	check("/v1/taxpayer/", taxpayerWrites)
	check("/v1/tenant/", tenantWrites)
	if t.Failed() {
		return
	}

	e := newSweepEnv(t)
	e.sharedReads(t)
	var late, last []specOp
	run := func(op specOp) {
		key := op.method + " " + op.path
		if strings.HasPrefix(op.path, "/v1/taxpayer/") {
			e.call(t, op, taxpayerWrites[key], true)
		} else {
			e.call(t, op, tenantWrites[key], false)
		}
	}
	for _, op := range ops {
		key := op.method + " " + op.path
		c, tp := taxpayerWrites[key]
		if !tp {
			var ok bool
			if c, ok = tenantWrites[key]; !ok {
				continue
			}
		}
		switch {
		case c.last:
			last = append(last, op)
			continue
		case c.late:
			late = append(late, op)
			continue
		}
		run(op)
	}
	for _, op := range append(late, last...) {
		run(op)
	}
	e.deepPaths(t)
}

// deepPaths (#318): the external services were really called and the
// receipt path went past OCR: drafts, low-confidence fields in the review
// queue, the source and bank syncs wrote rows.
func (e *sweepEnv) deepPaths(t *testing.T) {
	t.Helper()
	if !slices.Contains(e.fakes.ai.Purposes, "receipt") {
		t.Errorf("the AI fake read no receipt: %v", e.fakes.ai.Purposes)
	}
	if e.fakes.edm.Logins == 0 {
		t.Errorf("the EDM fake was not called: the e-invoice connection test did not log in")
	}
	if e.fakes.kolaybi.Tokens == 0 || e.fakes.easytrade.Logins == 0 || e.fakes.parasut.Refreshes == 0 || e.fakes.bank.Requests == 0 {
		t.Errorf("an external fake was not called: kolaybi tokens %d, easytrade logins %d, paraşüt tokens %d, bank calls %d",
			e.fakes.kolaybi.Tokens, e.fakes.easytrade.Logins, e.fakes.parasut.Refreshes, e.fakes.bank.Requests)
	}
	counts := map[string]string{
		"receipts read into a draft (not failed)": `SELECT count(*) FROM receipt_captures WHERE taxpayer_id = $3 AND status <> 'failed' AND kind IS NOT NULL AND $1 > 0 AND $2 > 0`,
		"receipt fields in the review queue":      `SELECT count(*) FROM suggestions WHERE organization_id = $1 AND engagement_id = $2 AND kind = 'receipt_field' AND $3 > 0`,
		"invoices from the source syncs":          `SELECT count(*) FROM invoices WHERE taxpayer_id = $3 AND source_system IN ('kolaybi', 'parasut', 'easytrade') AND $1 > 0 AND $2 > 0`,
		"bank movements from the web service":     `SELECT count(*) FROM bank_transactions WHERE taxpayer_id = $3 AND $1 > 0 AND $2 > 0`,
	}
	for what, q := range counts {
		if n := e.one(t, q, what); n == "0" {
			t.Errorf("deep path: no %s", what)
		}
	}
}

type specOp struct{ method, path string }

// specWriteOps lists POST/PUT/PATCH/DELETE operations in spec order.
func specWriteOps(t *testing.T) []specOp {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(backendRoot(), "docs", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	pathRe := regexp.MustCompile(`^  (/[^:\s]+):\s*$`)
	opRe := regexp.MustCompile(`^    (post|put|patch|delete):\s*$`)
	var out []specOp
	var current string
	for _, line := range strings.Split(string(raw), "\n") {
		if m := pathRe.FindStringSubmatch(line); m != nil {
			current = strings.Trim(m[1], `"'`)
			continue
		}
		if m := opRe.FindStringSubmatch(line); m != nil && current != "" {
			out = append(out, specOp{strings.ToUpper(m[1]), current})
		}
	}
	return out
}

func backendRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func newSweepEnv(t *testing.T) *sweepEnv {
	t.Helper()
	// SWEEP_DATABASE_URL (issue #317): sweep an existing database that
	// already holds the demo office — the production-like database the
	// owner change rehearsal (.github/workflows/db-owner-rehearsal.yml)
	// migrated to hakan_owner — instead of a fresh one.
	existing := os.Getenv("SWEEP_DATABASE_URL")
	var d *testdb.DB
	if existing != "" {
		d = testdb.OpenExisting(t, existing)
	} else {
		d = testdb.Open(t)
	}
	u, err := url.Parse(d.URL)
	if err != nil {
		t.Fatal(err)
	}
	pass, _ := u.User.Password()
	mr := miniredis.RunT(t)
	fakes := sweepFakes{
		ai: &testfakes.AI{}, kolaybi: testfakes.KolayBiSample(), parasut: testfakes.ParasutSample(),
		easytrade: testfakes.EasytradeSample(), bank: &testfakes.Vakifbank{}, edm: testfakes.EDMSample(),
	}
	aiSrv, kbSrv, prsSrv := testfakes.Serve(t, fakes.ai), testfakes.Serve(t, fakes.kolaybi), testfakes.Serve(t, fakes.parasut)
	etSrv, bankSrv := testfakes.Serve(t, fakes.easytrade), testfakes.Serve(t, fakes.bank)
	edmSrv := testfakes.Serve(t, fakes.edm)
	env := map[string]string{
		"APP_ENV": "development", "APP_NAME": "api", "APP_HTTP_ADDR": ":0",
		"DB_HOST": u.Hostname(), "DB_PORT": u.Port(), "DB_USER": u.User.Username(), "DB_PASSWORD": pass,
		"DB_NAME": strings.TrimPrefix(u.Path, "/"), "DB_SSLMODE": "disable",
		"REDIS_ADDR": mr.Addr(), "JWT_ACCESS_SECRET": strings.Repeat("s", 40),
		"QUEUE_ENABLED": "false", "SEARCH_ENABLED": "false", "CENTRIFUGO_ENABLED": "false",
		"SMTP_HOST": "127.0.0.1", "SMTP_PORT": "9", "SMTP_TIMEOUT": "1s",
		"AI_PROVIDER": "openai_compatible", "AI_BASE_URL": aiSrv.URL, "AI_API_KEY": "sweep", "AI_MODEL": "fake-ocr",
		"AI_IMAGE_REDACTION": "off", "RATE_LIMIT_STORE": "memory",
		"KOLAYBI_BASE_URL": kbSrv.URL, "KOLAYBI_CHANNEL": testfakes.KolayBiChannel,
		"EASYTRADE_API_BASE_URL": etSrv.URL,
		"PARASUT_BASE_URL":       prsSrv.URL, "PARASUT_CLIENT_ID": testfakes.ParasutClientID,
		"PARASUT_CLIENT_SECRET": testfakes.ParasutClientSecret, "PARASUT_REDIRECT_URL": "https://panel.example.test/parasut/callback",
		"STORAGE_DRIVER": "s3", "TENANCY_ENABLED": "true",
	}
	if p := u.Query().Get("sslmode"); p != "" {
		env["DB_SSLMODE"] = p
	}
	for k, v := range env {
		t.Setenv(k, v)
	}

	// The demo office (cmd/demo-seed): office, members, taxpayers, engagements.
	if existing == "" {
		bin := filepath.Join(t.TempDir(), "demo-seed")
		build := exec.Command("go", "build", "-o", bin, "./cmd/demo-seed")
		build.Dir = backendRoot()
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build demo-seed: %v\n%s", err, out)
		}
		seed := exec.Command(bin)
		seed.Dir = backendRoot()
		seed.Env = os.Environ()
		if out, err := seed.CombinedOutput(); err != nil {
			t.Fatalf("demo-seed: %v\n%s", err, out)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.DB.RLSRequestScope = true
	logOut := io.Discard
	if os.Getenv("SWEEP_VERBOSE") != "" {
		logOut = os.Stderr
	}
	log := slog.New(slog.NewTextHandler(logOut, &slog.HandlerOptions{Level: slog.LevelWarn}))
	srv, err := New(cfg, log, Deps{
		DB: d.Pool, Queries: database.NewQueries(d.Pool),
		Redis:               redis.NewClient(&redis.Options{Addr: mr.Addr()}),
		Storage:             memDriver{storage.NewMemory()},
		BankEndpoints:       map[string]string{"vakifbank": bankSrv.URL},
		EntegratorEndpoints: map[string]string{"edm": edmSrv.URL},
	})
	if err != nil {
		t.Fatal(err)
	}
	e := &sweepEnv{d: d, h: srv.http.Handler, backend: backendRoot(), createdBy: map[string]bool{}, fakes: fakes}
	ctx := context.Background()
	if err := rls.System(ctx, d.Pool, "test: sweep fixture", func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT id FROM organizations WHERE slug = 'demo-smmm'`).Scan(&e.orgID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			SELECT e.id, t.id, t.uuid::text FROM engagements e JOIN taxpayers t ON t.id = e.taxpayer_id
			WHERE e.organization_id = $1 AND t.tax_id = '9000000017' AND e.status = 'active'`, e.orgID).Scan(&e.engID, &e.tpID, &e.tpUUID)
	}); err != nil {
		t.Fatal(err)
	}
	e.ownerTok = e.login(t, "sahip@demo.hakan.local")
	e.tpTok = e.login(t, "mukellef@demo.hakan.local")

	// The office's invoices (demo UBL set), as the panel uploads them.
	files, _ := filepath.Glob(filepath.Join(backendRoot(), "internal", "demo", "ubl", "atlas-0[1-2]*.xml"))
	for _, f := range files {
		rec := e.do(t, http.MethodPost, "/v1/tenant/engagements/"+e.param(t, "engagementUuid", "")+"/ubl-uploads",
			sweepCase{file: "file", fileName: filepath.Base(f), fileBody: "@ubl:" + filepath.Base(f)}, false)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("seed upload %s: %d %s", f, rec.Code, rec.Body)
		}
	}
	return e
}

func (e *sweepEnv) login(t *testing.T, email string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{"email":"`+email+`","password":"Demo1234!"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	var out struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &out) != nil || out.Data.AccessToken == "" {
		t.Fatalf("login %s: %d %s", email, rec.Code, rec.Body)
	}
	return out.Data.AccessToken
}

var paramRe = regexp.MustCompile(`\{([A-Za-z]+)\}`)

func (e *sweepEnv) call(t *testing.T, op specOp, c sweepCase, taxpayer bool) {
	t.Helper()
	path := paramRe.ReplaceAllStringFunc(op.path, func(m string) string {
		return e.param(t, m[1:len(m)-1], op.path)
	})
	// A body may name the engagement (office-wide bulk routes, #386).
	if strings.Contains(c.body, "{engagementUuid}") {
		c.body = strings.ReplaceAll(c.body, "{engagementUuid}", e.param(t, "engagementUuid", op.path))
	}
	rec := e.do(t, op.method, path, c, taxpayer)
	if os.Getenv("SWEEP_VERBOSE") != "" {
		t.Logf("%d %s %s %s", rec.Code, op.method, op.path, strings.TrimSpace(firstLine(rec.Body.String())))
	}
	if rec.Code < 500 {
		return
	}
	t.Errorf("%s %s (%s) = %d: %s", op.method, op.path, path, rec.Code, strings.TrimSpace(rec.Body.String()))
}

func (e *sweepEnv) do(t *testing.T, method, path string, c sweepCase, taxpayer bool) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader
	contentType := "application/json"
	switch {
	case c.file != "":
		data := c.fileBody
		if name, ok := strings.CutPrefix(data, "@ubl:"); ok {
			raw, err := os.ReadFile(filepath.Join(e.backend, "internal", "demo", "ubl", name))
			if err != nil {
				t.Fatal(err)
			}
			data = string(raw)
		}
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		for k, v := range c.fields {
			_ = mw.WriteField(k, v)
		}
		fw, _ := mw.CreateFormFile(c.file, c.fileName)
		_, _ = fw.Write([]byte(data))
		_ = mw.Close()
		body, contentType = &buf, mw.FormDataContentType()
	case c.body != "":
		body = strings.NewReader(c.body)
	default:
		body = strings.NewReader(`{}`)
	}
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", contentType)
	if taxpayer {
		req.Header.Set("Authorization", "Bearer "+e.tpTok)
		req.Header.Set("X-Taxpayer", e.tpUUID)
	} else {
		req.Header.Set("Authorization", "Bearer "+e.ownerTok)
		req.Header.Set("X-Organization", "demo-smmm")
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

// paramValue queries: the most recent demo row for a path parameter. $1 is
// the demo office, $2 the Atlas engagement, $3 Atlas (taxpayer).
var paramQueries = map[string]string{
	// A real rule of office A (one is added when the office has none), so the
	// IDOR check sees office B ask for a record that is not its own: the
	// default rule ("default") exists in every office.
	"rule": `WITH ins AS (
		INSERT INTO vat_report_rules (organization_id, trigger_kind, day, label)
		SELECT $1, 'day', 29, 'Tarama kuralı' WHERE $2 > 0 AND $3 > 0
			AND NOT EXISTS (SELECT 1 FROM vat_report_rules WHERE organization_id = $1)
		RETURNING uuid)
	SELECT uuid::text FROM ins
	UNION ALL (SELECT uuid::text FROM vat_report_rules WHERE organization_id = $1 ORDER BY id LIMIT 1)
	LIMIT 1`,
	"userUuid":            `SELECT u.uuid::text FROM users u WHERE u.email = 'stajyer@demo.hakan.local' AND $1 > 0 AND $2 > 0 AND $3 > 0`,
	"roleKey":             `SELECT 'sweep_role' WHERE $1 > 0 AND $2 > 0 AND $3 > 0`,
	"code":                `SELECT 'BNK_UNDOCUMENTED_PAYMENT' WHERE $1 > 0 AND $2 > 0 AND $3 > 0`,
	"warningUuid":         `SELECT uuid::text FROM data_warnings WHERE organization_id = $1 AND engagement_id = $2 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"suggestionUuid":      `SELECT uuid::text FROM suggestions WHERE organization_id = $1 AND engagement_id = $2 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"importUuid":          `SELECT uuid::text FROM excel_imports WHERE organization_id = $1 AND engagement_id = $2 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"receiptUuid":         `SELECT uuid::text FROM receipt_captures WHERE organization_id = $1 AND engagement_id = $2 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"expenseUuid":         `SELECT uuid::text FROM expenses WHERE taxpayer_id = $3 AND $1 > 0 AND $2 > 0 ORDER BY id DESC LIMIT 1`,
	"categoryUuid":        `SELECT uuid::text FROM expense_categories WHERE organization_id = $1 AND $2 > 0 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"retailSaleUuid":      `SELECT uuid::text FROM retail_sales WHERE taxpayer_id = $3 AND $1 > 0 AND $2 > 0 ORDER BY id DESC LIMIT 1`,
	"vehicleUuid":         `SELECT uuid::text FROM taxpayer_vehicles WHERE taxpayer_id = $3 AND $1 > 0 AND $2 > 0 ORDER BY id DESC LIMIT 1`,
	"summaryUuid":         `SELECT uuid::text FROM period_summaries WHERE organization_id = $1 AND engagement_id = $2 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"automationUuid":      `SELECT uuid::text FROM automation_rules WHERE organization_id = $1 AND deleted_at IS NULL AND $2 > 0 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"agentUuid":           `SELECT uuid::text FROM desktop_agents WHERE organization_id = $1 AND $2 > 0 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"agentDocumentUuid":   `SELECT uuid::text FROM agent_documents WHERE organization_id = $1 AND $2 > 0 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"draftUuid":           `SELECT uuid::text FROM journal_drafts WHERE organization_id = $1 AND engagement_id = $2 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"batchUuid":           `SELECT uuid::text FROM export_batches WHERE organization_id = $1 AND engagement_id = $2 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"transferUuid":        `SELECT uuid::text FROM accounting_transfers WHERE organization_id = $1 AND engagement_id = $2 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"requestUuid":         `SELECT uuid::text FROM privacy_requests WHERE organization_id = $1 AND engagement_id = $2 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"aliasUuid":           `SELECT uuid::text FROM bank_counterparty_aliases WHERE organization_id = $1 AND engagement_id = $2 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"planUuid":            `SELECT p.uuid::text FROM copilot_plans p JOIN users u ON u.id = p.created_by WHERE p.organization_id = $1 AND u.email = 'sahip@demo.hakan.local' AND $2 > 0 AND $3 > 0 ORDER BY p.id DESC LIMIT 1`,
	"stepIndex":           `SELECT '0' WHERE $1 > 0 AND $2 > 0 AND $3 > 0`,
	"transactionUuid":     `SELECT uuid::text FROM bank_transactions WHERE taxpayer_id = $3 AND $1 > 0 AND $2 > 0 ORDER BY id DESC LIMIT 1`,
	"resolutionUuid":      `SELECT COALESCE((SELECT uuid::text FROM bank_difference_resolutions WHERE organization_id = $1 AND engagement_id = $2 AND $3 > 0 ORDER BY id DESC LIMIT 1), '00000000-0000-0000-0000-000000000000')`,
	"cashBoxUuid":         `SELECT uuid::text FROM cash_boxes WHERE taxpayer_id = $3 AND $1 > 0 AND $2 > 0 ORDER BY id DESC LIMIT 1`,
	"changeRequestUuid":   `SELECT uuid::text FROM change_requests WHERE organization_id = $1 AND engagement_id = $2 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"lateDocumentUuid":    `SELECT uuid::text FROM late_documents WHERE organization_id = $1 AND engagement_id = $2 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"foreignInvoiceUuid":  `SELECT uuid::text FROM foreign_service_invoices WHERE organization_id = $1 AND engagement_id = $2 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"ledgerUuid":          `SELECT uuid::text FROM ledger_files WHERE organization_id = $1 AND engagement_id = $2 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"taskUuid":            `SELECT uuid::text FROM document_tasks WHERE organization_id = $1 AND engagement_id = $2 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"employeeUuid":        `SELECT uuid::text FROM employees WHERE taxpayer_id = $3 AND $1 > 0 AND $2 > 0 ORDER BY id DESC LIMIT 1`,
	"runUuid":             `SELECT uuid::text FROM payroll_runs WHERE taxpayer_id = $3 AND $1 > 0 AND $2 > 0 ORDER BY id DESC LIMIT 1`,
	"premiumUuid":         `SELECT uuid::text FROM sgk_premiums WHERE taxpayer_id = $3 AND $1 > 0 AND $2 > 0 ORDER BY id DESC LIMIT 1`,
	"documentUuid":        `SELECT uuid::text FROM payroll_documents WHERE taxpayer_id = $3 AND $1 > 0 AND $2 > 0 ORDER BY id DESC LIMIT 1`,
	"invoiceUuid":         `SELECT uuid::text FROM invoices WHERE taxpayer_id = $3 AND $1 > 0 AND $2 > 0 ORDER BY id DESC LIMIT 1`,
	"connectionUuid":      `SELECT uuid::text FROM source_connections WHERE taxpayer_id = $3 AND $1 > 0 AND $2 > 0 ORDER BY id DESC LIMIT 1`,
	"templateUuid":        `SELECT uuid::text FROM excel_import_templates WHERE organization_id = $1 AND $2 > 0 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"ruleKey":             `SELECT rule_key FROM account_rules WHERE organization_id = $1 AND $2 > 0 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"linkUuid":            `SELECT uuid::text FROM whatsapp_chat_links WHERE organization_id = $1 AND engagement_id = $2 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"captureUuid":         `SELECT uuid::text FROM whatsapp_captures WHERE organization_id = $1 AND engagement_id = $2 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"archiveDocumentUuid": `SELECT uuid::text FROM taxpayer_documents WHERE taxpayer_id = $3 AND $1 > 0 AND $2 > 0 ORDER BY id DESC LIMIT 1`,
	"itemUuid":            `SELECT uuid::text FROM reconciliation_items WHERE organization_id = $1 AND engagement_id = $2 AND $3 > 0 AND result <> 'matched' ORDER BY id DESC LIMIT 1`,
	"counterpartyUuid":    `SELECT uuid::text FROM counterparty_reconciliations WHERE organization_id = $1 AND engagement_id = $2 AND $3 > 0 AND result <> 'matched' ORDER BY id DESC LIMIT 1`,
	"declarationUuid":     `SELECT uuid::text FROM babs_declarations WHERE organization_id = $1 AND engagement_id = $2 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"letterUuid":          `SELECT uuid::text FROM reconciliation_letters WHERE organization_id = $1 AND engagement_id = $2 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"groupUuid":           `SELECT uuid::text FROM taxpayer_groups WHERE organization_id = $1 AND $2 > 0 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"bulkUuid":            `SELECT uuid::text FROM bulk_sends WHERE organization_id = $1 AND $2 > 0 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"customerUuid":        `SELECT uuid::text FROM customers WHERE taxpayer_id = $3 AND type <> 'employee' AND $1 > 0 AND $2 > 0 ORDER BY id DESC LIMIT 1`,
}

// uuidQueries fill {uuid} by the operation's path.
var uuidQueries = map[string]string{
	"/v1/tenant/taxpayer-requests/":     `SELECT uuid::text FROM taxpayer_onboarding_requests WHERE organization_id = $1 AND $2 > 0 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"/v1/tenant/engagements/":           `SELECT uuid::text FROM taxpayer_invitations WHERE organization_id = $1 AND engagement_id = $2 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"/v1/tenant/invitations/":           `SELECT uuid::text FROM organization_invitations WHERE organization_id = $1 AND $2 > 0 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"/v1/tenant/taxpayer-imports/":      `SELECT uuid::text FROM taxpayer_import_batches WHERE organization_id = $1 AND $2 > 0 AND $3 > 0 ORDER BY id DESC LIMIT 1`,
	"/v1/taxpayer/onboarding-requests/": `SELECT uuid::text FROM taxpayer_onboarding_requests t WHERE $1 > 0 AND $2 > 0 AND t.tax_id = (SELECT tax_id FROM taxpayers WHERE id = $3) ORDER BY id DESC LIMIT 1`,
	"/v1/taxpayer/history-shares/":      `SELECT uuid::text FROM taxpayer_history_shares WHERE taxpayer_id = $3 AND $1 > 0 AND $2 > 0 ORDER BY id DESC LIMIT 1`,
	"/v1/tenant/notifications/":         `SELECT n.uuid::text FROM notifications n JOIN users u ON u.id = n.user_id WHERE n.organization_id = $1 AND u.email = 'sahip@demo.hakan.local' AND n.channel = 'inapp' AND $2 > 0 AND $3 > 0 ORDER BY n.id DESC LIMIT 1`,
	"/v1/taxpayer/notifications/":       `SELECT n.uuid::text FROM notifications n JOIN users u ON u.id = n.user_id WHERE u.email = 'mukellef@demo.hakan.local' AND n.channel = 'inapp' AND n.payload ->> 'taxpayer_uuid' = (SELECT uuid::text FROM taxpayers WHERE id = $3) AND $1 > 0 AND $2 > 0 ORDER BY n.id DESC LIMIT 1`,
	"/v1/taxpayer/receipts/":            `SELECT uuid::text FROM receipt_captures WHERE taxpayer_id = $3 AND $1 > 0 AND $2 > 0 ORDER BY id DESC LIMIT 1`,
}

// param resolves a path parameter from the demo data; a parameter without
// a row (nothing created it yet) gets a random id and the call answers 404.
func (e *sweepEnv) param(t *testing.T, name, path string) string {
	t.Helper()
	switch name {
	case "engagementUuid":
		return e.one(t, `SELECT uuid::text FROM engagements WHERE id = $2 AND $1 > 0 AND $3 > 0`, name)
	case "period":
		return "2026-10"
	case "rowNo", "lineNo":
		return "1"
	case "returnKind":
		return "kdv1"
	case "kind":
		return "receipt_field"
	case "importId":
		return uuid.NewString()
	case "connectionUuid":
		if strings.Contains(path, "/einvoice-connections/") {
			return e.one(t, `SELECT uuid::text FROM source_connections WHERE taxpayer_id = $3 AND source_system LIKE 'entegrator\_%' AND $1 > 0 AND $2 > 0 ORDER BY id DESC LIMIT 1`, name)
		}
	case "uuid":
		for prefix, q := range uuidQueries {
			if strings.HasPrefix(path, prefix) {
				return e.one(t, q, name)
			}
		}
	}
	if q, ok := paramQueries[name]; ok {
		return e.one(t, q, name)
	}
	t.Fatalf("no value for path parameter {%s} of %s: add it to paramQueries", name, path)
	return ""
}

func (e *sweepEnv) one(t *testing.T, q, name string) string {
	t.Helper()
	var v string
	err := rls.System(context.Background(), e.d.Pool, "test: sweep parameter", func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, q, e.orgID, e.engID, e.tpID).Scan(&v)
	})
	switch {
	case err == nil:
		return v
	case errors.Is(err, pgx.ErrNoRows):
		return uuid.NewString()
	default:
		t.Fatalf("parameter {%s}: %v", name, fmt.Errorf("%w (query %s)", err, q))
		return ""
	}
}

// memDriver is storage.Memory as a full storage.Driver for the sweep; the
// operations Memory lacks report the store as unavailable.
type memDriver struct{ *storage.Memory }

var errNoVersions = fmt.Errorf("sweep storage: not supported")

func (memDriver) DownloadVersion(context.Context, string, string) (io.ReadCloser, int64, error) {
	return nil, 0, errNoVersions
}
func (memDriver) DeleteVersion(context.Context, string, string) error { return errNoVersions }
func (memDriver) List(context.Context, storage.ListObjectsInput) (storage.ListObjectsResult, error) {
	return storage.ListObjectsResult{}, nil
}
func (memDriver) Head(context.Context, string) (storage.ObjectInfo, error) {
	return storage.ObjectInfo{}, errNoVersions
}
func (memDriver) Copy(context.Context, string, string) error                { return errNoVersions }
func (memDriver) CopyVersion(context.Context, string, string, string) error { return errNoVersions }
func (memDriver) ListVersions(context.Context, string) ([]storage.VersionInfo, error) {
	return nil, nil
}
func (memDriver) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "", errNoVersions
}
func (memDriver) PresignPut(context.Context, string, string, time.Duration) (string, error) {
	return "", errNoVersions
}
func (memDriver) Bucket() string { return "sweep" }

func firstLine(s string) string {
	if len(s) > 160 {
		s = s[:160]
	}
	return s
}

// sharedReads (issue #307): inside the office's request scope the
// platform's rows of the rls_apply_org_shared tables stay visible: the
// system roles in the role list (every permission screen reads them).
func (e *sweepEnv) sharedReads(t *testing.T) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/tenant/roles", nil)
	req.Header.Set("Authorization", "Bearer "+e.ownerTok)
	req.Header.Set("X-Organization", "demo-smmm")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `"key":"owner"`) || !strings.Contains(body, `"key":"member"`) {
		t.Fatalf("GET /v1/tenant/roles in the office scope = %d %s, want the system roles", rec.Code, firstLine(body))
	}
}
