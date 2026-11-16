//! Güncelleyici uçtan uca testi (#205): eski sürüm (0.1.0) yerel bir test
//! manifestinde yeni sürümü (0.2.0) görür, paketi indirir, geçici anahtar
//! çiftiyle imzasını doğrular ve kurulu ikilinin yerine koyar.
//!
//! - Anahtar çifti her çalıştırmada üretilir; gerçek anahtar kullanılmaz.
//! - Manifest ve paket yerel bir HTTP sunucusundan (127.0.0.1) gelir.
//! - "Kurulu uygulama" geçici klasördeki bir ELF dosyasıdır (Linux AppImage
//!   kurulum yolu); kurulumdan sonra içeriği yeni paket olmalıdır.
//! - Yanlış anahtarla imzalanmış paket kurulmaz, dosya değişmez.

#![cfg(target_os = "linux")]

use std::io::{BufRead, BufReader, Cursor, Write};
use std::net::TcpListener;
use std::os::unix::fs::PermissionsExt;
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::thread;

use base64::Engine;
use hakan_desktop_lib::updater::{self, Channel, Config, Progress};
use tauri::test::{mock_builder, mock_context, noop_assets};

struct Keys {
    pk_b64: String,
    pair: minisign::KeyPair,
}

fn keys() -> Keys {
    let pair = minisign::KeyPair::generate_unencrypted_keypair().unwrap();
    let pk_box = pair.pk.to_box().unwrap().to_string();
    Keys {
        pk_b64: base64::engine::general_purpose::STANDARD.encode(pk_box),
        pair,
    }
}

/// Tauri `.sig` biçimi: minisign imza kutusunun base64'ü.
fn sign(keys: &Keys, data: &[u8], version: &str) -> String {
    let trusted = format!("timestamp:0\tfile:hakan.AppImage\tversion:{version}");
    let sig = minisign::sign(
        Some(&keys.pair.pk),
        &keys.pair.sk,
        Cursor::new(data),
        Some(&trusted),
        Some("signature from tauri secret key"),
    )
    .unwrap();
    base64::engine::general_purpose::STANDARD.encode(sig.to_string())
}

/// Yerel bir port açar; adresi manifestte kullanılabilsin diye önce döner.
fn bind() -> (TcpListener, String) {
    let listener = TcpListener::bind("127.0.0.1:0").unwrap();
    let base = format!("http://{}", listener.local_addr().unwrap());
    (listener, base)
}

/// Yolları sabit içerikle yanıtlayan küçük HTTP sunucusu.
fn serve(listener: TcpListener, routes: Vec<(String, Vec<u8>)>) {
    let routes = Arc::new(routes);
    thread::spawn(move || {
        for stream in listener.incoming() {
            let Ok(mut stream) = stream else { continue };
            let routes = routes.clone();
            thread::spawn(move || {
                let mut reader = BufReader::new(stream.try_clone().unwrap());
                let mut line = String::new();
                if reader.read_line(&mut line).is_err() {
                    return;
                }
                let path = line.split_whitespace().nth(1).unwrap_or("").to_string();
                loop {
                    let mut h = String::new();
                    if reader.read_line(&mut h).unwrap_or(0) == 0 || h == "\r\n" {
                        break;
                    }
                }
                let (status, body) = match routes.iter().find(|(p, _)| *p == path) {
                    Some((_, b)) => ("200 OK", b.clone()),
                    None => ("404 Not Found", Vec::new()),
                };
                let _ = write!(
                    stream,
                    "HTTP/1.1 {status}\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
                    body.len()
                );
                let _ = stream.write_all(&body);
            });
        }
    });
}

fn platform_key() -> String {
    format!("linux-{}", std::env::consts::ARCH)
}

fn manifest(base: &str, version: &str, signature: &str) -> Vec<u8> {
    serde_json::to_vec(&serde_json::json!({
        "version": version,
        "notes": "Test sürümü",
        "pub_date": "2026-11-16T00:00:00Z",
        "platforms": {
            platform_key(): {
                "signature": signature,
                "url": format!("{base}/stable/hakan_{version}_amd64.AppImage"),
            }
        }
    }))
    .unwrap()
}

/// Kopyalanan ELF ikilisi: "kurulu eski uygulama" ya da "yeni paket".
fn elf_bytes(name: &str) -> Vec<u8> {
    for dir in ["/usr/bin", "/bin"] {
        if let Ok(b) = std::fs::read(Path::new(dir).join(name)) {
            return b;
        }
    }
    panic!("{name} bulunamadı");
}

fn install_dir() -> PathBuf {
    let d = std::env::temp_dir().join(format!(
        "hakan-updater-e2e-{}-{}",
        std::process::id(),
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos()
    ));
    std::fs::create_dir_all(&d).unwrap();
    d
}

fn app_with_updater(pubkey: &str) -> tauri::App<tauri::test::MockRuntime> {
    let mut ctx = mock_context(noop_assets());
    // tauri.conf.json'daki gibi: anahtar config'de boş, kabuk derleme
    // zamanındaki anahtarı Builder::pubkey ile verir.
    ctx.config_mut().plugins.0.insert(
        "updater".into(),
        serde_json::json!({ "pubkey": "", "endpoints": [] }),
    );
    mock_builder()
        .plugin(
            tauri_plugin_updater::Builder::new()
                .pubkey(pubkey.to_string())
                .build(),
        )
        .build(ctx)
        .unwrap()
}

