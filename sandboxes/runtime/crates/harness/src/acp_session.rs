//! Shared ACP-protocol driver used by every per-harness adapter.
//!
//! Holds a single long-running connection to the spawned ACP-agent
//! subprocess (claude-agent-acp, opencode acp, ...) and serializes
//! commands from the supervisor onto it. The protocol layer is fully
//! harness-agnostic; per-harness behaviour is supplied via the
//! [`HarnessAdapter`] trait — the builder for the `_meta` block on
//! `NewSessionRequest` and any human-readable harness name used in logs.

use crate::events;
use agent_client_protocol::schema::{
    CancelNotification, ContentBlock, InitializeRequest, LoadSessionRequest, NewSessionRequest,
    PromptRequest, ProtocolVersion, RequestPermissionOutcome, RequestPermissionRequest,
    RequestPermissionResponse, SelectedPermissionOutcome, SessionId, SessionNotification,
    TextContent,
};
use agent_client_protocol::{ByteStreams, Client, ConnectionTo, Responder};
use bridge_core::event::{BridgeEvent, BridgeEventType};
use bridge_core::mcp::McpServerDefinition;
use bridge_core::{AgentDefinition, BridgeError};
use dashmap::DashMap;
use serde_json::{json, Value};
use std::path::PathBuf;
use std::sync::Arc;
use tokio::process::{ChildStdin, ChildStdout};
use tokio::sync::{mpsc, oneshot, RwLock};
use tokio::task::JoinHandle;
use tokio_util::compat::{TokioAsyncReadCompatExt, TokioAsyncWriteCompatExt};
use tracing::{error, info};
use webhooks::{EventBus, PermissionManager};

/// Per-conversation context returned to the supervisor.
///
/// Event consumers attach via [`webhooks::EventBus::subscribe_sse`] using
/// the conversation id; the harness no longer hands out a single-shot
/// receiver because the SSE channel is now multi-subscriber.
pub struct ConversationContext {
    pub agent_id: String,
    pub conversation_id: String,
}

/// Per-harness behaviour the shared driver delegates to.
pub trait HarnessAdapter: Send + Sync + 'static {
    /// Short name shown in logs (`"claude"`, `"opencode"`).
    fn name(&self) -> &'static str;

    /// Build the `_meta` block to attach to `NewSessionRequest`. Each
    /// harness uses its own key under `_meta` (e.g. claude-agent-acp
    /// reads `_meta.claudeCode.options`). Return `None` to skip meta.
    fn build_session_meta(
        &self,
        agent: &AgentDefinition,
        api_key_override: Option<&str>,
        provider_override: Option<&bridge_core::ProviderConfig>,
    ) -> Option<serde_json::Map<String, Value>>;
}

struct SessionState {
    session_id: SessionId,
}

enum Cmd {
    NewSession {
        api_key_override: Option<String>,
        provider_override: Option<bridge_core::ProviderConfig>,
        per_conversation_mcp: Option<Vec<McpServerDefinition>>,
        reply: oneshot::Sender<Result<SessionId, String>>,
    },
    LoadSession {
        session_id: SessionId,
        reply: oneshot::Sender<Result<(), String>>,
    },
    Prompt {
        session_id: SessionId,
        text: String,
        reply: oneshot::Sender<Result<(), String>>,
    },
    Cancel {
        session_id: SessionId,
        reply: oneshot::Sender<Result<(), String>>,
    },
}

type AgentDefStore = Arc<RwLock<AgentDefinition>>;

/// Long-running ACP session pinned to one spawned agent process.
pub struct AcpSession {
    agent_id: String,
    agent_def: AgentDefStore,
    cmd_tx: mpsc::Sender<Cmd>,
    sessions: Arc<DashMap<String, SessionState>>,
    event_bus: Arc<EventBus>,
    _driver: JoinHandle<()>,
    /// JoinHandle for the subprocess watcher task. The child itself is
    /// owned (and `wait`ed on) inside the watcher task.
    _child_watcher: JoinHandle<()>,
}

