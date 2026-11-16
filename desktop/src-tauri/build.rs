fn main() {
    // Güncelleyici (#205): açık anahtar ve manifest kökü derleme zamanında
    // `option_env!` ile okunur; değişince yeniden derlensin.
    println!("cargo:rerun-if-env-changed=TAURI_UPDATER_PUBKEY");
    println!("cargo:rerun-if-env-changed=TAURI_UPDATER_BASE_URL");
    tauri_build::build()
}
