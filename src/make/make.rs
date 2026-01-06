// SPDX-License-Identifier: GPL-2.0-or-later

#![allow(clippy::unwrap_used)]

use std::{
    fs::Permissions,
    os::unix::fs::PermissionsExt,
    path::{Path, PathBuf},
};
use xshell::{Shell, cmd};

const USAGE: &str = "Utilities
  Commands:
    help         # Display this help and exit.
    run <args>   # Build and run app.
    run run      # Example run command.
    run-release  # Run in release mode.
    build-target <TARGET> # Build target.
    test-backend  # Run backend tests.
    lint-backend  # Run backend linters.
    lint-frontend # Run frontend linters.
    ci           # Full CI suite without file changes.
    ci-fix       # Full CI suite.
    ci-frontend  # Frontend CI suite.
    ci-backend   # Backend CI suite.
    clean          # Clean build directories.
  Use 'npm run cover x' to run javascript tests
";

#[allow(clippy::too_many_lines)]
fn main() {
    let sh = Shell::new().unwrap();

    let target_dir = match sh.var("CARGO_TARGET_DIR") {
        Ok(v) => PathBuf::from(v),
        Err(_) => sh.current_dir().join("target2"),
    };
    sh.set_var("CARGO_TARGET_DIR", &target_dir);

    let mut args = std::env::args();
    args.next().unwrap();
    let Some(arg1) = args.next() else {
        println!("{USAGE}");
        return;
    };
    match arg1.as_str() {
        "run" => {
            let packages = packages();
            cmd!(sh, "cargo build {packages...}").run().unwrap();
            update_plugin_dir(&sh, &target_dir, "debug");
            _ = cmd!(sh, "{target_dir}/debug/sentryshot {args...}").run();
        }
        "run-release" => {
            let packages = packages();
            cmd!(sh, "cargo build --release {packages...}")
                .run()
                .unwrap();
            update_plugin_dir(&sh, &target_dir, "release");
            _ = cmd!(sh, "{target_dir}/release/sentryshot {args...}").run();
        }
        "build-target" => {
            let Some(target) = args.next() else {
                println!("missing target: 'x86_64', 'aarch64'");
                return;
            };
            build_target(&sh, &target_dir, &target);
        }
        "build-target-nix" => {
            let Some(target) = args.next() else {
                println!("missing target: 'x86_64', 'aarch64'");
                return;
            };
            cmd!(sh, "nix-shell --pure ./misc/nix/shell-{target}.nix --run \"./misc/utils.sh build-target {target}\"").run().unwrap();
        }
        "test-backend" | "test-back" | "test-be" | "test-b" | "testb" => test_backend(&sh),
        "lint-backend" | "lint-back" | "lint-be" | "lint-b" | "lintb" => lint_backend_fix(&sh),
        "lint-frontend" | "lint-front" | "lint-fe" | "lint-f" | "lintf" => {
            lint_css_fix(&sh);
            lint_js_fix(&sh);
        }
        "ci" => {
            lint_js(&sh);
            test_js(&sh);
            lint_css(&sh);
            format_backend(&sh);
            lint_backend(&sh);
            test_backend(&sh);
            println!("all passed!");
        }
        "ci-fix" => {
            format_frontend(&sh);
            lint_js_fix(&sh);
            test_js(&sh);
            lint_css_fix(&sh);
            format_backend_fix(&sh);
            lint_backend_fix(&sh);
            test_backend(&sh);
            println!("all passed!");
        }
        "ci-frontend" | "ci-front" | "ci-fe" | "ci-f" | "cif" => {
            format_frontend(&sh);
            lint_js_fix(&sh);
            test_js(&sh);
            lint_css_fix(&sh);
        }
        "ci-backend" | "ci-back" | "ci-be" | "ci-b" | "cib" => {
            format_backend(&sh);
            lint_backend(&sh);
            test_backend(&sh);
        }
        "clean" => {
            let clean_dir = |dir: &Path| {
                for entry in std::fs::read_dir(dir).unwrap() {
                    let entry = entry.unwrap().path();
                    if entry.is_file() {
                        std::fs::remove_file(entry).unwrap();
                    } else {
                        std::fs::remove_dir_all(entry).unwrap();
                    }
                }
            };
            clean_dir(Path::new("./target"));
            clean_dir(Path::new("./target2"));
            clean_dir(Path::new("./build"));
        }
        _ => println!("{USAGE}"),
    }
}

