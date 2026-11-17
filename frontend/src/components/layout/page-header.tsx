import type { ReactNode } from "react";

import {
  Breadcrumb,
  type BreadcrumbItem,
} from "@/components/layout/breadcrumb";

type PageHeaderProps = {
  title: string;
  description?: string;
  icon?: ReactNode;
  breadcrumbs?: BreadcrumbItem[];
  actions?: ReactNode;
  /**
   * Başlık düzeyi. Mükellef detayında sayfanın üstünde mükellef başlığı
   * (h1) zaten olduğundan sekme sayfaları h2 kullanır (#468).
   */
  headingLevel?: 1 | 2;
};

export function PageHeader({
  title,
  description,
  icon,
  breadcrumbs,
  actions,
  headingLevel = 1,
}: PageHeaderProps) {
  const Heading = headingLevel === 2 ? "h2" : "h1";
  return (
    <div className="mb-2 space-y-3">
      {breadcrumbs?.length ? <Breadcrumb items={breadcrumbs} /> : null}
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <Heading
            className={
              headingLevel === 2
                ? "font-display flex items-center gap-2.5 text-xl font-semibold tracking-tight"
                : "font-display flex items-center gap-2.5 text-2xl font-semibold tracking-tight"
            }
          >
            {icon ? (
              <span className="text-muted-foreground inline-flex shrink-0">
                {icon}
              </span>
            ) : null}
            {title}
          </Heading>
          {description ? (
            <p className="text-muted-foreground">{description}</p>
          ) : null}
        </div>
        {actions ? (
          <div className="flex items-center gap-2">{actions}</div>
        ) : null}
      </div>
    </div>
  );
}
