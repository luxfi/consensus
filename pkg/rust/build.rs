use std::env;
use std::path::PathBuf;

fn main() {
    // docs.rs builds: skip everything below.
    if env::var("DOCS_RS").is_ok() {
        return;
    }

    // This crate is pure Rust (no `extern "C"` symbols are referenced from
    // src/lib.rs). The historical build.rs linked against a sibling C
    // library (`libluxconsensus`) that is built alongside in the upstream
    // repo, but no Rust code consumes those symbols.
    //
    // To keep in-tree development behaviour unchanged AND let downstream
    // consumers (and crates.io) build without the C library on disk, we
    // emit the link directives *only when the static or dynamic library
    // actually exists on the filesystem*.
    let dir = env::var("CARGO_MANIFEST_DIR").unwrap();
    let c_lib_path = PathBuf::from(&dir).parent().unwrap().join("c/lib");

    let candidates = [
        c_lib_path.join("libluxconsensus.a"),
        c_lib_path.join("libluxconsensus.dylib"),
        c_lib_path.join("libluxconsensus.so"),
    ];

    let any_exists = candidates.iter().any(|p| p.exists());
    if any_exists {
        println!("cargo:rustc-link-search=native={}", c_lib_path.display());
        println!("cargo:rustc-link-lib=luxconsensus");

        if cfg!(target_os = "macos") {
            println!("cargo:rustc-link-arg=-Wl,-rpath,{}", c_lib_path.display());
        }
    }

    // Rerun if the C library changes (or appears/disappears).
    for c in &candidates {
        println!("cargo:rerun-if-changed={}", c.display());
    }
    println!("cargo:rerun-if-env-changed=DOCS_RS");
}
