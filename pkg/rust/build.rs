use std::env;
use std::path::PathBuf;

fn main() {
    // Tell cargo to look for shared libraries in the C lib directory
    let dir = env::var("CARGO_MANIFEST_DIR").unwrap();
    let c_lib_path = PathBuf::from(&dir).parent().unwrap().join("c/lib");
    
    println!("cargo:rustc-link-search=native={}", c_lib_path.display());
    println!("cargo:rustc-link-lib=luxconsensus");
    
    // On macOS, we need to set rpath
    if cfg!(target_os = "macos") {
        println!("cargo:rustc-link-arg=-Wl,-rpath,{}", c_lib_path.display());
    }
    
    // Rerun if the C library changes
    println!("cargo:rerun-if-changed=../c/lib/libluxconsensus.a");
    println!("cargo:rerun-if-changed=../c/lib/libluxconsensus.dylib");
    println!("cargo:rerun-if-changed=../c/lib/libluxconsensus.so");
}