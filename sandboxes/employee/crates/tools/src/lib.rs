mod bash;
mod diff;
mod edit;
mod mutation_queue;
mod operations;
mod path;
mod process_registry;
mod read;
mod truncate;
mod write;

use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::Arc;

use async_trait::async_trait;
use domain::ToolSpec;
use serde_json::Value;

pub use bash::BashTool;
pub use edit::EditTool;
pub use operations::{
    BashError, BashExecOptions, BashExecResult, BashOperations, EditOperations, FsError,
    LocalBashOperations, LocalFsOperations, ReadOperations, WriteOperations,
};
pub use process_registry::ProcessRegistry;
pub use read::ReadTool;
pub use write::WriteTool;

#[derive(Debug, Clone)]
pub struct ToolDefinition {
    pub name: String,
    pub description: String,
    pub parameters: Value,
}

#[async_trait]
pub trait JsonTool: Send + Sync {
    fn definition(&self) -> ToolDefinition;
    async fn call(&self, args: Value) -> anyhow::Result<Value>;
}

pub fn schema_for<T: schemars::JsonSchema>() -> Value {
    serde_json::to_value(schemars::schema_for!(T))
        .unwrap_or_else(|_| serde_json::json!({"type":"object"}))
}

#[derive(Clone)]
pub struct ToolBuildContext {
    pub workspace_root: PathBuf,
    pub fs: Arc<LocalFsOperations>,
    pub bash: Arc<LocalBashOperations>,
    pub runtime_env: Arc<HashMap<String, String>>,
    pub process_registry: Arc<ProcessRegistry>,
}

impl ToolBuildContext {
    pub fn new(workspace_root: PathBuf) -> Self {
        Self {
            workspace_root,
            fs: Arc::new(LocalFsOperations::default()),
            bash: Arc::new(LocalBashOperations::default()),
            runtime_env: Arc::new(HashMap::new()),
            process_registry: Arc::new(ProcessRegistry::new()),
        }
    }
}

pub fn build_builtin_tools(
    specs: &[ToolSpec],
    context: &ToolBuildContext,
) -> Vec<Arc<dyn JsonTool>> {
    let mut tools: Vec<Arc<dyn JsonTool>> = Vec::new();
    for spec in specs {
        match spec {
            ToolSpec::Bash(config) => {
                tools.push(
                    BashTool::new(
                        config.clone(),
                        context.workspace_root.clone(),
                        context.bash.clone(),
                        context.runtime_env.clone(),
                    )
                    .with_process_registry(context.process_registry.clone())
                    .into_tool(),
                );
            }
            ToolSpec::ReadFile(config) => {
                tools.push(
                    ReadTool::new(
                        config.clone(),
                        context.workspace_root.clone(),
                        context.fs.clone(),
                    )
                    .into_tool(),
                );
            }
            ToolSpec::WriteFile(config) => {
                tools.push(
                    WriteTool::new(
                        config.clone(),
                        context.workspace_root.clone(),
                        context.fs.clone(),
                    )
                    .into_tool(),
                );
                tools.push(
                    EditTool::new(
                        config.clone(),
                        context.workspace_root.clone(),
                        context.fs.clone(),
                    )
                    .into_tool(),
                );
            }
            ToolSpec::PostStatusUpdate
            | ToolSpec::PostToChannel
            | ToolSpec::Cron
            | ToolSpec::Delegate
            | ToolSpec::CheckDelegatedStatus
            | ToolSpec::CheckBashStatus
            | ToolSpec::Wake
            | ToolSpec::LoadTools
            | ToolSpec::SkillsList
            | ToolSpec::SkillView
            | ToolSpec::SkillManage
            | ToolSpec::CloudAgentLaunchTask
            | ToolSpec::CloudAgentTaskStatus
            | ToolSpec::CloudAgentListTasks
            | ToolSpec::CloudAgentTaskSendMessage
            | ToolSpec::CloudAgentTaskTerminate => {}
        }
    }
    tools
}
