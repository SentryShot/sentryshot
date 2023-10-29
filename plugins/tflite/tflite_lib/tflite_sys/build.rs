use std::path::Path;

fn main() {
    println!("cargo:rerun-if-changed=src/wrapper.c");
    println!("cargo:rustc-link-lib=tensorflowlite_c");

    cc::Build::new()
        .file("src/wrapper.c")
        .include(Path::new("./src/includes"))
        .warnings_into_errors(true)
        .compile("wrapper");
}
