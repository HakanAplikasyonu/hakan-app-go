//! OS anahtar zinciri: macOS Keychain, Windows Credential Manager, Linux
//! Secret Service. API anahtarı ya da oturum belirteci diske düz yazılmaz.
//! Değer ön yüze yalnızca `secret_get` ile, istenince döner.

use keyring::Entry;

pub const SERVICE: &str = "app.hakan.desktop";

/// Anahtar adı: küçük harf, rakam, `_ . -`; 1–64 karakter.
pub fn valid_key(key: &str) -> bool {
    !key.is_empty()
        && key.len() <= 64
        && key.bytes().all(|c| {
            c.is_ascii_lowercase() || c.is_ascii_digit() || matches!(c, b'_' | b'.' | b'-')
        })
}

fn entry(key: &str) -> Result<Entry, String> {
    if !valid_key(key) {
        return Err(format!("geçersiz anahtar adı: {key:?}"));
    }
    Entry::new(SERVICE, key).map_err(|e| e.to_string())
}

/// Kabuk içi kullanım (ön yüze açık değil).
pub(crate) fn store(key: &str, value: &str) -> Result<(), String> {
    entry(key)?.set_password(value).map_err(|e| e.to_string())
}

pub(crate) fn load(key: &str) -> Result<Option<String>, String> {
    match entry(key)?.get_password() {
        Ok(v) => Ok(Some(v)),
        Err(keyring::Error::NoEntry) => Ok(None),
        Err(e) => Err(e.to_string()),
    }
}

pub(crate) fn remove(key: &str) -> Result<(), String> {
    match entry(key)?.delete_credential() {
        Ok(()) | Err(keyring::Error::NoEntry) => Ok(()),
        Err(e) => Err(e.to_string()),
    }
}

/// Ön yüzün erişemeyeceği anahtarlar: oturum belirteci yalnızca kabukta.
fn reserved(key: &str) -> bool {
    key == "refresh_token"
}

fn public_key(key: &str) -> Result<(), String> {
    if reserved(key) {
        return Err(format!("{key:?} yalnızca kabuk tarafından kullanılır"));
    }
    Ok(())
}

#[tauri::command]
pub fn secret_set(key: String, value: String) -> Result<(), String> {
    public_key(&key)?;
    if value.is_empty() {
        return Err("boş değer saklanmaz; silmek için secret_delete".into());
    }
    store(&key, &value)
}

#[tauri::command]
pub fn secret_get(key: String) -> Result<Option<String>, String> {
    public_key(&key)?;
    load(&key)
}

/// Değeri döndürmeden yalnızca var olup olmadığını söyler (arayüz göstergesi).
#[tauri::command]
pub fn secret_exists(key: String) -> Result<bool, String> {
    public_key(&key)?;
    Ok(load(&key)?.is_some())
}

#[tauri::command]
pub fn secret_delete(key: String) -> Result<(), String> {
    public_key(&key)?;
    remove(&key)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn reserved_keys_stay_in_shell() {
        assert!(secret_get("refresh_token".into()).is_err());
        assert!(secret_set("refresh_token".into(), "x".into()).is_err());
        assert!(secret_delete("refresh_token".into()).is_err());
    }

    #[test]
    fn key_names() {
        assert!(valid_key("api_token"));
        assert!(valid_key("office.default-token"));
        assert!(!valid_key(""));
        assert!(!valid_key("API"));
        assert!(!valid_key("a/b"));
        assert!(!valid_key(&"a".repeat(65)));
    }
}

#[cfg(test)]
mod keychain_tests {
    use super::*;

    /// Gerçek OS anahtar zinciriyle; masaüstü oturumu gerektirdiği için
    /// varsayılan olarak atlanır: `cargo test -- --ignored keychain`.
    #[test]
    #[ignore]
    fn keychain_roundtrip() {
        let key = "test.roundtrip".to_string();
        secret_delete(key.clone()).unwrap();
        assert_eq!(secret_get(key.clone()).unwrap(), None);
        secret_set(key.clone(), "s3cr3t".into()).unwrap();
        assert_eq!(secret_get(key.clone()).unwrap().as_deref(), Some("s3cr3t"));
        assert!(secret_exists(key.clone()).unwrap());
        secret_delete(key.clone()).unwrap();
        assert!(!secret_exists(key).unwrap());
    }
}
