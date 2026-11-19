"use client";

import { CornerDownLeft, Sparkles } from "lucide-react";
import { useState } from "react";

import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useLocale } from "@/providers/locale-provider";

import { useCopilot } from "./copilot-provider";

const EXAMPLES = [
  "example_transfer",
  "example_close",
  "example_reports",
] as const;

/** Natural language command input: plans the command, shows it for approval. */
export function CommandBar() {
  const { t } = useLocale();
  const { ask } = useCopilot();
  const [text, setText] = useState("");

  const submit = (value: string) => {
    if (!value.trim()) return;
    ask(value);
    setText("");
  };

  return (
    <form
      className="space-y-2"
      onSubmit={(e) => {
        e.preventDefault();
        submit(text);
      }}
    >
      <Label htmlFor="copilot-command" className="leading-normal">
        {t("office.copilot.command.label")}
      </Label>
      <div className="flex gap-2">
        <div className="relative flex-1">
          <Sparkles
            aria-hidden
            className="text-brand pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2"
          />
          <Input
            id="copilot-command"
            value={text}
            maxLength={500}
            onChange={(e) => setText(e.target.value)}
            placeholder={t("office.copilot.command.placeholder")}
            className="pl-9"
            autoComplete="off"
          />
        </div>
        <Button type="submit" disabled={!text.trim()}>
          <CornerDownLeft aria-hidden className="size-4" />
          {t("office.copilot.command.submit")}
        </Button>
      </div>
      <div className="flex flex-wrap items-center gap-1.5 text-xs">
        <span className="text-text-3">
          {t("office.copilot.command.examples")}
        </span>
        {EXAMPLES.map((k) => (
          <Button
            key={k}
            variant="ghost"
            className="bg-surface-2 hover:bg-surface-3 h-auto rounded-md border px-2.5 py-1 text-xs font-normal"
            onClick={() => submit(t(`office.copilot.command.${k}`))}
          >
            {t(`office.copilot.command.${k}`)}
          </Button>
        ))}
      </div>
      <p className="text-text-3 text-xs">{t("office.copilot.command.hint")}</p>
    </form>
  );
}
