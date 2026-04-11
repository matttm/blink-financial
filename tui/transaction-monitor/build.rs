fn main() -> Result<(), Box<dyn std::error::Error>> {
    println!("cargo:rerun-if-changed=../../proto/blink/transactions/v1/transactions.proto");

    tonic_prost_build::configure().compile_protos(
        &["../../proto/blink/transactions/v1/transactions.proto"],
        &["../../proto"],
    )?;

    Ok(())
}
