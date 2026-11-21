"use client";

import { Pencil, Plus, Send, Trash2, UsersRound } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import { usePeriodMatrix } from "@/features/office/hooks/use-office-data";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import {
  useDeleteTaxpayerGroup,
  useSaveTaxpayerGroup,
  useTaxpayerGroup,
  useTaxpayerGroups,
} from "../hooks/use-communication";
import type { TaxpayerGroup } from "../services/communication.service";

/**
 * "Gruplar" sekmesi (#780 B): ofisin ad verdiği sabit mükellef listeleri.
 * Bir grup toplu gönderimde tek tıkla hedef olur.
 */
export function GroupsTab({ onSend }: { onSend: (group: string) => void }) {
  const { t } = useLocale();
  const { permissions: perms } = useTenant();
  const canWrite = perms.includes(permissions.tenant.engagements.write);
  const groups = useTaxpayerGroups();
  const remove = useDeleteTaxpayerGroup();
  const { confirmDelete } = useDialogs();
  // null: kapalı; "": yeni grup; uuid: düzenle.
  const [editing, setEditing] = useState<string | null>(null);

  return (
    <EntitySectionCard title={t("communication.groups_title")}>
      <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <p className="text-muted-foreground max-w-2xl text-sm">
          {t("communication.groups_description")}
        </p>
        {canWrite ? (
          <Button
            size="sm"
            onClick={() => setEditing("")}
            data-testid="group-new"
          >
            <Plus className="size-4" />
            {t("communication.group_new")}
          </Button>
        ) : null}
      </div>
      {groups.isLoading ? <Loading label={t("common.loading")} /> : null}
      {groups.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void groups.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {groups.data && groups.data.items.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("communication.no_groups_yet")}
        </p>
      ) : null}
      <ul
        className="divide-border flex flex-col divide-y"
        data-testid="group-list"
      >
        {(groups.data?.items ?? []).map((g) => (
          <li
            key={g.uuid}
            className="flex flex-wrap items-center justify-between gap-2 py-3"
          >
            <div className="min-w-0">
              <p className="flex items-center gap-2 font-medium">
                <UsersRound className="text-muted-foreground size-4" />
                {g.name}
                <span className="text-muted-foreground text-xs font-normal">
                  {t("communication.member_count", { count: g.member_count })}
                </span>
              </p>
              {g.description ? (
                <p className="text-muted-foreground text-xs">{g.description}</p>
              ) : null}
            </div>
            <div className="flex flex-wrap gap-1.5">
              {canWrite ? (
                <>
                  <Button
                    size="sm"
                    onClick={() => onSend(g.uuid)}
                    disabled={g.member_count === 0}
                  >
                    <Send className="size-3.5" />
                    {t("communication.group_send")}
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => setEditing(g.uuid)}
                    aria-label={t("communication.group_edit")}
                  >
                    <Pencil className="size-3.5" />
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    aria-label={t("communication.group_delete")}
                    disabled={remove.isPending}
                    onClick={async () => {
                      const ok = await confirmDelete({
                        title: t("communication.group_delete_title", {
                          name: g.name,
                        }),
                        description: t(
                          "communication.group_delete_description",
                        ),
                      });
                      if (ok) remove.mutate(g.uuid);
                    }}
                  >
                    <Trash2 className="size-3.5" />
                  </Button>
                </>
              ) : null}
            </div>
          </li>
        ))}
      </ul>
      {editing !== null ? (
        <GroupDialog uuid={editing || null} onClose={() => setEditing(null)} />
      ) : null}
    </EntitySectionCard>
  );
}

function GroupDialog({
  uuid,
  onClose,
}: {
  uuid: string | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const existing = useTaxpayerGroup(uuid);
  if (uuid && !existing.data) {
    return (
      <Dialog open onOpenChange={(o) => (!o ? onClose() : null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("communication.group_edit")}</DialogTitle>
          </DialogHeader>
          <Loading label={t("common.loading")} />
        </DialogContent>
      </Dialog>
    );
  }
  return <GroupForm group={existing.data ?? null} onClose={onClose} />;
}

function GroupForm({
  group,
  onClose,
}: {
  group: TaxpayerGroup | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const save = useSaveTaxpayerGroup();
  const matrix = usePeriodMatrix(1);
  const [name, setName] = useState(group?.name ?? "");
  const [description, setDescription] = useState(group?.description ?? "");
  const [selected, setSelected] = useState<Set<string>>(
    () => new Set(group?.members.map((m) => m.engagement_uuid) ?? []),
  );
  const [query, setQuery] = useState("");

  const rows = useMemo(() => {
    const q = query.trim().toLocaleLowerCase("tr");
    return (matrix.data?.items ?? [])
      .filter(
        (r) =>
          !q ||
          r.taxpayer_title.toLocaleLowerCase("tr").includes(q) ||
          r.taxpayer_tax_id.includes(q),
      )
      .sort((a, b) => a.taxpayer_title.localeCompare(b.taxpayer_title, "tr"));
  }, [matrix.data, query]);

  const toggle = (id: string, on: boolean) =>
    setSelected((s) => {
      const next = new Set(s);
      if (on) next.add(id);
      else next.delete(id);
      return next;
    });

  const submit = () =>
    save.mutate(
      {
        uuid: group?.uuid,
        body: {
          name: name.trim(),
          description: description.trim(),
          engagement_uuids: [...selected],
        },
      },
      {
        onSuccess: () => {
          appToast.success(t("communication.group_saved"));
          onClose();
        },
      },
    );

  return (
    <Dialog open onOpenChange={(o) => (!o ? onClose() : null)}>
      <DialogContent className="max-h-[90vh] max-w-xl overflow-y-auto">
        <DialogHeader>
          <DialogTitle>
            {group
              ? t("communication.group_edit")
              : t("communication.group_new")}
          </DialogTitle>
          <DialogDescription>
            {t("communication.group_form_description")}
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-3">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="group-name">{t("communication.group_name")}</Label>
            <Input
              id="group-name"
              value={name}
              maxLength={80}
              onChange={(e) => setName(e.target.value)}
              placeholder={t("communication.group_name_placeholder")}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="group-description">
              {t("communication.group_description_label")}
            </Label>
            <Textarea
              id="group-description"
              rows={2}
              maxLength={500}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <span className="text-sm font-medium">
              {t("communication.group_members_selected", {
                count: selected.size,
              })}
            </span>
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t("communication.group_search")}
              aria-label={t("communication.group_search")}
            />
            <div className="max-h-64 overflow-y-auto rounded-md border p-1">
              {matrix.isLoading ? (
                <Loading label={t("common.loading")} />
              ) : null}
              {rows.map((r) => {
                const id = `member-${r.engagement_uuid}`;
                return (
                  <div
                    key={r.engagement_uuid}
                    className="hover:bg-surface-2 flex items-center gap-2 rounded px-2 py-1.5"
                  >
                    <Checkbox
                      id={id}
                      checked={selected.has(r.engagement_uuid)}
                      onCheckedChange={(v) =>
                        toggle(r.engagement_uuid, v === true)
                      }
                    />
                    <Label
                      htmlFor={id}
                      className="flex-1 cursor-pointer text-sm font-normal"
                    >
                      {r.taxpayer_title}
                    </Label>
                    <span className="text-muted-foreground text-xs">
                      {r.taxpayer_tax_id}
                    </span>
                  </div>
                );
              })}
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            onClick={submit}
            disabled={name.trim() === "" || save.isPending}
            data-testid="group-save"
          >
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
