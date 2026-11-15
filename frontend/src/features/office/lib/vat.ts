import { fromMinor, mul, sum, toMinor } from "./decimal";
import type { Invoice } from "../types";

/** Fatura tutarını TRY'ye çevirir (dövizliyse faturadaki kurla). */
export function inTry(inv: Invoice, amount: string): string {
  if (inv.currency === "TRY" || !inv.exchangeRate) return amount;
  return mul(amount, inv.exchangeRate);
}

function taxOf(inv: Invoice, code: string) {
  return sum(
    inv.taxes.filter((t) => t.code === code).map((t) => inTry(inv, t.amount)),
  );
}

export type VatCompass = {
  /** Satışlardan hesaplanan KDV (tevkifat düşülmüş). */
  output: string;
  /** Alışlardan indirilecek KDV. */
  input: string;
  /** output − input; eksiyse devreden KDV. */
  net: string;
  salesBase: string;
  purchaseBase: string;
};

/**
 * Dönem KDV tahmini. Yalnızca 0015 (KDV) ve 9015 (KDV tevkifatı) satırları;
 * iade, istisna ve beyan düzeltmeleri müşavirin işi. Gösterimde "tahmini".
 */
export function vatCompass(invoices: Invoice[]): VatCompass {
  const sales = invoices.filter((i) => i.direction === "sales");
  const purchases = invoices.filter((i) => i.direction === "purchase");
  const output =
    toMinor(sum(sales.map((i) => taxOf(i, "0015")))) -
    toMinor(sum(sales.map((i) => taxOf(i, "9015"))));
  const input = toMinor(sum(purchases.map((i) => taxOf(i, "0015"))));
  return {
    output: fromMinor(output),
    input: fromMinor(input),
    net: fromMinor(output - input),
    salesBase: sum(sales.map((i) => inTry(i, i.totals.taxExclusive))),
    purchaseBase: sum(purchases.map((i) => inTry(i, i.totals.taxExclusive))),
  };
}
