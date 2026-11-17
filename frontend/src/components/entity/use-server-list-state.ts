"use client";

import {
  useCallback,
  useMemo,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import type {
  ColumnFiltersState,
  OnChangeFn,
  PaginationState,
  SortingState,
} from "@tanstack/react-table";

import {
  sortingToSortParam,
  sortParamToSorting,
} from "@/components/entity/list-search-params";
import { useDebounce } from "@/hooks/use-debounce";

export type ServerListParams = {
  limit: number;
  offset: number;
  sort?: string;
  q?: string;
};

export type UseServerListStateOptions = {
  initialPageSize?: number;
  initialSort?: string;
  searchDebounceMs?: number;
};

/**
 * Controlled server-list state for DataTable + OpenAPI list endpoints
 * (`limit` / `offset` / `sort` / `q`). Durum bellekte; filtre/sıralama
 * URL'de kalmalıysa `useUrlListState` kullanın.
 */
export function useServerListState(options: UseServerListStateOptions = {}) {
  const {
    initialPageSize = 20,
    initialSort = "-created_at",
    searchDebounceMs = 300,
  } = options;

  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: initialPageSize,
  });
  const [sorting, setSorting] = useState<SortingState>(() =>
    sortParamToSorting(initialSort),
  );
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([]);
  const [globalFilter, setGlobalFilter] = useState("");
  const debouncedSearch = useDebounce(globalFilter, searchDebounceMs);

  const onPaginationChange: OnChangeFn<PaginationState> = useCallback(
    (updater) => {
      setPagination((prev) =>
        typeof updater === "function" ? updater(prev) : updater,
      );
    },
    [],
  );

  const onSortingChange: OnChangeFn<SortingState> = useCallback((updater) => {
    setSorting((prev) =>
      typeof updater === "function" ? updater(prev) : updater,
    );
    setPagination((prev) => ({ ...prev, pageIndex: 0 }));
  }, []);

  const onColumnFiltersChange: OnChangeFn<ColumnFiltersState> = useCallback(
    (updater) => {
      setColumnFilters((prev) =>
        typeof updater === "function" ? updater(prev) : updater,
      );
      setPagination((prev) => ({ ...prev, pageIndex: 0 }));
    },
    [],
  );

  const onGlobalFilterChange: OnChangeFn<string> = useCallback((updater) => {
    // TanStack `resetGlobalFilter()` may pass `undefined` — keep a string.
    setGlobalFilter((prev) => {
      const next = typeof updater === "function" ? updater(prev) : updater;
      return next ?? "";
    });
    setPagination((prev) => ({ ...prev, pageIndex: 0 }));
  }, []);

  const params: ServerListParams = useMemo(() => {
    const q = (debouncedSearch ?? "").trim();
    return {
      limit: pagination.pageSize,
      offset: pagination.pageIndex * pagination.pageSize,
      sort: sortingToSortParam(sorting) ?? initialSort,
      ...(q ? { q } : {}),
    };
  }, [debouncedSearch, initialSort, pagination, sorting]);

  const resetListState = useCallback(() => {
    setPagination({ pageIndex: 0, pageSize: initialPageSize });
    setSorting(sortParamToSorting(initialSort));
    setColumnFilters([]);
    setGlobalFilter("");
  }, [initialPageSize, initialSort]);

  return {
    params,
    pagination,
    setPagination: setPagination as Dispatch<SetStateAction<PaginationState>>,
    sorting,
    columnFilters,
    globalFilter,
    onPaginationChange,
    onSortingChange,
    onColumnFiltersChange,
    onGlobalFilterChange,
    resetListState,
    tableState: {
      pagination,
      onPaginationChange,
      sorting,
      onSortingChange,
      columnFilters,
      onColumnFiltersChange,
      globalFilter,
      onGlobalFilterChange,
    },
  };
}
