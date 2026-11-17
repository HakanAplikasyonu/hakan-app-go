import { describe, expect, it } from "vitest";

import {
  defaultParse,
  defaultToText,
  displayText,
  isColumnEditable,
  validateCell,
  type EditableGridColumn,
} from "./grid-columns";

type Row = { code: string | null; amount: string | null; locked?: boolean };

const msgs = {
  required: "zorunlu",
  invalidNumber: "sayı",
  invalidOption: "liste",
};

const money: EditableGridColumn<Row> = {
  id: "amount",
  header: "Tutar",
  value: (r) => r.amount,
  editor: "money",
};
const num: EditableGridColumn<Row> = { ...money, id: "n", editor: "number" };
const select: EditableGridColumn<Row> = {
  id: "dir",
  header: "Yön",
  value: () => null,
  editor: "select",
  options: [
    { value: "in", label: "Giriş" },
    { value: "out", label: "Çıkış" },
  ],
};
const code: EditableGridColumn<Row> = {
  id: "code",
  header: "Hesap",
  value: (r) => r.code,
  editor: "text",
  required: true,
  readOnly: (r) => !!r.locked,
  validate: (v) => (/^\d{3}/.test(String(v)) ? null : "kod"),
};
const row: Row = { code: null, amount: null };

describe("defaultParse", () => {
  it("para: tr biçimi kanonik ondalığa, boş → null, çözülemeyen ham kalır", () => {
    expect(defaultParse(money, " 1.234,50 ")).toBe("1234.5");
    expect(defaultParse(money, "")).toBeNull();
    expect(defaultParse(money, "abc")).toBe("abc");
  });
  it("sayı: Number döner", () => {
    expect(defaultParse(num, "12,5")).toBe(12.5);
  });
  it("seçim: değer ya da etiket eşleşir (büyük/küçük harf duyarsız)", () => {
    expect(defaultParse(select, "out")).toBe("out");
    expect(defaultParse(select, "giriş")).toBe("in");
    expect(defaultParse(select, "GİRİŞ")).toBe("in");
    expect(defaultParse(select, "yok")).toBe("yok");
  });
  it("metin kırpılır", () => {
    expect(defaultParse(code, "  600.01 ")).toBe("600.01");
  });
});

describe("metin ve görünüm", () => {
  it("tr'de ondalık virgül; en'de nokta", () => {
    expect(defaultToText(money, "1234.5", "tr")).toBe("1234,5");
    expect(defaultToText(money, "1234.5", "en")).toBe("1234.5");
    expect(defaultToText(money, null, "tr")).toBe("");
    expect(defaultToText(money, "abc", "tr")).toBe("abc");
  });
  it("para ve sayı gruplanarak gösterilir", () => {
    expect(displayText(money, "1234567.5", row, "tr")).toBe("1.234.567,50");
    expect(displayText(money, "-3", row, "en")).toBe("-3.00");
    expect(displayText(num, 1234.25, row, "tr")).toBe("1.234,25");
    expect(displayText(money, "abc", row, "tr")).toBe("abc");
  });

  it("seçim etiketi gösterilir", () => {
    expect(displayText(select, "in", row, "tr")).toBe("Giriş");
  });
});

describe("validateCell", () => {
  it("zorunlu, tür ve kolon kuralı", () => {
    expect(validateCell(code, "", row, msgs)).toBe("zorunlu");
    expect(validateCell(code, "x", row, msgs)).toBe("kod");
    expect(validateCell(code, "600", row, msgs)).toBeNull();
    expect(validateCell(money, "abc", row, msgs)).toBe("sayı");
    expect(validateCell(money, "-12.5", row, msgs)).toBeNull();
    expect(validateCell(num, "12", row, msgs)).toBe("sayı");
    expect(validateCell(select, "yok", row, msgs)).toBe("liste");
    expect(validateCell(money, null, row, msgs)).toBeNull();
  });
});

describe("isColumnEditable", () => {
  it("düzenleyicisiz kolon ve satır bazında kilit", () => {
    expect(isColumnEditable({ ...code, editor: undefined }, row)).toBe(false);
    expect(isColumnEditable(code, row)).toBe(true);
    expect(isColumnEditable(code, { ...row, locked: true })).toBe(false);
  });
});
