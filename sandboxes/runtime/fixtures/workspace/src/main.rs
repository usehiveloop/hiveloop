fn main() {
    let config = Config::default();
    println!("Hello from workspace fixture: {}", config.name);
}

struct Config {
    name: String,
    debug: bool,
}

impl Default for Config {
    fn default() -> Self {
        Self {
            name: "fixture".to_string(),
            debug: false,
        }
    }
}
