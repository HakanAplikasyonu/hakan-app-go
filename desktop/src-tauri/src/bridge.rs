//! Go sidecar ile satır tabanlı JSON protokolünün Rust tarafı
//! (`desktop/sidecar/rpc`). Tauri'den bağımsızdır: yazma işlevi dışarıdan
//! verilir, okunan satırlar `on_line` ile beslenir. Böylece aynı kod hem
//! Tauri sidecar'ı hem testteki gerçek alt süreç ile çalışır.

use std::collections::HashMap;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Mutex;
use std::time::Duration;

use serde::{Deserialize, Serialize};
use serde_json::Value;
use tokio::sync::oneshot;

/// Sidecar'ın ya da köprünün döndürdüğü hata. `code` Go tarafındaki
/// `rpc.Code*` sabitleriyle aynıdır; köprü kendi kodlarını da ekler.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct RpcError {
    pub code: String,
    pub message: String,
}

impl RpcError {
    pub fn new(code: &str, message: impl Into<String>) -> Self {
        Self {
            code: code.to_string(),
            message: message.into(),
        }
    }
}

impl std::fmt::Display for RpcError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}: {}", self.code, self.message)
    }
}

/// Sidecar'ın kendiliğinden yolladığı olay (ör. `ready`).
#[derive(Debug, Clone, Serialize, PartialEq)]
pub struct Event {
    pub event: String,
    pub data: Value,
}

#[derive(Deserialize)]
struct Incoming {
    #[serde(default)]
    id: u64,
    result: Option<Value>,
    error: Option<RpcError>,
    event: Option<String>,
    #[serde(default)]
    data: Value,
}

type Pending = oneshot::Sender<Result<Value, RpcError>>;
type Writer = Box<dyn Fn(Vec<u8>) -> Result<(), String> + Send + Sync>;

pub struct Bridge {
    next_id: AtomicU64,
    pending: Mutex<HashMap<u64, Pending>>,
    writer: Writer,
    alive: AtomicBool,
}

impl Bridge {
    pub fn new(writer: Writer) -> Self {
        Self {
            next_id: AtomicU64::new(1),
            pending: Mutex::new(HashMap::new()),
            writer,
            alive: AtomicBool::new(true),
        }
    }

    pub fn is_alive(&self) -> bool {
        self.alive.load(Ordering::SeqCst)
    }

    /// Bir yöntemi çağırır ve yanıtı `timeout` süresince bekler.
    pub async fn call(
        &self,
        method: &str,
        params: Value,
        timeout: Duration,
    ) -> Result<Value, RpcError> {
        if !self.is_alive() {
            return Err(RpcError::new("SIDECAR_DOWN", "sidecar çalışmıyor"));
        }
        let id = self.next_id.fetch_add(1, Ordering::SeqCst);
        let (tx, rx) = oneshot::channel();
        self.pending.lock().unwrap().insert(id, tx);

        let mut line = serde_json::to_vec(
            &serde_json::json!({ "id": id, "method": method, "params": params }),
        )
        .map_err(|e| RpcError::new("ENCODE_ERROR", e.to_string()))?;
        line.push(b'\n');
        if let Err(e) = (self.writer)(line) {
            self.pending.lock().unwrap().remove(&id);
            return Err(RpcError::new(
                "SIDECAR_DOWN",
                format!("sidecar'a yazılamadı: {e}"),
            ));
        }

        match tokio::time::timeout(timeout, rx).await {
            Ok(Ok(res)) => res,
            Ok(Err(_)) => Err(RpcError::new(
                "SIDECAR_DOWN",
                "sidecar yanıt vermeden kapandı",
            )),
            Err(_) => {
                self.pending.lock().unwrap().remove(&id);
                Err(RpcError::new(
                    "TIMEOUT",
                    format!("{method}: {} sn içinde yanıt yok", timeout.as_secs()),
                ))
            }
        }
    }

    /// Sidecar'dan okunan bir satırı işler. Yanıtsa bekleyen çağrıyı
    /// tamamlar; olaysa döndürür.
    pub fn on_line(&self, line: &[u8]) -> Option<Event> {
        let line = trim(line);
        if line.is_empty() {
            return None;
        }
        let msg: Incoming = match serde_json::from_slice(line) {
            Ok(m) => m,
            Err(e) => {
                log::warn!("sidecar: çözülemeyen satır: {e}");
                return None;
            }
        };
        if let Some(event) = msg.event {
            return Some(Event {
                event,
                data: msg.data,
            });
        }
        let waiter = self.pending.lock().unwrap().remove(&msg.id);
        match waiter {
            Some(tx) => {
                let res = match msg.error {
                    Some(err) => Err(err),
                    None => Ok(msg.result.unwrap_or(Value::Null)),
                };
                let _ = tx.send(res);
            }
            None => log::warn!("sidecar: beklenmeyen yanıt id={}", msg.id),
        }
        None
    }

