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

use std::path::PathBuf;
use std::sync::Arc;

use adk_rust::prelude::Tool as AdkTool;
use domain::ToolSpec;

pub use bash::BashTool;
pub use edit::EditTool;
pub use operations::{
    BashError, BashExecOptions, BashExecResult, BashOperations, EditOperations, FsError,
    LocalBashOperations, LocalFsOperations, ReadOperations, WriteOperations,
};
pub use process_registry::ProcessRegistry;
pub use read::ReadTool;
pub use write::WriteTool;

pub struct ToolBuildContext {
    pub workspace_root: PathBuf,
    pub fs: Arc<LocalFsOperations>,
    pub bash: Arc<LocalBashOperations>,
    pub process_registry: Arc<ProcessRegistry>,
}

impl ToolBuildContext {
    pub fn new(workspace_root: PathBuf) -> Self {
        Self {
            workspace_root,
            fs: Arc::new(LocalFsOperations::default()),
            bash: Arc::new(LocalBashOperations::default()),
            process_registry: Arc::new(ProcessRegistry::new()),
        }
    }
}

pub fn build_builtin_tools(
    specs: &[ToolSpec],
    context: &ToolBuildContext,
) -> Vec<Arc<dyn AdkTool>> {
    let mut tools: Vec<Arc<dyn AdkTool>> = Vec::new();
    for spec in specs {
        match spec {
            ToolSpec::Bash(config) => {
                tools.push(BashTool::new(
                    config.clone(),
                    context.workspace_root.clone(),
                    context.bash.clone(),
                )
                .with_process_registry(context.process_registry.clone())
                .into_adk_tool());
            }
            ToolSpec::ReadFile(config) => {
                tools.push(ReadTool::new(
                    config.clone(),
                    context.workspace_root.clone(),
                    context.fs.clone(),
                ).into_adk_tool());
            }
            ToolSpec::WriteFile(config) => {
                tools.push(WriteTool::new(
                    config.clone(),
                    context.workspace_root.clone(),
                    context.fs.clone(),
                ).into_adk_tool());
                tools.push(EditTool::new(
                    config.clone(),
                    context.workspace_root.clone(),
                    context.fs.clone(),
                ).into_adk_tool());
            }
            ToolSpec::PostStatusUpdate
            | ToolSpec::PostToChannel
            | ToolSpec::Cron
            | ToolSpec::Delegate
            | ToolSpec::CheckDelegatedStatus
            |             ToolSpec::CheckBashStatus
            | ToolSpec::Wake => {}
        }
    }
    tools
}