impl AcpSession {
    /// Wire up the ACP connection over the given child stdio and start
    /// the command dispatcher task.
    #[allow(clippy::too_many_arguments)]
    pub async fn start(
        agent: AgentDefinition,
        cwd: PathBuf,
        stdin: ChildStdin,
        stdout: ChildStdout,
        mut child: tokio::process::Child,
        event_bus: Arc<EventBus>,
        permission_manager: Arc<PermissionManager>,
        adapter: Arc<dyn HarnessAdapter>,
    ) -> Result<Arc<Self>, BridgeError> {
        let (cmd_tx, mut cmd_rx) = mpsc::channel::<Cmd>(64);
        let sessions: Arc<DashMap<String, SessionState>> = Arc::new(DashMap::new());
        let agent_id = agent.id.clone();
        let agent_def: AgentDefStore = Arc::new(RwLock::new(agent));

        // Watch the spawned ACP-agent subprocess. If it exits unexpectedly,
        // log + Sentry-capture so the failure isn't silent. We can't restart
        // it from here without losing in-flight session state, so this is
        // an alarm channel only.
        let child_watcher_pid = child.id();
        let child_watcher_harness = adapter.name();
        let child_watcher_agent = agent_id.clone();
        let child_handle = tokio::spawn(async move {
            match child.wait().await {
                Ok(status) => {
                    let code = status.code();
                    let signal = {
                        #[cfg(unix)]
                        {
                            use std::os::unix::process::ExitStatusExt;
                            status.signal()
                        }
                        #[cfg(not(unix))]
                        {
                            None::<i32>
                        }
                    };
                    error!(
                        harness = child_watcher_harness,
                        agent_id = %child_watcher_agent,
                        pid = ?child_watcher_pid,
                        exit_code = ?code,
                        signal = ?signal,
                        "ACP agent subprocess exited — bridge can no longer drive new turns for this session"
                    );
                }
                Err(e) => {
                    error!(
                        harness = child_watcher_harness,
                        agent_id = %child_watcher_agent,
                        error = %e,
                        "ACP agent subprocess wait() failed"
                    );
                }
            }
        });

        let agent_id_for_notif = agent_id.clone();
        let agent_id_for_perm = agent_id.clone();
        let agent_id_for_prompt = agent_id.clone();
        let sessions_for_notif = sessions.clone();
        let sessions_for_perm = sessions.clone();
        let event_bus_for_notif = event_bus.clone();
        let event_bus_for_perm = event_bus.clone();
        let event_bus_for_prompt = event_bus.clone();
        let agent_def_for_driver = agent_def.clone();
        let adapter_for_driver = adapter.clone();
        let cwd_for_driver = cwd.clone();
        let harness_name = adapter.name();

        let transport = ByteStreams::new(stdin.compat_write(), stdout.compat());

        let driver = tokio::spawn(async move {
            let result = Client
                .builder()
                .name("bridge")
                .on_receive_notification(
                    move |notification: SessionNotification, _cx| {
                        let agent_id = agent_id_for_notif.clone();
                        let sessions = sessions_for_notif.clone();
                        let event_bus = event_bus_for_notif.clone();
                        async move {
                            handle_notification(&agent_id, &sessions, &event_bus, notification)
                                .await;
                            Ok(())
                        }
                    },
                    agent_client_protocol::on_receive_notification!(),
                )
                .on_receive_request(
                    move |req: RequestPermissionRequest,
                          responder: Responder<RequestPermissionResponse>,
                          _cx| {
                        let perm = permission_manager.clone();
                        let agent_id = agent_id_for_perm.clone();
                        let event_bus = event_bus_for_perm.clone();
                        let sessions = sessions_for_perm.clone();
                        async move {
                            handle_permission(perm, event_bus, &sessions, &agent_id, req, responder)
                                .await
                        }
                    },
                    agent_client_protocol::on_receive_request!(),
                )
                .connect_with(
                    transport,
                    move |cx: ConnectionTo<agent_client_protocol::Agent>| async move {
                        cx.send_request(InitializeRequest::new(ProtocolVersion::V1))
                            .block_task()
                            .await?;
                        info!(harness = harness_name, "ACP initialized");

                        while let Some(cmd) = cmd_rx.recv().await {
                            match cmd {
                                Cmd::NewSession {
                                    api_key_override,
                                    provider_override,
                                    per_conversation_mcp,
                                    reply,
                                } => {
                                    let agent_def = agent_def_for_driver.read().await.clone();
                                    let mut req = NewSessionRequest::new(cwd_for_driver.clone());
                                    let mcp_servers = build_mcp_servers(
                                        &agent_def.mcp_servers,
                                        per_conversation_mcp.as_deref(),
                                    );
                                    if !mcp_servers.is_empty() {
                                        req = req.mcp_servers(mcp_servers);
                                    }
                                    if let Some(meta) = adapter_for_driver.build_session_meta(
                                        &agent_def,
                                        api_key_override.as_deref(),
                                        provider_override.as_ref(),
                                    ) {
                                        req = req.meta(meta);
                                    }
                                    match cx.send_request(req).block_task().await {
                                        Ok(resp) => {
                                            let _ = reply.send(Ok(resp.session_id));
                                        }
                                        Err(e) => {
                                            let _ =
                                                reply.send(Err(format!("session/new failed: {e}")));
                                        }
                                    }
                                }
                                Cmd::Prompt {
                                    session_id,
                                    text,
                                    reply,
                                } => {
                                    let req = PromptRequest::new(
                                        session_id.clone(),
                                        vec![ContentBlock::Text(TextContent::new(text))],
                                    );
                                    let send = cx.send_request(req);
                                    let agent_id = agent_id_for_prompt.clone();
                                    let event_bus = event_bus_for_prompt.clone();
                                    let conv_id = session_id.0.to_string();
                                    tokio::spawn(async move {
                                        match send.block_task().await {
                                            Ok(resp) => {
                                                let stop = format!("{:?}", resp.stop_reason)
                                                    .to_ascii_lowercase();
                                                event_bus.emit(BridgeEvent::new(
                                                    BridgeEventType::TurnCompleted,
                                                    &agent_id,
                                                    &conv_id,
                                                    json!({ "stop_reason": stop }),
                                                ));
                                            }
                                            Err(e) => {
                                                error!(
                                                    agent_id = %agent_id,
                                                    conversation_id = %conv_id,
                                                    error = %e,
                                                    "ACP prompt failed"
                                                );
                                                event_bus.emit(BridgeEvent::new(
                                                    BridgeEventType::AgentError,
                                                    &agent_id,
                                                    &conv_id,
                                                    json!({ "error": e.to_string() }),
                                                ));
                                            }
                                        }
                                    });
                                    let _ = reply.send(Ok(()));
                                }
                                Cmd::Cancel { session_id, reply } => {
                                    let _ =
                                        cx.send_notification(CancelNotification::new(session_id));
                                    let _ = reply.send(Ok(()));
                                }
                                Cmd::LoadSession { session_id, reply } => {
                                    let agent_def = agent_def_for_driver.read().await.clone();
                                    let mcp_servers =
                                        build_mcp_servers(&agent_def.mcp_servers, None);
                                    let mut req =
                                        LoadSessionRequest::new(session_id, cwd_for_driver.clone());
                                    if !mcp_servers.is_empty() {
                                        req = req.mcp_servers(mcp_servers);
                                    }
                                    match cx.send_request(req).block_task().await {
                                        Ok(_) => {
                                            let _ = reply.send(Ok(()));
                                        }
                                        Err(e) => {
                                            let _ = reply
                                                .send(Err(format!("session/load failed: {e}")));
                                        }
                                    }
                                }
                            }
                        }
                        Ok(())
                    },
                )
                .await;
            if let Err(e) = result {
                error!(harness = harness_name, error = %e, "ACP connection driver exited");
            }
        });

        Ok(Arc::new(Self {
            agent_id,
            agent_def,
            cmd_tx,
            sessions,
            event_bus,
            _driver: driver,
            _child_watcher: child_handle,
        }))
    }