    /// Sidecar kapandı: bekleyen bütün çağrılar hata alır.
    pub fn on_exit(&self) {
        self.alive.store(false, Ordering::SeqCst);
        let pending: Vec<_> = self.pending.lock().unwrap().drain().collect();
        for (_, tx) in pending {
            let _ = tx.send(Err(RpcError::new("SIDECAR_DOWN", "sidecar kapandı")));
        }
    }
}

fn trim(mut b: &[u8]) -> &[u8] {
    while let [rest @ .., b'\n' | b'\r' | b' '] = b {
        b = rest;
    }
    b
}

/// Parçalı gelen stdout'u satırlara böler.
#[derive(Default)]
pub struct LineBuffer {
    buf: Vec<u8>,
}

impl LineBuffer {
    pub fn push(&mut self, chunk: &[u8]) -> Vec<Vec<u8>> {
        self.buf.extend_from_slice(chunk);
        let mut out = Vec::new();
        while let Some(pos) = self.buf.iter().position(|&c| c == b'\n') {
            let line: Vec<u8> = self.buf.drain(..=pos).collect();
            out.push(line);
        }
        out
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Arc;

    fn capture() -> (Bridge, Arc<Mutex<Vec<Vec<u8>>>>) {
        let sent = Arc::new(Mutex::new(Vec::new()));
        let s = sent.clone();
        let b = Bridge::new(Box::new(move |line| {
            s.lock().unwrap().push(line);
            Ok(())
        }));
        (b, sent)
    }

    #[tokio::test]
    async fn call_resolves_by_id() {
        let (b, sent) = capture();
        let b = Arc::new(b);
        let b2 = b.clone();
        let task = tokio::spawn(async move {
            b2.call("ping", serde_json::json!({}), Duration::from_secs(2))
                .await
        });
        // Yazılan isteği bekle, yanıtı ver.
        for _ in 0..100 {
            if !sent.lock().unwrap().is_empty() {
                break;
            }
            tokio::time::sleep(Duration::from_millis(5)).await;
        }
        let req: Value = serde_json::from_slice(&sent.lock().unwrap()[0]).unwrap();
        assert_eq!(req["method"], "ping");
        let id = req["id"].as_u64().unwrap();
        assert!(b
            .on_line(format!("{{\"id\":{id},\"result\":{{\"pong\":true}}}}\n").as_bytes())
            .is_none());
        assert_eq!(
            task.await.unwrap().unwrap(),
            serde_json::json!({"pong": true})
        );
    }

    #[tokio::test]
    async fn error_and_timeout_and_exit() {
        let (b, _) = capture();
        let err = b
            .call("x", Value::Null, Duration::from_millis(20))
            .await
            .unwrap_err();
        assert_eq!(err.code, "TIMEOUT");

        let b = Arc::new(b);
        let b2 = b.clone();
        let task =
            tokio::spawn(async move { b2.call("x", Value::Null, Duration::from_secs(2)).await });
        tokio::time::sleep(Duration::from_millis(20)).await;
        b.on_line(br#"{"id":2,"error":{"code":"NOT_UBL","message":"m"}}"#);
        assert_eq!(task.await.unwrap().unwrap_err().code, "NOT_UBL");

        let b3 = b.clone();
        let task =
            tokio::spawn(async move { b3.call("x", Value::Null, Duration::from_secs(2)).await });
        tokio::time::sleep(Duration::from_millis(20)).await;
        b.on_exit();
        assert_eq!(task.await.unwrap().unwrap_err().code, "SIDECAR_DOWN");
        assert_eq!(
            b.call("x", Value::Null, Duration::from_secs(1))
                .await
                .unwrap_err()
                .code,
            "SIDECAR_DOWN"
        );
    }

    #[test]
    fn events_and_lines() {
        let (b, _) = capture();
        let ev = b
            .on_line(br#"{"event":"ready","data":{"version":"1"}}"#)
            .unwrap();
        assert_eq!(ev.event, "ready");
        assert_eq!(ev.data["version"], "1");
        assert!(b.on_line(b"not json").is_none());

        let mut lb = LineBuffer::default();
        assert!(lb.push(b"{\"a\":").is_empty());
        let lines = lb.push(b"1}\n{\"b\":2}\n{\"c\"");
        assert_eq!(lines.len(), 2);
        assert_eq!(lb.push(b":3}\n").len(), 1);
    }
}
