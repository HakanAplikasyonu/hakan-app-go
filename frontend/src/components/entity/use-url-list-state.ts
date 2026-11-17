"use client";

import type {
  ColumnFiltersState,
  OnChangeFn,
  PaginationState,
  SortingState,
} from "@tanstack/react-table";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import {
  applyListSearchParams,
  parseListSearchParams,
  sortingToSortParam,
  toHref,
  type ListUrlOptions,
  type ListUrlPatch,
} from "@/components/entity/list-search-params";
import type { ServerListParams } from "@/components/entity/use-server-list-state";
import { useDebounce } from "@/hooks/use-debounce";

export type UseUrlListStateOptions = {
  initialPageSize?: number;
  /** Sunucunun varsayılan sıralaması; URL'ye yazılmaz. Boş: sıralama yok. */
  initialSort?: string;
  searchDebounceMs?: number;
  /** URL'de tutulan kolon süzgeçleri (faceted kolon kimlikleri). */
  filterKeys?: readonly string[];
  /** Aynı sayfada iki liste varsa anahtar öneki. */
  prefix?: string;
};

export type UrlServerListParams = ServerListParams & {
  /** URL'deki kolon süzgeçleri: kimlik → seçili değerler. */
  filters: Record<string, string[]>;
};

/**
 * `useServerListState` ile aynı biçim, ama durum URL'de (#381): sayfa,
 * sayfa boyutu, sıralama, arama ve seçili kolon süzgeçleri sorgu
 * dizesinde durur; geri tuşu ve paylaşılan bağlantı aynı listeyi açar.
 * Sayfaya özel anahtarlar (ör. açık çekmece) `getParam` / `setParams` ile.
 *
 * `useSearchParams` kullandığı için çağıran bileşen Suspense içinde olmalı.
 */
export function useUrlListState(options: UseUrlListStateOptions = {}) {
  const {
    initialPageSize = 20,
    initialSort,
    searchDebounceMs = 300,
    filterKeys,
    prefix,
  } = options;
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();

  // filterKeys çoğunlukla satır içi dizi; içeriğe göre karşılaştır.
  const filterKeysKey = (filterKeys ?? []).join("|");
  const urlOptions: ListUrlOptions = useMemo(
    () => ({
      defaultPageSize: initialPageSize,
      defaultSort: initialSort,
      filterKeys: filterKeysKey ? filterKeysKey.split("|") : [],
      prefix,
    }),
    [initialPageSize, initialSort, prefix, filterKeysKey],
  );

  const state = useMemo(
    () => parseListSearchParams(searchParams, urlOptions),
    [searchParams, urlOptions],
  );

  // Son yazılan sorgu: aynı tikte gelen iki güncelleme birbirini ezmesin.
  const pendingRef = useRef<string | null>(null);
  const searchKey = searchParams.toString();
  useEffect(() => {
    pendingRef.current = null;
  }, [searchKey]);

  const write = useCallback(
    (patch: ListUrlPatch) => {
      const base = pendingRef.current ?? searchParams.toString();
      const next = applyListSearchParams(base, patch, urlOptions);
      const nextStr = next.toString();
      if (nextStr === base) return;
      pendingRef.current = nextStr;
      router.replace(toHref(pathname, next), { scroll: false });
    },
    [pathname, router, searchParams, urlOptions],
  );

  // Arama kutusu anında yanıt verir; URL'ye gecikmeli yazılır.
  const [searchInput, setSearchInput] = useState(state.q);
  const debouncedSearch = useDebounce(searchInput, searchDebounceMs);
  useEffect(() => {
    if (debouncedSearch.trim() !== state.q.trim()) {
      write({ q: debouncedSearch });
    }
    // Yalnızca yazılan değer değişince; URL'den gelen q değişimi aşağıda.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [debouncedSearch]);

  const onPaginationChange: OnChangeFn<PaginationState> = useCallback(
    (updater) => {
      const next =
        typeof updater === "function" ? updater(state.pagination) : updater;
      write({ pagination: next });
    },
    [state.pagination, write],
  );

  const onSortingChange: OnChangeFn<SortingState> = useCallback(
    (updater) => {
      const next =
        typeof updater === "function" ? updater(state.sorting) : updater;
      write({ sorting: next });
    },
    [state.sorting, write],
  );

  const onColumnFiltersChange: OnChangeFn<ColumnFiltersState> = useCallback(
    (updater) => {
      const next =
        typeof updater === "function" ? updater(state.columnFilters) : updater;
      write({ columnFilters: next });
    },
    [state.columnFilters, write],
  );

  const onGlobalFilterChange: OnChangeFn<string> = useCallback((updater) => {
    setSearchInput((prev) => {
      const next = typeof updater === "function" ? updater(prev) : updater;
      return next ?? "";
    });
  }, []);

  const params: UrlServerListParams = useMemo(() => {
    const q = state.q.trim();
    const filters: Record<string, string[]> = {};
    for (const f of state.columnFilters) {
      filters[f.id] = (f.value as string[]) ?? [];
    }
    const sort = sortingToSortParam(state.sorting) ?? initialSort;
    return {
      limit: state.pagination.pageSize,
      offset: state.pagination.pageIndex * state.pagination.pageSize,
      ...(sort ? { sort } : {}),
      ...(q ? { q } : {}),
      filters,
    };
  }, [initialSort, state]);

  const getParam = useCallback(
    (key: string) => searchParams.get(key),
    [searchParams],
  );

  /**
   * Sayfaya özel anahtarları yaz. `resetPage` (varsayılan true) liste
   * süzgeci gibi davranır; çekmece gibi anahtarlarda false verin.
   */
  const setParams = useCallback(
    (
      extra: Record<string, string | null | undefined>,
      opts: { resetPage?: boolean } = {},
    ) => {
      const resetPage = opts.resetPage ?? true;
      write(
        resetPage
          ? {
              extra,
              pagination: { ...state.pagination, pageIndex: 0 },
            }
          : { extra },
      );
    },
    [state.pagination, write],
  );

  const resetListState = useCallback(() => {
    setSearchInput("");
    write({
      q: "",
      sorting: [],
      columnFilters: [],
      pagination: { pageIndex: 0, pageSize: initialPageSize },
    });
  }, [initialPageSize, write]);

  return {
    params,
    pagination: state.pagination,
    sorting: state.sorting,
    columnFilters: state.columnFilters,
    globalFilter: searchInput,
    onPaginationChange,
    onSortingChange,
    onColumnFiltersChange,
    onGlobalFilterChange,
    resetListState,
    getParam,
    setParams,
    tableState: {
      pagination: state.pagination,
      onPaginationChange,
      sorting: state.sorting,
      onSortingChange,
      columnFilters: state.columnFilters,
      onColumnFiltersChange,
      globalFilter: searchInput,
      onGlobalFilterChange,
    },
  };
}