    /// Update the active agent definition. Picked up on the next session creation.
    pub async fn set_definition(&self, def: AgentDefinition) {
        *self.agent_def.write().await = def;
    }

    pub async fn create_conversation(
        &self,
        api_key_override: Option<String>,
        provider_override: Option<bridge_core::ProviderConfig>,
        per_conversation_mcp: Option<Vec<McpServerDefinition>>,
    ) -> Result<ConversationContext, BridgeError> {
        let (reply_tx, reply_rx) = oneshot::channel();
        self.cmd_tx
            .send(Cmd::NewSession {
                api_key_override,
                provider_override,
                per_conversation_mcp,
                reply: reply_tx,
            })
            .await
            .map_err(|_| BridgeError::HarnessError("harness driver dropped".into()))?;
        let session_id = reply_rx
            .await
            .map_err(|_| BridgeError::HarnessError("session creation cancelled".into()))?
            .map_err(BridgeError::HarnessError)?;

        self.event_bus.register_sse_stream(session_id.0.to_string());
        self.sessions.insert(
            session_id.0.to_string(),
            SessionState {
                session_id: session_id.clone(),
            },
        );

        Ok(ConversationContext {
            agent_id: self.agent_id.clone(),
            conversation_id: session_id.0.to_string(),
        })
    }

