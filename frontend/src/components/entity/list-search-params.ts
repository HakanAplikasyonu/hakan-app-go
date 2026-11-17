import type {
  ColumnFiltersState,
  PaginationState,
  SortingState,
} from "@tanstack/react-table";

/**
 * Sunucu sayfalı listelerin URL durumu (#381). Saf yardımcılar: hook'tan
 * bağımsız test edilir.
 *
 * Anahtarlar (önek ile, ör. `prefix: "inv_"` → `inv_page`):
 *   page  1 tabanlı sayfa (1 ise yazılmaz)
 *   size  sayfa boyutu (varsayılansa yazılmaz)
 *   sort  "-created_at,name" (varsayılansa yazılmaz)
 *   q     arama
 *   <filterKey>  kolon süzgeci; çoklu değer virgülle
 */
export type ListUrlOptions = {
  defaultPageSize: number;
  defaultSort?: string;
  /** URL'ye yazılan kolon süzgeci kimlikleri. */
  filterKeys?: readonly string[];
  prefix?: string;
};

export type ListUrlState = {
  pagination: PaginationState;
  sorting: SortingState;
  q: string;
  columnFilters: ColumnFiltersState;
};

type ReadableParams = { get(name: string): string | null };

export function sortingToSortParam(sorting: SortingState): string | undefined {
  if (!sorting.length) return undefined;
  return sorting.map((item) => (item.desc ? `-${item.id}` : item.id)).join(",");
}

export function sortParamToSorting(sort?: string | null): SortingState {
  if (!sort) return [];
  return sort
    .split(",")
    .filter(Boolean)
    .map((token) =>
      token.startsWith("-")
        ? { id: token.slice(1), desc: true }
        : { id: token, desc: false },
    );
}

function positiveInt(raw: string | null, fallback: number): number {
  const n = Number(raw);
  return Number.isInteger(n) && n > 0 ? n : fallback;
}

export function parseListSearchParams(
  params: ReadableParams,
  options: ListUrlOptions,
): ListUrlState {
  const p = options.prefix ?? "";
  const pageSize = positiveInt(params.get(`${p}size`), options.defaultPageSize);
  const page = positiveInt(params.get(`${p}page`), 1);
  const sortRaw = params.get(`${p}sort`);
  const columnFilters: ColumnFiltersState = [];
  for (const key of options.filterKeys ?? []) {
    const raw = params.get(`${p}${key}`);
    if (!raw) continue;
    const values = raw.split(",").filter(Boolean);
    if (values.length) columnFilters.push({ id: key, value: values });
  }
  return {
    pagination: { pageIndex: page - 1, pageSize },
    sorting: sortParamToSorting(sortRaw ?? options.defaultSort),
    q: params.get(`${p}q`) ?? "",
    columnFilters,
  };
}

export type ListUrlPatch = Partial<{
  pagination: PaginationState;
  sorting: SortingState;
  q: string;
  columnFilters: ColumnFiltersState;
  /** Sayfaya özel ek anahtarlar; "" / null siler. Önek uygulanmaz. */
  extra: Record<string, string | null | undefined>;
}>;

function filterValueToParam(value: unknown): string {
  if (Array.isArray(value)) return value.map(String).join(",");
  if (value === null || value === undefined) return "";
  return String(value);
}

/**
 * Mevcut URL parametrelerine yamayı uygular. Süzgeç, arama veya sıralama
 * değişirse sayfa başa döner. Varsayılan değerler yazılmaz (temiz URL).
 */
export function applyListSearchParams(
  current: string | URLSearchParams,
  patch: ListUrlPatch,
  options: ListUrlOptions,
): URLSearchParams {
  const p = options.prefix ?? "";
  const next = new URLSearchParams(current);
  const set = (key: string, value: string | undefined) => {
    if (value === undefined || value === "") next.delete(key);
    else next.set(key, value);
  };

  let resetPage = false;
  if (patch.sorting) {
    const sort = sortingToSortParam(patch.sorting);
    // Sıralama kaldırılırsa anahtar silinir: varsayılan sıralamaya dönülür
    // (sunucu da aynı varsayılanı uygular).
    set(`${p}sort`, sort === options.defaultSort ? undefined : sort);
    resetPage = true;
  }
  if (patch.q !== undefined) {
    set(`${p}q`, patch.q.trim() || undefined);
    resetPage = true;
  }
  if (patch.columnFilters) {
    for (const key of options.filterKeys ?? []) {
      const f = patch.columnFilters.find((item) => item.id === key);
      set(`${p}${key}`, f ? filterValueToParam(f.value) : undefined);
    }
    resetPage = true;
  }
  if (patch.extra) {
    for (const [key, value] of Object.entries(patch.extra)) {
      set(key, value ?? undefined);
    }
  }
  if (patch.pagination) {
    const { pageIndex, pageSize } = patch.pagination;
    set(
      `${p}size`,
      pageSize === options.defaultPageSize ? undefined : String(pageSize),
    );
    set(`${p}page`, pageIndex > 0 ? String(pageIndex + 1) : undefined);
  } else if (resetPage) {
    next.delete(`${p}page`);
  }
  return next;
}

/** `?a=1` biçiminde sorgu dizesi; boşsa yalnızca yol. */
export function toHref(pathname: string, params: URLSearchParams): string {
  const qs = params.toString();
  return qs ? `${pathname}?${qs}` : pathname;
}
