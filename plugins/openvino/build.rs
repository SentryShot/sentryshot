fn main() -> Result<(), Box<dyn std::error::Error>> {
    tonic_prost_build::configure().compile_protos(
        &["proto/tensorflow_serving/apis/prediction_service.proto"],
        &["proto/"],
    )?;
    Ok(())
}