package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// ListSpecialistRuntimes returns all specialists for the employee with their top 3
// most recent tasks and top 10 most recent events per task.
func (h *SpecialistTaskHandler) ListSpecialistRuntimes(w http.ResponseWriter, r *http.Request) {
	employee := h.authEmployee(w, r)
	if employee == nil {
		return
	}

	disabled := disabledSpecialistSet(employee.DisabledSpecialists)
	agents := make([]model.Agent, 0, len(specialistTemplates))
	specialistIDs := make([]uuid.UUID, 0, len(specialistTemplates))
	for i := range specialistTemplates {
		template := &specialistTemplates[i]
		if disabled[template.Slug] {
			continue
		}
		agent := specialistAgentFromTemplate(employee, template)
		agents = append(agents, agent)
		specialistIDs = append(specialistIDs, agent.ID)
	}

	var allTasks []model.CloudAgentTask
	if err := h.db.
		Where("employee_agent_id = ? AND cloud_agent_id IN ?", employee.ID, specialistIDs).
		Order("created_at DESC").
		Find(&allTasks).Error; err != nil {
		captureCloudAgentFailure(r.Context(), "list_cloud_agents", err, cloudAgentSentryContext{
			Operation:  "load_recent_tasks",
			OrgID:      uuidValue(employee.OrgID),
			EmployeeID: employee.ID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load tasks"})
		return
	}

	tasksByAgent := make(map[uuid.UUID][]model.CloudAgentTask)
	for _, t := range allTasks {
		list := tasksByAgent[t.CloudAgentID]
		if len(list) < 3 {
			tasksByAgent[t.CloudAgentID] = append(list, t)
		}
	}

	var convIDs []uuid.UUID
	for _, tasks := range tasksByAgent {
		for _, t := range tasks {
			convIDs = append(convIDs, t.ConversationID)
		}
	}
	eventsByConv := h.recentEventsByConversation(convIDs, 10)

	result := make([]cloudAgentWithTasks, 0, len(agents))
	for _, agent := range agents {
		ca := cloudAgentWithTasks{
			ID:           agent.ID.String(),
			Name:         agent.Name,
			SystemPrompt: agent.SystemPrompt,
			Model:        agent.Model,
			Tools:        agent.Tools,
			Skills:       agent.Skills,
			RecentTasks:  []cloudAgentTaskWithEvents{},
		}

		for _, task := range tasksByAgent[agent.ID] {
			tw := cloudAgentTaskWithEvents{
				ID:             task.ID.String(),
				Brief:          task.Brief,
				Metadata:       task.Metadata,
				ConversationID: task.ConversationID.String(),
				SandboxID:      task.SandboxID.String(),
				CreatedAt:      task.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
				RecentEvents:   []cloudAgentEventBrief{},
			}
			for _, ev := range eventsByConv[task.ConversationID] {
				tw.RecentEvents = append(tw.RecentEvents, cloudAgentEventBrief{
					EventType: ev.EventType,
					Data:      ev.Data,
					CreatedAt: ev.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
				})
			}
			ca.RecentTasks = append(ca.RecentTasks, tw)
		}

		result = append(result, ca)
	}

	writeJSON(w, http.StatusOK, map[string]any{"cloud_agents": result})
}

// ListTasks returns paginated tasks for a specific specialist.
func (h *SpecialistTaskHandler) ListTasks(w http.ResponseWriter, r *http.Request) {
	employee := h.authEmployee(w, r)
	if employee == nil {
		return
	}

	_, agentID, ok := h.validateSpecialist(r.Context(), w, employee, chi.URLParam(r, "specialistSlug"))
	if !ok {
		return
	}

	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	q := h.db.Where("org_id = ? AND employee_agent_id = ? AND cloud_agent_id = ?", employee.OrgID, employee.ID, agentID)
	q = applyPagination(q, cursor, limit)

	var tasks []model.CloudAgentTask
	if err := q.Find(&tasks).Error; err != nil {
		captureCloudAgentFailure(r.Context(), "list_tasks", err, cloudAgentSentryContext{
			Operation:    "load_tasks",
			OrgID:        uuidValue(employee.OrgID),
			EmployeeID:   employee.ID,
			CloudAgentID: agentID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load tasks"})
		return
	}

	hasMore := len(tasks) > limit
	if hasMore {
		tasks = tasks[:limit]
	}

	eventsByConv := h.recentEventsByConversation(conversationIDsForTasks(tasks), 10)

	items := make([]cloudAgentTaskResponse, len(tasks))
	for i, t := range tasks {
		items[i] = taskToResponse(t, eventsByConv[t.ConversationID])
	}

	var nextCursor *string
	if hasMore && len(tasks) > 0 {
		last := tasks[len(tasks)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		nextCursor = &c
	}

	writeJSON(w, http.StatusOK, paginatedResponse[cloudAgentTaskResponse]{
		Data:       items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	})
}

// GetTask returns a single task by ID.
func (h *SpecialistTaskHandler) GetTask(w http.ResponseWriter, r *http.Request) {
	employee := h.authEmployee(w, r)
	if employee == nil {
		return
	}

	_, agentID, ok := h.validateSpecialist(r.Context(), w, employee, chi.URLParam(r, "specialistSlug"))
	if !ok {
		return
	}

	taskID, err := uuid.Parse(chi.URLParam(r, "taskID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid task_id"})
		return
	}

	task, ok := h.loadTask(r.Context(), w, employee, agentID, taskID)
	if !ok {
		return
	}

	events := h.recentEventsForConversation(task.ConversationID, 10)
	writeJSON(w, http.StatusOK, taskToResponse(*task, events))
}