    pub async fn restore_conversation(
        &self,
        conversation_id: &str,
    ) -> Result<ConversationContext, BridgeError> {
        let session_id = SessionId::new(conversation_id);
        let (reply_tx, reply_rx) = oneshot::channel();
        self.cmd_tx
            .send(Cmd::LoadSession {
                session_id: session_id.clone(),
                reply: reply_tx,
            })
            .await
            .map_err(|_| BridgeError::HarnessError("harness driver dropped".into()))?;
        reply_rx
            .await
            .map_err(|_| BridgeError::HarnessError("session load cancelled".into()))?
            .map_err(BridgeError::HarnessError)?;

        self.event_bus.register_sse_stream(session_id.0.to_string());
        self.sessions.insert(
            session_id.0.to_string(),
            SessionState {
                session_id: session_id.clone(),
            },
        );
        Ok(ConversationContext {
            agent_id: self.agent_id.clone(),
            conversation_id: session_id.0.to_string(),
        })
    }

    pub async fn send_message(
        &self,
        conversation_id: &str,
        content: String,
        _system_reminder: Option<String>,
    ) -> Result<(), BridgeError> {
        let session_id = self
            .sessions
            .get(conversation_id)
            .map(|s| s.session_id.clone())
            .ok_or_else(|| BridgeError::ConversationNotFound(conversation_id.into()))?;

        let (reply_tx, reply_rx) = oneshot::channel();
        self.cmd_tx
            .send(Cmd::Prompt {
                session_id,
                text: content,
                reply: reply_tx,
            })
            .await
            .map_err(|_| BridgeError::HarnessError("harness driver dropped".into()))?;
        reply_rx
            .await
            .map_err(|_| BridgeError::HarnessError("prompt cancelled".into()))?
            .map_err(BridgeError::HarnessError)
    }

    pub async fn abort(&self, conversation_id: &str) -> Result<(), BridgeError> {
        let session_id = self
            .sessions
            .get(conversation_id)
            .map(|s| s.session_id.clone())
            .ok_or_else(|| BridgeError::ConversationNotFound(conversation_id.into()))?;
        let (reply_tx, reply_rx) = oneshot::channel();
        self.cmd_tx
            .send(Cmd::Cancel {
                session_id,
                reply: reply_tx,
            })
            .await
            .map_err(|_| BridgeError::HarnessError("harness driver dropped".into()))?;
        reply_rx
            .await
            .map_err(|_| BridgeError::HarnessError("cancel cancelled".into()))?
            .map_err(BridgeError::HarnessError)
    }

    pub async fn end(&self, conversation_id: &str) {
        self.sessions.remove(conversation_id);
        self.event_bus.remove_sse_stream(conversation_id);
    }