#[tokio::test(flavor = "multi_thread")]
async fn old_version_updates_to_new_version() {
    let keys = keys();
    let new_bundle = elf_bytes("false");
    let old_app = elf_bytes("true");
    assert_ne!(new_bundle, old_app);
    let sig = sign(&keys, &new_bundle, "0.2.0");

    let base = serve_with_manifest(&new_bundle, &sig, "0.2.0");

    let app = app_with_updater(&keys.pk_b64);
    let cfg = Config::resolve(Some(&keys.pk_b64), Some(&base), false).unwrap();
    let dir = install_dir();
    let exe = dir.join("hakan.AppImage");
    std::fs::write(&exe, &old_app).unwrap();
    // Kurulu AppImage gibi çalıştırılabilir; kurulum izinleri korur.
    std::fs::set_permissions(&exe, std::fs::Permissions::from_mode(0o755)).unwrap();
    assert_eq!(
        std::process::Command::new(&exe).status().unwrap().code(),
        Some(0),
        "eski sürüm (true) çalışıyor"
    );

    let u = updater::updater_for(app.handle(), &cfg, Channel::Stable, Some(&exe)).unwrap();
    let update = u.check().await.unwrap().expect("0.1.0 → 0.2.0 görülmeli");
    assert_eq!(update.current_version, "0.1.0");
    assert_eq!(update.version, "0.2.0");
    assert_eq!(update.body.as_deref(), Some("Test sürümü"));

    let mut progress = Progress::default();
    let mut finished = false;
    update
        .download_and_install(|c, t| progress.add(c, t), || finished = true)
        .await
        .expect("imzalı paket kurulmalı");
    assert!(finished);
    assert_eq!(progress.downloaded, new_bundle.len() as u64);
    assert_eq!(progress.total, Some(new_bundle.len() as u64));
    assert_eq!(
        std::fs::read(&exe).unwrap(),
        new_bundle,
        "eski ikili yenisiyle değişmeli"
    );

    // Kurulan "yeni sürüm" çalışır durumda (false → çıkış kodu 1).
    let status = std::process::Command::new(&exe).status().unwrap();
    assert_eq!(status.code(), Some(1));

    // Aynı sürümde güncelleme görünmez.
    let same = serve_with_manifest(&new_bundle, &sig, "0.1.0");
    let cfg = Config::resolve(Some(&keys.pk_b64), Some(&same), false).unwrap();
    let u = updater::updater_for(app.handle(), &cfg, Channel::Stable, Some(&exe)).unwrap();
    assert!(u.check().await.unwrap().is_none());

    let _ = std::fs::remove_dir_all(dir);
}

#[tokio::test(flavor = "multi_thread")]
async fn wrong_key_is_rejected_and_app_untouched() {
    let trusted = keys();
    let attacker = keys();
    let bundle = elf_bytes("false");
    let old_app = elf_bytes("true");
    let sig = sign(&attacker, &bundle, "0.2.0");
    let base = serve_with_manifest(&bundle, &sig, "0.2.0");

    let app = app_with_updater(&trusted.pk_b64);
    let cfg = Config::resolve(Some(&trusted.pk_b64), Some(&base), false).unwrap();
    let dir = install_dir();
    let exe = dir.join("hakan.AppImage");
    std::fs::write(&exe, &old_app).unwrap();

    let u = updater::updater_for(app.handle(), &cfg, Channel::Stable, Some(&exe)).unwrap();
    let update = u
        .check()
        .await
        .unwrap()
        .expect("manifest yeni sürüm bildiriyor");
    let err = update
        .download_and_install(|_, _| {}, || {})
        .await
        .expect_err("yanlış anahtar reddedilmeli");
    assert!(
        updater::user_error(&err.to_string()).contains("imza"),
        "hata: {err}"
    );
    assert_eq!(
        std::fs::read(&exe).unwrap(),
        old_app,
        "kurulu ikili değişmemeli"
    );
    let _ = std::fs::remove_dir_all(dir);
}

#[tokio::test(flavor = "multi_thread")]
async fn channel_selects_manifest() {
    let keys = keys();
    let bundle = elf_bytes("false");
    let sig = sign(&keys, &bundle, "0.3.0-beta.1");
    // Yalnızca beta kanalında manifest var.
    let (listener, base) = bind();
    serve(
        listener,
        vec![(
            "/beta/latest.json".into(),
            manifest(&base, "0.3.0-beta.1", &sig),
        )],
    );
    let app = app_with_updater(&keys.pk_b64);
    let cfg = Config::resolve(Some(&keys.pk_b64), Some(&base), false).unwrap();

    let stable = updater::updater_for(app.handle(), &cfg, Channel::Stable, None).unwrap();
    assert!(
        stable.check().await.is_err(),
        "kararlı kanalda manifest yok"
    );
    let beta = updater::updater_for(app.handle(), &cfg, Channel::Beta, None).unwrap();
    let u = beta
        .check()
        .await
        .unwrap()
        .expect("beta kanalında güncelleme");
    assert_eq!(u.version, "0.3.0-beta.1");
}

/// Manifest ve paketi aynı sunucudan sunar; manifestteki paket adresi
/// sunucunun kendi adresidir.
fn serve_with_manifest(bundle: &[u8], sig: &str, version: &str) -> String {
    let (listener, base) = bind();
    serve(
        listener,
        vec![
            (
                "/stable/latest.json".to_string(),
                manifest(&base, version, sig),
            ),
            (
                format!("/stable/hakan_{version}_amd64.AppImage"),
                bundle.to_vec(),
            ),
        ],
    );
    base
}
