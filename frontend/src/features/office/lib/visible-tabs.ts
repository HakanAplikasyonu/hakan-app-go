/** İzni olmayan sekmeleri eler; izinsiz sekme herkese açıktır. */
export function visibleTabs<T extends { permission?: string }>(
  tabs: T[],
  can: (permission: string) => boolean,
): Omit<T, "permission">[] {
  return tabs
    .filter((x) => !x.permission || can(x.permission))
    .map(({ permission: _, ...tab }) => tab);
}
