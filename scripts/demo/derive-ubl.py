#!/usr/bin/env python3
"""Demo UBL setini go-ubltr'nin GİB örneklerinden türetir (#17).

Kaynak: github.com/furkanmeclis/go-ubltr testdata/valid (MIT). Yalnızca kimlik
alanları değişir: VKN/TCKN, unvan, kişi adı, ETTN, fatura numarası ve tarih
(2026-03 → 2026-10). Tutarlar, vergi kırılımları ve yapı GİB örneğindeki gibi
kalır; böylece XSD + Schematron geçerliliği korunur (bkz. demo_test.go).

Kullanım (repo kökünden):
    python3 -I scripts/demo/derive-ubl.py "$(cd backend && go list -m -f '{{.Dir}}' github.com/furkanmeclis/go-ubltr)"
"""
import pathlib
import re
import sys
import uuid

SRC = pathlib.Path(sys.argv[1]) / "testdata" / "valid"
OUT = pathlib.Path(__file__).resolve().parents[2] / "backend" / "internal" / "demo" / "ubl"
NS = uuid.UUID("6f1c0b9e-5d2a-4c1e-9a7b-000000000017")

# Demo kimlikleri. Gerçek kişi/firmaya ait değil; frontend demo verisiyle aynı.
ATLAS = ("9000000017", "Atlas Gıda Ticaret A.Ş.")
YILDIZ = ("9100000021", "Yıldız Market Ltd. Şti.")
GIB_A = ("1288331521", "AAA Anonim Şirketi")
GIB_B = ("9205121120", "BBB Limited Şirketi")
GIB_PERSON_TCKN = "12345678901"
SELIN_TCKN = "10000000146"

# çıktı, kaynak, numara eşlemesi, kişi (TCKN, ad, soyad) değişimi
SPECS = [
    ("atlas-01-temel-satis.xml", "temelfatura-satis.xml", {"GIB2026000000001": "ATL2026000000101"}, None),
    ("atlas-02-ticari-satis.xml", "ticarifatura-satis.xml", {"GIB2026000000002": "ATL2026000000102", "IRS2026000000007": "ATI2026000000007"}, None),
    ("atlas-03-tevkifat.xml", "temelfatura-tevkifat.xml", {"GIB2026000000003": "ATL2026000000103"}, None),
    ("atlas-04-istisna.xml", "temelfatura-istisna.xml", {"GIB2026000000004": "ATL2026000000104"}, None),
    ("atlas-05-iade-alis.xml", "temelfatura-iade.xml", {"GIB2026000000005": "YLD2026000000005", "GIB2026000000002": "ATL2026000000102"}, None),
    ("atlas-06-doviz-usd.xml", "ticarifatura-usd.xml", {"GIB2026000000007": "ATL2026000000106"}, None),
    ("atlas-07-earsiv-satis.xml", "earsivfatura-satis.xml", {"EAR2026000000001": "ATE2026000000001"}, None),
    ("selin-01-temel-alis.xml", "temelfatura-satis.xml", {"GIB2026000000001": "ATL2026000000108"}, (SELIN_TCKN, "Selin", "Yılmaz")),
    ("selin-02-earsiv-alis.xml", "earsivfatura-satis.xml", {"EAR2026000000001": "ATE2026000000002"}, (SELIN_TCKN, "Selin", "Yılmaz")),
]


def derive(text: str, numbers: dict, person) -> str:
    for old, new in ((GIB_A, ATLAS), (GIB_B, YILDIZ)):
        text = text.replace(f">{old[0]}<", f">{new[0]}<").replace(old[1], new[1])
    for old, new in numbers.items():
        text = text.replace(old, new)
    if person:
        tckn, first, family = person
        text = text.replace(f">{GIB_PERSON_TCKN}<", f">{tckn}<")
        text = re.sub(r"(<FirstName[^>]*>)[^<]*(<)", rf"\g<1>{first}\2", text)
        text = re.sub(r"(<FamilyName[^>]*>)[^<]*(<)", rf"\g<1>{family}\2", text)
    text = text.replace("2026-03-", "2026-10-")
    main_number = next(iter(numbers.values()))
    ettn = str(uuid.uuid5(NS, main_number)).upper()
    return re.sub(r"(<UUID[^>]*>)[^<]*(<)", rf"\g<1>{ettn}\2", text, count=1)


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    for out, src, numbers, person in SPECS:
        text = (SRC / src).read_text(encoding="utf-8")
        (OUT / out).write_text(derive(text, numbers, person), encoding="utf-8")
        print(f"{out:32} ← {src}")


if __name__ == "__main__":
    main()
