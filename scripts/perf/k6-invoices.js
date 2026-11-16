// Temel yük testi (issue #182): ofis fatura listesi (ilk sayfa, imleçle
// derin sayfa), mükellef × dönem matrisi.
//
// Veri: `make perf-seed` (500 mükellef, 10.000 fatura; faturaların %30'u ilk
// mükellefte). Çalıştırma:
//
//   k6 run scripts/perf/k6-invoices.js
//   BASE_URL=https://staging.example.com k6 run scripts/perf/k6-invoices.js
//
// Ortam değişkenleri: BASE_URL (http://127.0.0.1:8080), PERF_EMAIL,
// PERF_PASSWORD, PERF_ORG (perf-smmm), PERF_ENGAGEMENT (boşsa matristen en
// çok faturası olan ilk mükellef seçilir), VUS, DURATION.
import http from 'k6/http';
import { check, fail } from 'k6';
import { Trend } from 'k6/metrics';

const BASE = __ENV.BASE_URL || 'http://127.0.0.1:8080';
const ORG = __ENV.PERF_ORG || 'perf-smmm';

export const options = {
  scenarios: {
    office: {
      executor: 'constant-vus',
      vus: Number(__ENV.VUS || 10),
      duration: __ENV.DURATION || '1m',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    'list_first_page': ['p(95)<300'],
    'list_cursor_page': ['p(95)<300'],
    'period_matrix': ['p(95)<300'],
  },
};

const firstPage = new Trend('list_first_page', true);
const cursorPage = new Trend('list_cursor_page', true);
const matrix = new Trend('period_matrix', true);

export function setup() {
  const login = http.post(`${BASE}/v1/auth/login`, JSON.stringify({
    email: __ENV.PERF_EMAIL || 'perf-owner@perf.test',
    password: __ENV.PERF_PASSWORD || 'Perf1234!',
  }), { headers: { 'Content-Type': 'application/json' } });
  if (login.status !== 200) fail(`login: ${login.status}`);
  const token = login.json('data.access_token');
  const headers = { Authorization: `Bearer ${token}`, 'X-Organization': ORG };
  let engagement = __ENV.PERF_ENGAGEMENT;
  if (!engagement) {
    const m = http.get(`${BASE}/v1/tenant/periods/matrix?months=1`, { headers });
    const rows = m.json('data.items') || [];
    rows.sort((a, b) => a.taxpayer_tax_id.localeCompare(b.taxpayer_tax_id));
    if (!rows.length) fail('no engagements; run make perf-seed');
    engagement = rows[0].engagement_uuid;
  }
  return { headers, engagement };
}

export default function (data) {
  const params = { headers: data.headers };
  const list = `${BASE}/v1/tenant/engagements/${data.engagement}/invoices?limit=50`;

  const p1 = http.get(list, Object.assign({ tags: { name: 'list_first_page' } }, params));
  check(p1, { 'first page 200': (r) => r.status === 200 });
  firstPage.add(p1.timings.duration);

  // Walk up to 10 pages with the cursor, as a user scrolling would.
  let cursor = p1.status === 200 ? p1.json('data.next_cursor') : null;
  for (let i = 0; i < 10 && cursor; i++) {
    const r = http.get(`${list}&cursor=${encodeURIComponent(cursor)}`, Object.assign({ tags: { name: 'list_cursor_page' } }, params));
    check(r, { 'cursor page 200': (x) => x.status === 200 });
    cursorPage.add(r.timings.duration);
    cursor = r.status === 200 ? r.json('data.next_cursor') : null;
  }

  const m = http.get(`${BASE}/v1/tenant/periods/matrix?months=6`, Object.assign({ tags: { name: 'period_matrix' } }, params));
  check(m, { 'matrix 200': (r) => r.status === 200 });
  matrix.add(m.timings.duration);
}
