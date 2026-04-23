use super::TestHarness;

impl TestHarness {
    /// Returns a reference to the HTTP client.
    pub fn client(&self) -> &reqwest::Client {
        &self.client
    }

    /// Returns the bridge base URL (e.g. "http://127.0.0.1:12345").
    pub fn bridge_url(&self) -> &str {
        &self.bridge_base_url
    }

    /// Returns the mock control plane base URL.
    pub fn cp_url(&self) -> &str {
        &self.cp_base_url
    }

    /// Returns the workspace root path.
    pub fn workspace_root(&self) -> &std::path::Path {
        &self.workspace_root
    }
}
