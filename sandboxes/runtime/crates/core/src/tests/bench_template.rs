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
        if let Some(hs) = &def.config.history_strip {
            eprintln!(
                "  history_strip: enabled={}, age_threshold={}, pin_recent_count={}, pin_errors={}",
                hs.enabled, hs.age_threshold, hs.pin_recent_count, hs.pin_errors
            );
        }
        if let Some(im) = &def.config.immortal {
            eprintln!(
                "  immortal: token_budget={}, expose_journal_tools={}",
                im.token_budget, im.expose_journal_tools
            );
        }
        eprintln!("permissions keys: {:?}", def.permissions.keys().collect::<Vec<_>>());
        eprintln!("system_reminder_refresh_turns: {:?}", def.config.system_reminder_refresh_turns);
        assert!(def.config.immortal.is_some(), "expected immortal");
    }
}
