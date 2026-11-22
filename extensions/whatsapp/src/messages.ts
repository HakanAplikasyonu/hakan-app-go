// İçerik betiği / açılır pencere ↔ arka plan mesajları.

export type Target = "expense" | "income" | "document" | "foreign_invoice";

export const TARGET_LABELS: Record<Target, string> = {
  expense: "Gider olarak kaydet",
  income: "Gelir olarak kaydet",
  document: "Belge olarak kaydet",
  // #780 KDV2: Google Ads, AWS gibi yurt dışı hizmet faturası → KDV2 taslağı.
  foreign_invoice: "Yabancı fatura olarak kaydet",
};

export interface Taxpayer {
  engagement_uuid: string;
  taxpayer_uuid: string;
  title: string;
  tax_id: string;
}

export interface ChatLookup {
  chat_key: string;
  link: { uuid: string; chat_key: string; label: string; taxpayer: Taxpayer } | null;
  suggestions: Taxpayer[];
}

export interface OpenTask {
  uuid: string;
  title: string;
  period: string;
  status: string;
}

export interface Capture {
  uuid: string;
  target: Target;
  file_name: string;
  taxpayer: Taxpayer;
  receipt_uuid: string | null;
  receipt_status: string | null;
  task_uuid: string | null;
  task_title: string | null;
  has_file: boolean;
  replayed: boolean;
}

export interface Status {
  paired: boolean;
  apiBase: string;
  organization: { slug: string; name: string } | null;
  user: { email: string; name: string } | null;
}

export type Request =
  | { type: "status" }
  | { type: "pair"; apiBase: string; code: string; deviceName: string }
  | { type: "unpair" }
  | { type: "lookup"; chatKey: string }
  | { type: "search"; q: string }
  | { type: "link"; engagementUuid: string; chatKey: string; label: string }
  | { type: "tasks"; engagementUuid: string }
  /** Ofis özeti (#780); force: önbelleği (5 dk) atla. */
  | { type: "summary"; force?: boolean }
  | { type: "setWebBase"; webBase: string }
  | {
      type: "capture";
      engagementUuid: string;
      target: Target;
      chatKey: string;
      fileName: string;
      mimeType: string;
      /** base64 (runtime mesajları JSON taşır). */
      data: string;
      taskUuid?: string;
    };

export type Reply<T = unknown> = { ok: true; data: T } | { ok: false; error: string; code?: string };
