use axum::{
    extract::{Path, State},
    Json,
};

use crate::state::ApiState;

#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/observability/traces/{trace_id}/events",
    params(("trace_id" = String, Path, description = "Trace identifier")),
    responses(
        (status = 200, description = "Trace events", body = Vec<observability::ObservabilityEvent>)
    ),
    security(("bearer" = []))
))]
pub async fn get_trace_events(
    State(state): State<ApiState>,
    Path(trace_id): Path<String>,
) -> Json<Vec<observability::ObservabilityEvent>> {
    Json(state.observability.list_by_trace(&trace_id))
}

#[cfg_attr(feature = "openapi", utoipa::path(
    get,
    path = "/observability/traces/{trace_id}/summary",
    params(("trace_id" = String, Path, description = "Trace identifier")),
    responses(
        (status = 200, description = "Trace summary", body = observability::TraceSummary)
    ),
    security(("bearer" = []))
))]
pub async fn get_trace_summary(
    State(state): State<ApiState>,
    Path(trace_id): Path<String>,
) -> Json<observability::TraceSummary> {
    Json(state.observability.summarize_trace(&trace_id))
}
