use std::pin::Pin;
use std::sync::Arc;

use adk_rust::prelude::*;
use domain::{event_types, OutboundEvent, SessionId, ToolSpec};
use gateway::ChannelGateway;
use outbound::OutboundEmitter;
use mcp::McpRegistry;
use storage::CronJobRepo;
use tools::ProcessRegistry;

use crate::check_bash_status_tool::CheckBashStatusTool;
use crate::check_delegated_status_tool::CheckDelegatedStatusTool;
use crate::cron_tool::CronTool;
use crate::delegate_tool::{DelegateContext, DelegateTool};
use crate::load_tools_tool::LoadToolsTool;
use crate::post_to_channel_tool::PostToChannelTool;
use crate::status_update_tool::PostStatusUpdateTool;
use crate::wake_tool::WakeTool;

pub fn attach_tool_event_callbacks(
    builder: LlmAgentBuilder,
    emitter: Arc<OutboundEmitter>,
    session_id: SessionId,
) -> LlmAgentBuilder {
    let after_emitter = emitter.clone();
    let after_session_id = session_id.clone();
    let on_error_emitter = emitter;
    let on_error_session_id = session_id;
    builder
        .after_tool_callback_full(Box::new(
            move |_ctx,
                  tool: Arc<dyn Tool>,
                  args: serde_json::Value,
                  response: serde_json::Value| {
                let emitter = after_emitter.clone();
                let session_id = after_session_id.clone();
                let tool_name = tool.name().to_string();
                Box::pin(async move {
                    let summary: String = response
                        .to_string()
                        .chars()
                        .take(200)
                        .collect();
                    emitter
                        .emit(OutboundEvent::new(
                            event_types::TOOL_INVOKED,
                            serde_json::json!({
                                "session_id": session_id.as_str(),
                                "tool": tool_name,
                                "args": args,
                                "result_summary": summary,
                            }),
                        ))
                        .await;
                    Ok(None)
                })
                    as Pin<Box<dyn std::future::Future<Output = adk_rust::Result<Option<serde_json::Value>>> + Send>>
            },
        ))
        .on_tool_error(Box::new(
            move |_ctx, tool: Arc<dyn Tool>, args: serde_json::Value, error: String| {
                let emitter = on_error_emitter.clone();
                let session_id = on_error_session_id.clone();
                let tool_name = tool.name().to_string();
                Box::pin(async move {
                    emitter
                        .emit(OutboundEvent::new(
                            event_types::ERROR_TOOL,
                            serde_json::json!({
                                "session_id": session_id.as_str(),
                                "tool": tool_name,
                                "args": args,
                                "error": error,
                            }),
                        ))
                        .await;
                    Ok(None)
                })
                    as Pin<Box<dyn std::future::Future<Output = adk_rust::Result<Option<serde_json::Value>>> + Send>>
            },
        ))
}

pub struct ToolContext {
    pub gateway: Option<Arc<dyn ChannelGateway>>,
    pub cron_repo: Option<Arc<dyn CronJobRepo>>,
    pub delegate_ctx: Option<Arc<DelegateContext>>,
    pub process_registry: Option<Arc<ProcessRegistry>>,
    pub mcp_registry: Option<Arc<McpRegistry>>,
}

pub fn build_agent_tools(
    specs: &[ToolSpec],
    session_id: &SessionId,
    ctx: &ToolContext,
) -> Vec<Arc<dyn Tool>> {
    let mut tools: Vec<Arc<dyn Tool>> = Vec::new();
    let session_is_cron = is_cron_session(session_id);

    for spec in specs {
        match spec {
            ToolSpec::PostStatusUpdate => {
                if let Some(gateway) = &ctx.gateway {
                    if session_is_cron {
                        tools.push(
                            PostToChannelTool::new(gateway.clone(), derive_channel(session_id))
                                .into_adk_tool(),
                        );
                    } else {
                        tools.push(
                            PostStatusUpdateTool::new(gateway.clone(), session_id.clone())
                                .into_adk_tool(),
                        );
                    }
                }
            }
            ToolSpec::PostToChannel => {
                if let Some(gateway) = &ctx.gateway {
                    if session_is_cron {
                        tools.push(
                            PostToChannelTool::new(gateway.clone(), derive_channel(session_id))
                                .into_adk_tool(),
                        );
                    }
                }
            }
            ToolSpec::Cron => {
                if let Some(cron_repo) = &ctx.cron_repo {
                    if !session_is_cron {
                        tools.push(
                            CronTool::new(cron_repo.clone(), session_id.clone()).into_adk_tool(),
                        );
                    }
                }
            }
            ToolSpec::Delegate => {
                if let (Some(cron_repo), Some(delegate_ctx)) =
                    (&ctx.cron_repo, &ctx.delegate_ctx)
                {
                    if !session_is_cron {
                        tools.push(
                            DelegateTool::new(
                                delegate_ctx.clone(),
                                session_id.clone(),
                                cron_repo.clone(),
                            )
                            .into_adk_tool(),
                        );
                    }
                }
            }
            ToolSpec::CheckDelegatedStatus => {
                if let (Some(cron_repo), Some(delegate_ctx)) =
                    (&ctx.cron_repo, &ctx.delegate_ctx)
                {
                    if !session_is_cron {
                        tools.push(
                            CheckDelegatedStatusTool::new(
                                cron_repo.clone(),
                                delegate_ctx.session_service.clone(),
                            )
                            .into_adk_tool(),
                        );
                    }
                }
            }
            ToolSpec::CheckBashStatus => {
                if let Some(registry) = &ctx.process_registry {
                    tools.push(CheckBashStatusTool::new(registry.clone()).into_adk_tool());
                }
            }
            ToolSpec::Wake => {
                if let Some(cron_repo) = &ctx.cron_repo {
                    if !session_is_cron {
                        tools.push(
                            WakeTool::new(cron_repo.clone(), session_id.clone()).into_adk_tool(),
                        );
                    }
                }
            }
            ToolSpec::LoadTools => {
                if let Some(registry) = &ctx.mcp_registry {
                    tools.push(LoadToolsTool::new(registry.clone()).into_adk_tool());
                }
            }
            _ => {}
        }
    }
    tools
}

fn is_cron_session(session_id: &SessionId) -> bool {
    session_id.as_str().contains("-cron-")
}

fn derive_channel(session_id: &SessionId) -> String {
    session_id
        .as_str()
        .split_once('-')
        .map(|(c, _)| c.to_string())
        .unwrap_or_else(|| session_id.as_str().to_string())
}
