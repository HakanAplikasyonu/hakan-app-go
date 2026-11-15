//! Kabuk köprüsü (`bridge`) ile gerçek Go sidecar ikilisi arasında uçtan uca
//! konuşma. İkili `scripts/build-sidecar.mjs` ile `binaries/` altına derlenir.
//! Bulunamazsa test atlanır; CI'da HAKAN_SIDECAR_REQUIRED=1 ile zorunludur.

use std::path::PathBuf;
use std::process::Stdio;
use std::sync::{Arc, Mutex};
use std::time::Duration;

use hakan_desktop_lib::bridge::{Bridge, LineBuffer};
use serde_json::json;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::process::Command;
use tokio::sync::mpsc;

fn sidecar_bin() -> Option<PathBuf> {
    if let Ok(p) = std::env::var("HAKAN_SIDECAR_BIN") {
        return Some(PathBuf::from(p));
    }
    let dir = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("binaries");
    std::fs::read_dir(dir)
        .ok()?
        .filter_map(Result::ok)
        .map(|e| e.path())
        .find(|p| {
            p.file_name()
                .is_some_and(|n| n.to_string_lossy().starts_with("hakan-sidecar-"))
        })
}

#[tokio::test(flavor = "multi_thread")]
async fn shell_talks_to_go_sidecar() {
    let Some(bin) = sidecar_bin() else {
        assert!(
            std::env::var("HAKAN_SIDECAR_REQUIRED").is_err(),
            "sidecar ikilisi yok (npm run sidecar)"
        );
        eprintln!("sidecar ikilisi yok; test atlandı");
        return;
    };

    let data_dir = std::env::temp_dir().join(format!("hakan-sidecar-test-{}", std::process::id()));
    let mut child = Command::new(&bin)
        .arg("--data-dir")
        .arg(&data_dir)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::inherit())
        .kill_on_drop(true)
        .spawn()
        .expect("sidecar başlatılamadı");

    // Köprünün yazıcısı senkron; bir kanal üzerinden stdin'e aktarılır.
    let (tx, mut rx) = mpsc::unbounded_channel::<Vec<u8>>();
    let mut stdin = child.stdin.take().unwrap();
    tokio::spawn(async move {
        while let Some(line) = rx.recv().await {
            if stdin.write_all(&line).await.is_err() {
                break;
            }
        }
    });
    let bridge = Arc::new(Bridge::new(Box::new(move |line| {
        tx.send(line).map_err(|e| e.to_string())
    })));

    let events = Arc::new(Mutex::new(Vec::new()));
    let mut stdout = child.stdout.take().unwrap();
    let (b, ev) = (bridge.clone(), events.clone());
    let reader = tokio::spawn(async move {
        let mut lines = LineBuffer::default();
        let mut buf = [0u8; 8192];
        loop {
            let n = stdout.read(&mut buf).await.unwrap_or(0);
            if n == 0 {
                break;
            }
            for line in lines.push(&buf[..n]) {
                if let Some(e) = b.on_line(&line) {
                    ev.lock().unwrap().push(e);
                }
            }
        }
        b.on_exit();
    });

    let t = Duration::from_secs(10);
    let pong = bridge
        .call("ping", json!({"echo": "merhaba"}), t)
        .await
        .unwrap();
    assert_eq!(pong["pong"], true);
    assert_eq!(pong["echo"], "merhaba");

    let version = bridge.call("version", json!({}), t).await.unwrap();
    assert_eq!(version["os"], std::env::consts::OS);

    // Backend'in demo UBL'i, sidecar'daki go-ubltr ile.
    let xml = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../backend/internal/demo/ubl/atlas-01-temel-satis.xml");
    let sum = bridge
        .call("ubl.inspect", json!({"path": xml}), t)
        .await
        .unwrap();
    assert!(sum["uuid"].as_str().is_some_and(|s| !s.is_empty()), "{sum}");

    // Yerel ajan: oturum yok, makine kimliği üretildi ve durum dosyası yazıldı.
    let st = bridge.call("agent.status", json!({}), t).await.unwrap();
    assert_eq!(st["signed_in"], false);
    assert!(
        st["machine_id"]
            .as_str()
            .is_some_and(|m| m.starts_with("hk-")),
        "{st}"
    );
    assert!(data_dir.join("agent-state.json").exists());
    let err = bridge
        .call("auth.resume", json!({"refresh_token": ""}), t)
        .await
        .unwrap_err();
    assert_eq!(err.code, "SIGNED_OUT");

    let err = bridge.call("yok.boyle", json!({}), t).await.unwrap_err();
    assert_eq!(err.code, "UNKNOWN_METHOD");

    // Eşzamanlı çağrılar id ile eşlenir.
    let calls: Vec<_> = (0..20)
        .map(|i| {
            let b = bridge.clone();
            tokio::spawn(async move { b.call("ping", json!({"echo": i.to_string()}), t).await })
        })
        .collect();
    for (i, c) in calls.into_iter().enumerate() {
        assert_eq!(c.await.unwrap().unwrap()["echo"], i.to_string());
    }

    let ready = events.lock().unwrap().iter().any(|e| e.event == "ready");
    assert!(ready, "ready olayı gelmedi");

    // stdin kapanınca sidecar çıkar; bekleyen çağrı kalmaz.
    drop(bridge);
    child.kill().await.ok();
    let _ = tokio::time::timeout(Duration::from_secs(5), reader).await;
    let _ = std::fs::remove_dir_all(&data_dir);
}
