use super::*;

#[test]
fn test_installable_servers_list() {
    let servers = installable_servers();
    assert!(!servers.is_empty(), "should have installable servers");

    // Check that popular servers are included
    let ids: Vec<&str> = servers.iter().map(|s| s.id.as_str()).collect();
    assert!(ids.contains(&"typescript"), "should include typescript");
    assert!(ids.contains(&"rust"), "should include rust");
    assert!(ids.contains(&"go"), "should include go");
    assert!(ids.contains(&"python"), "should include python");
}

#[test]
fn test_installer_new() {
    let installer = LspInstaller::new();
    let available = installer.available_servers();
    assert!(!available.is_empty(), "should have available servers");
    assert!(available.contains(&"typescript".to_string()));
}

#[test]
fn test_server_methods() {
    let servers = installable_servers();

    // Check various install methods are represented
    let has_npm = servers
        .iter()
        .any(|s| matches!(s.method, InstallMethod::Npm { .. }));
    let has_cargo = servers
        .iter()
        .any(|s| matches!(s.method, InstallMethod::Cargo { .. }));
    let has_go = servers
        .iter()
        .any(|s| matches!(s.method, InstallMethod::Go { .. }));

    assert!(has_npm, "should have npm-based servers");
    assert!(has_cargo, "should have cargo-based servers");
    assert!(has_go, "should have go-based servers");
}