    pub async fn shutdown(&self) {
        for entry in self.sessions.iter() {
            self.event_bus.remove_sse_stream(entry.key());
        }
        self.sessions.clear();
    }
}

async fn handle_notification(
    agent_id: &str,
    _sessions: &DashMap<String, SessionState>,
    event_bus: &EventBus,
    notification: SessionNotification,
) {
    let conv_id = notification.session_id.0.to_string();
    let events = events::map_update(agent_id, &conv_id, &notification.update);
    for ev in events {
        event_bus.emit(ev);
    }
}

async fn handle_permission(
    perm: Arc<PermissionManager>,
    event_bus: Arc<EventBus>,
    _sessions: &DashMap<String, SessionState>,
    agent_id: &str,
    req: RequestPermissionRequest,
    responder: Responder<RequestPermissionResponse>,
) -> Result<(), agent_client_protocol::Error> {
    let conv_id = req.session_id.0.to_string();

    let allow_id = req
        .options
        .iter()
        .find(|o| o.option_id.0.as_ref() == "allow")
        .map(|o| o.option_id.clone())
        .or_else(|| req.options.first().map(|o| o.option_id.clone()));
    let reject_id = req
        .options
        .iter()
        .find(|o| o.option_id.0.as_ref() == "reject")
        .map(|o| o.option_id.clone());

    let tool_name = req
        .tool_call
        .fields
        .title
        .clone()
        .unwrap_or_else(|| "unknown".to_string());
    let arguments = req
        .tool_call
        .fields
        .raw_input
        .clone()
        .unwrap_or(Value::Null);
    let tool_call_id = req.tool_call.tool_call_id.0.to_string();

    let result = perm
        .request_approval(
            agent_id,
            &conv_id,
            &tool_name,
            &tool_call_id,
            &arguments,
            &event_bus,
            None,
            None,
        )
        .await;

    let outcome = match &result {
        Ok((bridge_core::ApprovalDecision::Approve, _)) => match &allow_id {
            Some(id) => {
                RequestPermissionOutcome::Selected(SelectedPermissionOutcome::new(id.clone()))
            }
            None => RequestPermissionOutcome::Cancelled,
        },
        Ok((bridge_core::ApprovalDecision::Deny, _)) => match &reject_id {
            Some(id) => {
                RequestPermissionOutcome::Selected(SelectedPermissionOutcome::new(id.clone()))
            }
            None => RequestPermissionOutcome::Cancelled,
        },
        Err(_) => RequestPermissionOutcome::Cancelled,
    };

    responder.respond(RequestPermissionResponse::new(outcome))
}

fn build_mcp_servers(
    agent: &[McpServerDefinition],
    per_conv: Option<&[McpServerDefinition]>,
) -> Vec<agent_client_protocol::schema::McpServer> {
    let mut out = Vec::new();
    for s in agent.iter().chain(per_conv.unwrap_or(&[]).iter()) {
        out.push(translate_mcp(s));
    }
    out
}

fn translate_mcp(def: &McpServerDefinition) -> agent_client_protocol::schema::McpServer {
    use agent_client_protocol::schema::{
        EnvVariable, HttpHeader, McpServer, McpServerHttp, McpServerStdio,
    };
    use bridge_core::mcp::McpTransport;
    match &def.transport {
        McpTransport::Stdio { command, args, env } => {
            let env_vec: Vec<EnvVariable> = env
                .iter()
                .map(|(k, v)| EnvVariable::new(k.clone(), v.clone()))
                .collect();
            McpServer::Stdio(
                McpServerStdio::new(def.name.clone(), PathBuf::from(command))
                    .args(args.clone())
                    .env(env_vec),
            )
        }
        McpTransport::StreamableHttp { url, headers } => {
            let header_vec: Vec<HttpHeader> = headers
                .iter()
                .map(|(k, v)| HttpHeader::new(k.clone(), v.clone()))
                .collect();
            McpServer::Http(McpServerHttp::new(def.name.clone(), url.clone()).headers(header_vec))
        }
    }
}
