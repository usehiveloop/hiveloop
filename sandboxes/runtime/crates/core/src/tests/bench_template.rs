#[cfg(test)]
mod t {
    use crate::agent::AgentDefinition;
    #[test]
    fn bench_template_parses() {
        let json = std::fs::read_to_string(
            "/Users/bahdcoder/code/useportal.bridge/scripts/bench/agent.tmpl.json",
        )
        .unwrap();
        let json = json
            .replace("__SYSTEM_PROMPT__", "test")
            .replace("__MODEL__", "x/y")
            .replace("__BASE_URL__", "http://localhost/v1");
        let def: AgentDefinition = serde_json::from_str(&json).expect("parse");
        eprintln!("immortal present: {}", def.config.immortal.is_some());
        eprintln!(
            "history_strip present: {}",
            def.config.history_strip.is_some()
        );
        assert!(def.config.immortal.is_some(), "expected immortal");
    }
}
