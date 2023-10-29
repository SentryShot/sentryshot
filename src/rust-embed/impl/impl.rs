#![recursion_limit = "1024"]
#![forbid(unsafe_code)]
#[macro_use]
extern crate quote;
extern crate proc_macro;

use proc_macro::TokenStream;
use proc_macro2::TokenStream as TokenStream2;
use std::{env, path::Path};
use syn::{Data, DeriveInput, Fields, Lit, Meta, MetaNameValue};

fn generate_assets(ident: &syn::Ident, folder_path: String) -> TokenStream2 {
    extern crate rust_embed_utils;

    let mut map_values = Vec::<TokenStream2>::new();
    let mut list_values = Vec::<String>::new();

    for rust_embed_utils::FileEntry {
        relative_path,
        full_path,
    } in rust_embed_utils::get_files(folder_path)
    {
        map_values.push(quote! {
            (#relative_path.to_owned(), std::borrow::Cow::from(&include_bytes!(#full_path)[..])),
        });

        list_values.push(relative_path);
    }

    quote! {
        impl #ident {
            /// Load the embedded files into a `HashMap`.
            pub fn load() -> std::collections::HashMap<String, rust_embed::EmbeddedFile> {
                std::collections::HashMap::from([
                    #(#map_values)*
                ])
            }
        }

        impl rust_embed::RustEmbed for #ident {
          fn load() -> std::collections::HashMap<String, rust_embed::EmbeddedFile> {
            #ident::load()
          }
        }
    }
}

/// Find all pairs of the `name = "value"` attribute from the derive input
fn find_attribute_values(ast: &syn::DeriveInput, attr_name: &str) -> Vec<String> {
    ast.attrs
        .iter()
        .filter(|value| value.path.is_ident(attr_name))
        .filter_map(|attr| attr.parse_meta().ok())
        .filter_map(|meta| match meta {
            Meta::NameValue(MetaNameValue {
                lit: Lit::Str(val), ..
            }) => Some(val.value()),
            _ => None,
        })
        .collect()
}

fn impl_rust_embed(ast: &syn::DeriveInput) -> TokenStream2 {
    match ast.data {
        Data::Struct(ref data) => match data.fields {
            Fields::Unit => {}
            _ => panic!("RustEmbed can only be derived for unit structs"),
        },
        _ => panic!("RustEmbed can only be derived for unit structs"),
    };

    let mut folder_paths = find_attribute_values(ast, "folder");
    if folder_paths.len() != 1 {
        panic!("#[derive(RustEmbed)] must contain one attribute like this #[folder = \"public/\"]");
    }
    let folder_path = folder_paths.remove(0);

    // Base relative paths on the Cargo.toml location
    let folder_path = if Path::new(&folder_path).is_relative() {
        Path::new(&env::var("CARGO_MANIFEST_DIR").unwrap())
            .join(folder_path)
            .to_str()
            .unwrap()
            .to_owned()
    } else {
        folder_path
    };

    if !Path::new(&folder_path).exists() {
        let message = format!(
            "#[derive(RustEmbed)] folder '{}' does not exist. cwd: '{}'",
            folder_path,
            std::env::current_dir().unwrap().to_str().unwrap()
        );
        panic!("{}", message);
    };

    generate_assets(&ast.ident, folder_path)
}

#[proc_macro_derive(RustEmbed, attributes(folder))]
pub fn derive_input_object(input: TokenStream) -> TokenStream {
    let ast: DeriveInput = syn::parse(input).unwrap();
    let gen = impl_rust_embed(&ast);
    gen.into()
}
