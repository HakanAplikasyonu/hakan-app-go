// Sahte Tauri IPC'si (#206): ui/ dosyaları tarayıcıda, kabuk ve sidecar
// olmadan çalışır. Test `window.__mock` ile durumu kurar, olay yollar ve
// çağrıları denetler. Sayfadan önce `addInitScript` ile yüklenir.
(() => {
  const ORG = { uuid: "0b6a3c1e-7d2f-4c55-9a10-3f1e2d4c5b6a", slug: "atlas-ymm", name: "Atlas YMM", status: "active" };
  const ENGAGEMENTS = [
    { uuid: "6f1d2e3c-4b5a-4c7d-8e9f-0a1b2c3d4e5f", taxpayer_uuid: "aa11bb22-cc33-4d44-8e55-ff6677889900", taxpayer_title: "Zeytin Gıda Ltd. Şti.", status: "active" },
    { uuid: "7a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d", taxpayer_uuid: "bb22cc33-dd44-4e55-9f66-007788990011", taxpayer_title: "Ahmet Yılmaz", status: "active" },
  ];
  const m = {
    calls: [],
    handlers: {},
    signedIn: false,
    running: false,
    mfa: true,
    watches: [],
    picked: "/home/ofis/E-Faturalar/Zeytin",
    update: {
      enabled: false,
      disabled_reason: "Bu derlemede otomatik güncelleme kapalı: imza anahtarı tanımlanmamış.",
      current: "0.2.0",
      channel: "stable",
      state: "disabled",
      available: null,
      notes: null,
      checked_at: null,
      error: null,
    },
    status() {
      return {
        base_url: "https://hakan.ornek.com",
        signed_in: this.signedIn,
        organization: this.running ? ORG : null,
        agent_uuid: this.running ? "c0ffee00-1111-4222-8333-444455556666" : undefined,
        agent_name: this.running ? "Muhasebe masası" : undefined,
        machine_id: "hk-0123456789abcdef0123456789abcdef",
        running: this.running,
        last_heartbeat: this.running ? new Date().toISOString() : undefined,
        watches: this.watches,
        stats: { watched_folders: this.watches.length, uploaded: 3, failed: 1 },
        uploaded_total: 3,
        heartbeat_error: this.heartbeatError || undefined,
      };
    },
    emit(name, payload) {
      for (const h of this.handlers[name] || []) h({ event: name, payload });
    },
    sidecar(method, p) {
      switch (method) {
        case "agent.status":
          return this.status();
        case "api.health":
          return { ok: true, app_name: "Hakan", latency_ms: 12 };
        case "auth.login":
          if (p.password !== "dogru") throw { code: "INVALID_CREDENTIALS", message: "invalid email or password" };
          if (this.mfa && !p.totp_code) throw { code: "MFA_REQUIRED", message: "mfa required" };
          this.signedIn = true;
          return { organizations: [ORG, { ...ORG, slug: "eski", name: "Kapanmış Ofis", status: "suspended" }] };
        case "agent.organizations":
          return { organizations: [ORG] };
        case "agent.use_organization":
          this.running = true;
          return this.status();
        case "agent.engagements":
          return { items: ENGAGEMENTS };
        case "watch.add":
          this.watches.push({
            path: p.path,
            engagement_uuid: p.engagement_uuid,
            label: p.label,
            taxpayer_title: p.taxpayer_title,
            activity: {
              uploaded: 2,
              last_file: "fatura-002.xml",
              last_outcome: "rejected",
              last_upload_at: new Date(Date.now() - 5 * 60000).toISOString(),
              errors: [
                { name: "bozuk.xml", message: "Dosya e-fatura (UBL-TR) değil; yüklenmedi.", retry: false, at: new Date().toISOString() },
              ],
            },
          });
          return this.watches.at(-1);
        case "watch.remove":
          this.watches = this.watches.filter((w) => w.path !== p.path);
          return this.status();
        case "watch.clear_errors":
          for (const w of this.watches) if (w.path === p.path) w.activity.errors = [];
          return this.status();
        case "auth.logout":
          this.signedIn = false;
          this.running = false;
          return this.status();
        case "ping":
          return { pong: true };
        case "source.test":
          return { ok: false, tables: ["LG_001_01_INVOICE", "LG_001_CLCARD", "LG_001_01_STFICHE"], missing: ["LG_001_CLCARD"] };
        case "source.preview":
          return {
            source: "logo_tiger",
            total: 2,
            skipped: 0,
            truncated: false,
            by_direction: { sales: 1, purchase: 1 },
            warning_counts: {},
            batch_warnings: [],
            sample: [
              { doc_no: "ABC2026000000012", date: "2026-10-03", direction: "sales", counterparty_title: "Deniz Market", guid: "11112222-3333-4444-8555-666677778888", ettn: "99998888-7777-4666-8555-444433332222", totals: { net: "1180.50" }, raw: { LOGICALREF: 5 } },
              { doc_no: "XYZ2026000000034", date: "2026-10-09", direction: "purchase", counterparty_title: "Kırtasiye A.Ş.", totals: { net: "240" }, raw: {} },
            ],
          };
        default:
          throw { code: "FORBIDDEN_METHOD", message: `izin verilmeyen yöntem: ${method}` };
      }
    },
    async invoke(cmd, args = {}) {
      this.calls.push({ cmd, args: JSON.parse(JSON.stringify(args)) });
      switch (cmd) {
        case "sidecar_call":
          return this.sidecar(args.method, args.params || {});
        case "sidecar_status":
          return { alive: true, ready: { version: "0.2.0", commit: "abc1234", os: "linux", arch: "amd64", pid: 4242 } };
        case "sidecar_restart":
          return null;
        case "pick_folder":
          return this.picked;
        case "shell_settings":
          return { autostart: false, paused: false };
        case "set_autostart_enabled":
          return args.enabled;
        case "set_watch_paused":
          return args.paused;
        case "update_status":
        case "update_check":
          return this.update;
        case "update_set_channel":
          this.update = { ...this.update, channel: args.channel };
          return this.update;
        default:
          throw `bilinmeyen komut: ${cmd}`;
      }
    },
  };
  window.__mock = m;
  window.__TAURI__ = {
    core: { invoke: (cmd, args) => m.invoke(cmd, args) },
    event: {
      listen: async (name, h) => {
        (m.handlers[name] ||= []).push(h);
        return () => {};
      },
    },
    app: { getVersion: async () => "0.2.0" },
  };
})();