const PLUGINS: [&str; 7] = [
    "auth_basic",
    "auth_none",
    "motion",
    "mqtt",
    "object_detection",
    "object_detection_tflite",
    "thumb_scale",
];

// Returns ["-p", "auth_basic", "-p", "auth_none"]
fn packages() -> Vec<String> {
    let mut packages = vec!["-p".to_owned(), "sentryshot".to_owned()];
    for p in PLUGINS {
        packages.push("-p".to_owned());
        packages.push(p.to_owned());
    }
    packages
}

fn update_plugin_dir(sh: &Shell, target_dir: &Path, mode: &str) {
    let plugin_dir = PathBuf::from("./plugin_dir");
    if plugin_dir.exists() {
        std::fs::remove_file("./plugin_dir").unwrap();
    }
    cmd!(sh, "ln -sf {target_dir}/{mode} ./plugin_dir")
        .run()
        .unwrap();
}

fn build_target(sh: &Shell, target_dir: &Path, target: &str) {
    println!("build");
    let packages = packages();
    cmd!(
        sh,
        "cargo build --release --target={target}-unknown-linux-gnu {packages...}"
    )
    .run()
    .unwrap();

    let output_dir = PathBuf::from("./build").join(target);
    std::fs::create_dir_all(output_dir.join("plugins")).unwrap();

    println!("copy files");
    println!("./build/{target}/sentryshot");
    let release_dir = target_dir
        .join(format!("{target}-unknown-linux-gnu"))
        .join("release");
    let output = output_dir.join("sentryshot");
    std::fs::copy(
        release_dir.join("sentryshot"),
        output_dir.join("sentryshot"),
    )
    .unwrap();
    patch_rpath(sh, &output);
    patch_interpreter(sh, &output, target);

    // Copy plugins.
    for plugin in PLUGINS {
        println!("./build/{target}/plugins/libsentryshot_{plugin}.so");
        // Cargo doesn't let you specify the output file: https://github.com/rust-lang/cargo/issues/9778
        let binary = release_dir.join(format!("lib{plugin}.so"));
        let output = output_dir
            .join("plugins")
            .join(format!("libsentryshot_{plugin}.so"));
        std::fs::copy(binary, &output).unwrap();
        patch_rpath(sh, &output);
    }

    let get_env = |var: &str| PathBuf::from(sh.var(var).unwrap());
    let libs_dir = output_dir.join("libs");
    let copy_to_libs = |file: &Path| {
        sh.copy_file(file, &libs_dir).unwrap();
    };

    // Copy libs.
    std::fs::remove_dir_all(&libs_dir).unwrap();
    std::fs::create_dir_all(&libs_dir).unwrap();

    println!("./build/{target}/libs/libavutil.so");
    let fflibs = get_env("FFLIBS");
    copy_to_libs(&fflibs.join("libavutil.so.58"));
    println!("./build/{target}/libs/libavcodec.so");
    copy_to_libs(&fflibs.join("libavcodec.so.60"));

    println!("./build/{target}/libs/libtensorflowlite_c.so");
    copy_to_libs(&get_env("TFLITELIB").join("libtensorflowlite_c.so"));

    println!("./build/{target}/libs/libedgetpu.so.1");
    std::fs::copy(
        get_env("EDGETPULIB").join("libedgetpu.so.1.0"),
        output_dir.join("libs").join("libedgetpu.so.1"),
    )
    .unwrap();

    println!("./build/{target}/libs/libopenh264.so.6");
    copy_to_libs(&get_env("OPENH264LIB").join("libopenh264.so.6"));

    for f in std::fs::read_dir(&libs_dir).unwrap() {
        let file = f.unwrap().path();
        std::fs::set_permissions(&file, Permissions::from_mode(0o644)).unwrap();

        // Remove the nix runpath.
        cmd!(sh, "patchelf --remove-rpath {file}").run().unwrap();
    }
}

