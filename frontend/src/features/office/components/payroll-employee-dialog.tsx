"use client";

import { useMutation } from "@tanstack/react-query";
import { useId, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
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
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

import {
  payrollService,
  type PayrollEmployee,
  type PayrollEmployeeInput,
} from "../services/payroll.service";

type Form = {
  full_name: string;
  national_id: string;
  sgk_no: string;
  job_title: string;
  department: string;
  iban: string;
  hire_date: string;
  exit_date: string;
  exit_reason: string;
};

type TextField = Exclude<keyof Form, "hire_date" | "exit_date">;

function initial(e: PayrollEmployee | null): Form {
  return {
    full_name: e?.full_name ?? "",
    national_id: "",
    sgk_no: e?.sgk_no ?? "",
    job_title: e?.job_title ?? "",
    department: e?.department ?? "",
    iban: e?.iban ?? "",
    hire_date: e?.hire_date ?? "",
    exit_date: e?.exit_date ?? "",
    exit_reason: e?.exit_reason ?? "",
  };
}

/**
 * Yeni kayıtta yalnız dolu alanlar gider. Düzenlemede boş alan temizlenir;
 * TC kimlik no ise sunucuda maskeli durduğu için boş bırakılınca değişmez.
 */
function toBody(f: Form, editing: boolean): PayrollEmployeeInput {
  const body: PayrollEmployeeInput = {};
  for (const [k, v] of Object.entries(f) as [keyof Form, string][]) {
    const t = v.trim();
    if (k === "national_id" || !editing) {
      if (t) body[k] = t;
    } else {
      body[k] = t;
    }
  }
  return body;
}

/** Sunucunun doğrulama yanıtını kullanıcının anlayacağı cümleye çevirir. */
function errorKey(e: unknown): string | null {
  if (!isApiError(e)) return null;
  if (e.code === "EMPLOYEE_EXISTS") return "office.payroll.employee_err_exists";
  const m = e.message;
  if (m.includes("full_name")) return "office.payroll.employee_err_name";
  if (m.includes("national_id")) return "office.payroll.employee_err_tckn";
  if (m.includes("iban")) return "office.payroll.employee_err_iban";
  if (m.includes("exit_date")) return "office.payroll.employee_err_dates";
  if (m.includes("too long")) return "office.payroll.employee_err_long";
  return null;
}

/**
 * Bordro personelini elle ekleme / düzenleme penceresi (#669 J). Yalnız
 * açıkken çizilir; her açılışta form baştan kurulur.
 */
export function PayrollEmployeeDialog({
  engagement,
  employee,
  onClose,
  onSaved,
}: {
  engagement: string;
  /** null: yeni personel. */
  employee: PayrollEmployee | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const id = useId();
  const editing = employee !== null;
  const [form, setForm] = useState<Form>(() => initial(employee));
  const [error, setError] = useState<string | null>(null);

  const save = useMutation({
    mutationFn: (f: Form) =>
      employee
        ? payrollService.updateEmployee(
            slug,
            engagement,
            employee.uuid,
            toBody(f, true),
          )
        : payrollService.createEmployee(slug, engagement, toBody(f, false)),
    onSuccess: (e) => {
      toast.success(
        t(
          editing
            ? "office.payroll.employee_updated"
            : "office.payroll.employee_created",
          { name: e.full_name },
        ),
      );
      onSaved();
      onClose();
    },
    onError: (e) => {
      const key = errorKey(e);
      setError(
        key
          ? t(key)
          : isApiError(e) && e.message
            ? e.message
            : t("office.payroll.action_failed"),
      );
    },
  });

  const set = (k: keyof Form, v: string) => {
    setForm((f) => ({ ...f, [k]: v }));
    setError(null);
  };

  const text = (
    k: TextField,
    label: string,
    opts: { required?: boolean; mono?: boolean; hint?: string } = {},
  ) => (
    <div className="flex flex-col gap-1 text-sm">
      <Label htmlFor={`${id}-${k}`} className="leading-normal font-normal">
        {label}
      </Label>
      <Input
        id={`${id}-${k}`}
        required={opts.required}
        className={opts.mono ? "font-mono" : undefined}
        value={form[k]}
        onChange={(e) => set(k, e.target.value)}
        aria-describedby={opts.hint ? `${id}-${k}-hint` : undefined}
      />
      {opts.hint ? (
        <p id={`${id}-${k}-hint`} className="text-text-3 text-xs">
          {opts.hint}
        </p>
      ) : null}
    </div>
  );

  const date = (k: "hire_date" | "exit_date", label: string) => (
    <div className="flex flex-col gap-1 text-sm">
      <Label htmlFor={`${id}-${k}`} className="leading-normal font-normal">
        {label}
      </Label>
      <DatePicker
        id={`${id}-${k}`}
        value={form[k]}
        onChange={(v) => set(k, v)}
      />
    </div>
  );

  return (
    <Dialog open onOpenChange={(v) => (!v ? onClose() : undefined)}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {editing
              ? t("office.payroll.employee_edit_title")
              : t("office.payroll.employee_add_title")}
          </DialogTitle>
          <DialogDescription>
            {t("office.payroll.employee_form_hint")}
          </DialogDescription>
        </DialogHeader>
        <form
          id={`${id}-form`}
          className="grid gap-3 sm:grid-cols-2"
          onSubmit={(e) => {
            e.preventDefault();
            setError(null);
            save.mutate(form);
          }}
        >
          {text("full_name", t("office.payroll.field_employee_name"), {
            required: true,
          })}
          {text("national_id", t("office.payroll.field_national_id"), {
            mono: true,
            hint:
              editing && employee.national_id_masked
                ? t("office.payroll.employee_tckn_keep", {
                    masked: employee.national_id_masked,
                  })
                : undefined,
          })}
          {text("sgk_no", t("office.payroll.field_sgk_no"))}
          {text("job_title", t("office.payroll.field_job_title"))}
          {text("department", t("office.payroll.field_department"))}
          {text("iban", t("office.payroll.employee_field_iban"), {
            mono: true,
          })}
          {date("hire_date", t("office.payroll.field_hire_date"))}
          {date("exit_date", t("office.payroll.field_exit_date"))}
          <div className="sm:col-span-2">
            {text(
              "exit_reason",
              t("office.payroll.employee_field_exit_reason"),
            )}
          </div>
        </form>
        {error ? (
          <p
            role="alert"
            className="border-danger/40 bg-danger-tint text-danger rounded-lg border p-3 text-sm"
          >
            {error}
          </p>
        ) : null}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("office.payroll.employee_cancel")}
          </Button>
          <Button
            type="submit"
            form={`${id}-form`}
            disabled={save.isPending || !form.full_name.trim()}
          >
            {t("office.payroll.employee_save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
