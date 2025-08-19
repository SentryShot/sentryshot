use std::path::PathBuf;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let proto_files: Vec<PathBuf> = glob::glob("proto/**/*.proto")?
        .filter_map(Result::ok)
        .collect();

    tonic_prost_build::configure()
        .compile_protos(&proto_files, &[PathBuf::from("proto/")])?;
    Ok(())
}