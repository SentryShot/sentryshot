// SPDX-License-Identifier: GPL-2.0-or-later

#[cfg(test)]
mod test;

mod minify;
mod serve_content;
mod templater;

pub use minify::minify;
pub use serve_content::serve_mp4_content;
pub use templater::Templater;
