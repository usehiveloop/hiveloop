use api::ApiDoc;
use utoipa::OpenApi;

fn main() {
    let spec = ApiDoc::openapi()
        .to_pretty_json()
        .expect("failed to serialize OpenAPI spec");

    let path = std::env::args()
        .nth(1)
        .unwrap_or_else(|| "openapi.json".to_string());
    std::fs::write(&path, spec).expect("failed to write openapi.json");

    eprintln!("Wrote OpenAPI spec to {path}");
}
