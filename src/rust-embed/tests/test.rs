use rust_embed::RustEmbed;

/// Test doc comment
#[derive(RustEmbed)]
#[folder = "public/"]
struct Asset;

#[test]
fn get_works() {
    let files = Asset::load();
    assert!(files.get("index.html").is_some(), "index.html should exist");
    assert!(files.get("gg.html").is_none(), "gg.html should not exist");
    assert!(
        files.get("images/llama.png").is_some(),
        "llama.png should exist"
    );
}

#[test]
fn trait_works_generic() {
    trait_works_generic_helper::<Asset>();
}
fn trait_works_generic_helper<E: RustEmbed>() {
    let mut num_files = 0;
    let files = E::load();
    for file in files.keys() {
        assert!(files.get(file).is_some());
        num_files += 1;
    }
    assert_eq!(num_files, 5);
    assert!(files.get("gg.html").is_none(), "gg.html should not exist");
}

#[derive(RustEmbed)]
#[folder = "public/"]
struct Assets;

/// Prevent attempts to access files outside of the embedded folder.
#[test]
fn path_traversal_attack_fails() {
    assert!(Assets::load().get("../basic.rs").is_none());
}