// Removes the nix interpreter prefix.
fn patch_interpreter(sh: &Shell, file: &Path, target: &str) {
    let interpreter = match target {
        "x86_64" => "/lib64/ld-linux-x86-64.so.2",
        "aarch64" => "/lib/ld-linux-aarch64.so.1",
        _ => todo!(),
    };
    cmd!(sh, "patchelf --set-interpreter {interpreter} {file}")
        .run()
        .unwrap();
}

fn patch_rpath(sh: &Shell, out: &Path) {
    cmd!(
        sh,
        "patchelf --set-rpath '$ORIGIN/libs:$ORIGIN/../libs' {out}"
    )
    .run()
    .unwrap();
}

fn format_frontend(sh: &Shell) {
    println!("format frontend");
    cmd!(sh, "npm run format").run().unwrap();
}

fn lint_js(sh: &Shell) {
    println!("lint js");
    cmd!(sh, "npm run lint-js").run().unwrap();
}

fn lint_js_fix(sh: &Shell) {
    println!("lint js");
    cmd!(sh, "npm run lint-js-fix").run().unwrap();
}

fn test_js(sh: &Shell) {
    println!("test js");
    cmd!(sh, "npm run test").run().unwrap();
}

fn lint_css(sh: &Shell) {
    println!("lint css");
    cmd!(sh, "npm run lint-css").run().unwrap();
}

fn lint_css_fix(sh: &Shell) {
    println!("lint css");
    cmd!(sh, "npm run lint-css-fix").run().unwrap();
}

fn format_backend(sh: &Shell) {
    println!("format backend");
    cmd!(sh, "cargo fmt --check --all").run().unwrap();
}

fn format_backend_fix(sh: &Shell) {
    println!("format backend");
    cmd!(sh, "cargo fmt --all").run().unwrap();
}

fn lint_backend(sh: &Shell) {
    println!("lint backend");
    cmd!(
        sh,
        "cargo clippy --workspace --no-deps --all-targets -- -Dwarnings"
    )
    .run()
    .unwrap();
    let files = String::from_utf8(cmd!(sh, "git ls-files").output().unwrap().stdout).unwrap();
    for file in files.lines() {
        if !std::path::Path::new(file)
            .extension()
            .is_some_and(|ext| ext.eq_ignore_ascii_case("sh"))
        {
            continue;
        }
        println!("{file}");
        cmd!(sh, "shellcheck {file}").run().unwrap();
    }
}

fn lint_backend_fix(sh: &Shell) {
    println!("lint backend");
    cmd!(
        sh,
        "cargo clippy --workspace --no-deps --all-targets --fix --allow-staged --allow-dirty"
    )
    .run()
    .unwrap();
    cmd!(
        sh,
        "cargo clippy --workspace --no-deps --all-targets -- -Dwarnings"
    )
    .run()
    .unwrap();
    let files = String::from_utf8(cmd!(sh, "git ls-files").output().unwrap().stdout).unwrap();
    for file in files.lines() {
        if !std::path::Path::new(file)
            .extension()
            .is_some_and(|ext| ext.eq_ignore_ascii_case("sh"))
        {
            continue;
        }
        println!("{file}");
        cmd!(sh, "shellcheck {file}").run().unwrap();
    }
}

fn test_backend(sh: &Shell) {
    println!("test_backend");
    cmd!(sh, "cargo test --workspace").run().unwrap();
}
