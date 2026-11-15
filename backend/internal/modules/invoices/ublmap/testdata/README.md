# Test faturaları

- `earsivfatura-satis.xml`, `ihracat-istisna.xml`, `temelfatura-*.xml`,
  `ticarifatura-satis.xml`, `ticarifatura-usd.xml`: go-ubltr `testdata/valid`
  (MIT, Copyright (c) 2026 Furkan Meclis). XSD ve GİB Schematron'dan geçen
  örnekler.
- `ticarifatura-uc-oran-iskonto.xml` (%1 / %10 / %20, satır iskontosu) ve
  `earsiv-nihai-tuketici.xml` (TCKN 11111111111): go-ubltr `builder` ile
  üretildi. İlki XSD + Schematron'dan geçiyor; ikincisi e-Arşiv olduğu için
  yalnızca `earchive` tipinde Schematron'dan geçer.

Bütün VKN/TCKN değerleri GİB örneklerindeki sahte değerlerdir.
