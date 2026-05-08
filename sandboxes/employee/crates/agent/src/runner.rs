use std::path::PathBuf;
use std::sync::{Arc, Mutex};

use adk_rust::prelude::*;
use adk_rust::session::SessionService;
use async_trait::async_trait;
use domain::{ConfigStore, SessionId};
use futures::{stream::BoxStream, StreamExt};
use gateway::ChannelGateway;
use outbound::OutboundEmitter;
use storage::CronJobRepo;
use tools::ToolBuildContext;

use crate::model_helpers::{build_model, build_user_content, pick_model_for_turn};
use crate::session_helpers::{
    ensure_session, log_full_conversation, seed_history_into_session,
    session_contains_image_parts, session_event_count,
};
use crate::tool_registry::{attach_tool_event_callbacks, build_agent_tools, ToolContext};
use crate::{AgentError, AgentEvent, AgentRunner, Result, TurnInput};

const RUNTIME_USER_ID: &str = "runtime";

pub struct AdkAgentRunner {
    config: ConfigStore,
    session_service: Arc<dyn SessionService>,
    app_name: String,
    tool_context: Arc<ToolBuildContext>,
    outbound_emitter: Option<Arc<OutboundEmitter>>,
    gateway: Option<Arc<dyn ChannelGateway>>,
    cron_repo: Option<Arc<dyn CronJobRepo>>,
}

impl AdkAgentRunner {
    pub fn new(
        config: ConfigStore,
        session_service: Arc<dyn SessionService>,
        app_name: impl Into<String>,
        workspace_root: PathBuf,
    ) -> Self {
        Self {
            config,
            session_service,
            app_name: app_name.into(),
            tool_context: Arc::new(ToolBuildContext::new(workspace_root)),
            outbound_emitter: None,
            gateway: None,
            cron_repo: None,
        }
    }

    pub fn with_in_memory(
        config: ConfigStore,
        app_name: impl Into<String>,
        workspace_root: PathBuf,
    ) -> Self {
        Self::new(
            config,
            Arc::new(InMemorySessionService::new()),
            app_name,
            workspace_root,
        )
    }

    pub fn with_outbound_emitter(mut self, emitter: Arc<OutboundEmitter>) -> Self {
        self.outbound_emitter = Some(emitter);
        self
    }

    pub fn with_gateway(mut self, gateway: Arc<dyn ChannelGateway>) -> Self {
        self.gateway = Some(gateway);
        self
    }

    pub fn with_cron_repo(mut self, cron_repo: Arc<dyn CronJobRepo>) -> Self {
        self.cron_repo = Some(cron_repo);
        self
    }
}

#[async_trait]
impl AgentRunner for AdkAgentRunner {
    async fn run_turn(
        &self,
        session_id: &SessionId,
        user_input: TurnInput,
    ) -> Result<BoxStream<'static, AgentEvent>> {
        let snapshot = self.config.snapshot();
        let agent_name = snapshot.agent.name.clone();

        let was_just_created = ensure_session(
            &self.session_service,
            &self.app_name,
            RUNTIME_USER_ID,
            session_id.as_str(),
        )
        .await?;

        let event_count = session_event_count(
            &self.session_service,
            &self.app_name,
            RUNTIME_USER_ID,
            session_id.as_str(),
        )
        .await?;
        let session_is_empty = was_just_created || event_count == 0;

        if session_is_empty && !user_input.prior_history.is_empty() {
            tracing::info!(
                session_id = %session_id,
                history_size = user_input.prior_history.len(),
                "seeding empty session with thread history"
            );
            seed_history_into_session(
                &self.session_service,
                session_id.as_str(),
                &agent_name,
                &user_input.prior_history,
            )
            .await?;
        }

        let session_has_image_history = session_contains_image_parts(
            &self.session_service,
            &self.app_name,
            RUNTIME_USER_ID,
            session_id.as_str(),
        )
        .await;

        let active_model_config =
            pick_model_for_turn(&snapshot, &user_input, session_has_image_history);
        let model = build_model(active_model_config)?;

        let builtin_tools = tools::build_builtin_tools(&snapshot.tools, &self.tool_context);
        let agent_tools = build_agent_tools(
            &snapshot.tools,
            session_id,
            &ToolContext {
                gateway: self.gateway.clone(),
                cron_repo: self.cron_repo.clone(),
            },
        );

        let mut agent_builder = LlmAgentBuilder::new(agent_name.clone())
            .instruction(snapshot.agent.system_prompt.clone())
            .model(model);
        for tool in builtin_tools {
            agent_builder = agent_builder.tool(tool);
        }
        for tool in agent_tools {
            agent_builder = agent_builder.tool(tool);
        }
        if let Some(emitter) = self.outbound_emitter.as_ref() {
            agent_builder =
                attach_tool_event_callbacks(agent_builder, emitter.clone(), session_id.clone());
        }
        let agent: Arc<dyn Agent> = Arc::new(
            agent_builder
                .build()
                .map_err(|e| AgentError::Other(anyhow::anyhow!("agent build: {e}")))?,
        );

        let runner = Runner::builder()
            .app_name(self.app_name.clone())
            .agent(agent)
            .session_service(self.session_service.clone())
            .build()
            .map_err(|e| AgentError::Other(anyhow::anyhow!("runner build: {e}")))?;

        log_full_conversation(
            &self.session_service,
            &self.app_name,
            RUNTIME_USER_ID,
            session_id.as_str(),
            &snapshot.agent.system_prompt,
            &agent_name,
            &user_input.text,
            user_input.images.len(),
        )
        .await;

        let user_id = adk_rust::UserId::new(RUNTIME_USER_ID)
            .map_err(|e| AgentError::Other(anyhow::anyhow!("user_id: {e}")))?;
        let adk_sid = adk_rust::SessionId::new(session_id.as_str())
            .map_err(|e| AgentError::Other(anyhow::anyhow!("session_id: {e}")))?;

        let content = build_user_content(user_input.text, user_input.images);
        let raw_stream = runner
            .run(user_id, adk_sid, content)
            .await
            .map_err(|e| AgentError::Model(format!("runner.run: {e}")))?;

        let buffer = Arc::new(Mutex::new(String::new()));
        let buffer_for_final = buffer.clone();

        let chunks = raw_stream.filter_map(move |res| {
            let buffer = buffer.clone();
            async move {
                match res {
                    Err(e) => Some(AgentEvent::Error {
                        message: e.to_string(),
                    }),
                    Ok(evt) => {
                        if !evt.llm_response.partial {
                            return None;
                        }
                        let Some(content) = evt.llm_response.content else {
                            return None;
                        };
                        let mut text = String::new();
                        for part in &content.parts {
                            if let Part::Text { text: t } = part {
                                text.push_str(t);
                            }
                        }
                        if text.is_empty() {
                            return None;
                        }
                        buffer.lock().unwrap().push_str(&text);
                        Some(AgentEvent::TokenChunk { text })
                    }
                }
            }
        });

        let final_msg = futures::stream::once(async move {
            let text = buffer_for_final.lock().unwrap().clone();
            AgentEvent::FinalMessage { text }
        });

        Ok(Box::pin(chunks.chain(final_msg)))
    }
}
