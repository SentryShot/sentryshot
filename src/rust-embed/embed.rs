#![forbid(unsafe_code)]

pub use rust_embed_impl::RustEmbed;
pub use rust_embed_utils::{EmbeddedFile, EmbeddedFiles};

/// A directory of binary assets.
///
/// The files in the specified folder will be embedded into the executable in
/// release builds.
///
/// This trait is meant to be derived like so:
/// ```
/// use rust_embed::RustEmbed;
///
/// #[derive(RustEmbed)]
/// #[folder = "public/"]
/// struct Asset;
///
/// fn main() {}
/// ```
pub trait RustEmbed {
    /// Load the embedded files into a `HashMap`.
    fn load() -> EmbeddedFiles;
}
