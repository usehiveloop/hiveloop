use axum::{
    extract::{Path, State},
    Json,
};

use crate::state::ApiState;

pub async fn get_trace_events(
    State(state): State<ApiState>,
    Path(trace_id): Path<String>,
) -> Json<Vec<observability::ObservabilityEvent>> {
    Json(state.observability.list_by_trace(&trace_id))
}

pub async fn get_trace_summary(
    State(state): State<ApiState>,
    Path(trace_id): Path<String>,
) -> Json<observability::TraceSummary> {
    Json(state.observability.summarize_trace(&trace_id))
}
