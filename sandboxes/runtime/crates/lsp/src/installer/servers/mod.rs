use super::methods::{InstallMethod, InstallableServer};

mod data_a;
mod data_b;
mod data_c;

/// Returns all installable LSP servers
pub fn installable_servers() -> Vec<InstallableServer> {
    let mut servers = Vec::new();
    data_a::append(&mut servers);
    data_b::append(&mut servers);
    data_c::append(&mut servers);
    servers
}

/// Construct an `InstallableServer` with a single-entry binaries list.
pub(super) fn entry(
    id: &str,
    method: InstallMethod,
    binary: &str,
    description: &str,
) -> InstallableServer {
    InstallableServer {
        id: id.to_string(),
        method,
        binaries: vec![binary.to_string()],
        description: description.to_string(),
    }
}

pub(super) fn npm(package: &str) -> InstallMethod {
    InstallMethod::Npm {
        package: package.to_string(),
    }
}

pub(super) fn cargo_crate(crate_name: &str) -> InstallMethod {
    InstallMethod::Cargo {
        crate_name: crate_name.to_string(),
    }
}

pub(super) fn go_path(path: &str) -> InstallMethod {
    InstallMethod::Go {
        path: path.to_string(),
    }
}

pub(super) fn pip(package: &str) -> InstallMethod {
    InstallMethod::Pip {
        package: package.to_string(),
    }
}

pub(super) fn bash_cmd(script: &str) -> InstallMethod {
    InstallMethod::Custom {
        command: "bash".to_string(),
        args: vec!["-c".to_string(), script.to_string()],
    }
}
