//! Kalıcı günlük için kişisel veri maskesi (#204). Masaüstü günlüğü
//! kullanıcının diskinde kalır ve destek için paylaşılabilir: e-posta,
//! VKN/TCKN, IBAN, telefon ve oturum belirteçleri yazılmadan önce maskelenir.
//! Backend'deki maskeleme (`observability.ScrubString`, #179) ile aynı ilke:
//! harf ya da rakamla bitişik numaralar (UUID, uzun fatura no) bozulmaz.

use std::sync::OnceLock;

use regex::{Captures, Regex};

struct Rules {
    bearer: Regex,
    jwt: Regex,
    secret_kv: Regex,
    email: Regex,
    iban: Regex,
    tax_id: Regex,
    phone: Regex,
}

fn rules() -> &'static Rules {
    static R: OnceLock<Rules> = OnceLock::new();
    R.get_or_init(|| Rules {
        bearer: Regex::new(r"(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+").unwrap(),
        jwt: Regex::new(r"\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]+").unwrap(),
        secret_kv: Regex::new(
            r#"(?i)("?(?:refresh_token|access_token|token|password|parola|secret)"?\s*[:=]\s*"?)([^"\s,}&]+)"#,
        )
        .unwrap(),
        email: Regex::new(r"[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}").unwrap(),
        iban: Regex::new(r"(?i)\bTR\d{2}(?:\s?\d{4}){5}\s?\d{2}\b").unwrap(),
        // 10 (VKN) ya da 11 (TCKN) hane; bitişik harf/rakam/tire varsa dokunma.
        tax_id: Regex::new(r"(^|[^A-Za-z0-9-])(\d{10,11})($|[^A-Za-z0-9-])").unwrap(),
        phone: Regex::new(r"(\+90\s?|\b0)5\d{2}\s?\d{3}\s?\d{2}\s?\d{2}\b").unwrap(),
    })
}

/// Bir günlük satırını maskeler.
pub fn mask(line: &str) -> String {
    let r = rules();
    let s = r.bearer.replace_all(line, "Bearer ***");
    let s = r.jwt.replace_all(&s, "***");
    let s = r
        .secret_kv
        .replace_all(&s, |c: &Captures| format!("{}***", &c[1]));
    let s = r.email.replace_all(&s, "***@***");
    let s = r.iban.replace_all(&s, "TR**");
    let s = r.phone.replace_all(&s, "***");
    // Örtüşen eşleşmeler (ardışık numaralar) için iki geçiş.
    let mut out = s.into_owned();
    for _ in 0..2 {
        out = r
            .tax_id
            .replace_all(&out, |c: &Captures| format!("{}***{}", &c[1], &c[3]))
            .into_owned();
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn masks_personal_data() {
        let line = "giriş: ayse.demir@ornek.com.tr VKN 1234567890 TCKN 10000000146 IBAN TR76 0001 0005 1978 6457 8413 26 tel 0532 123 45 67";
        let m = mask(line);
        for leak in ["ayse.demir", "1234567890", "10000000146", "8413", "0532"] {
            assert!(!m.contains(leak), "{leak} sızdı: {m}");
        }
        assert!(m.contains("giriş:"));
    }

    #[test]
    fn masks_tokens() {
        let m = mask(
            r#"auth.resume {"refresh_token":"abc.def-123"} Authorization: Bearer xyz.987 eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjMifQ.c2lnbmF0dXJl password=Gizli123!"#,
        );
        for leak in ["abc.def-123", "xyz.987", "eyJhbGci", "Gizli123"] {
            assert!(!m.contains(leak), "{leak} sızdı: {m}");
        }
    }

    #[test]
    fn keeps_ids_and_invoice_numbers() {
        let line =
            "iş 3f2a1c9e-1234-4abc-9def-0123456789ab fatura ATL2026000000101 süre 1234567 ms";
        assert_eq!(mask(line), line);
    }

    #[test]
    fn adjacent_numbers() {
        assert_eq!(mask("1234567890,10000000146"), "***,***");
    }
}
