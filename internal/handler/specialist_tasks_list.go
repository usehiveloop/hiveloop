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

	attached := attachedSpecialistSet(employee.AttachedSpecialists)
	defs := h.catalog.List()
	agents := make([]model.Employee, 0, len(defs))
	specialistIDs := make([]uuid.UUID, 0, len(defs))
	for _, def := range defs {
		if !attached[def.Slug] {
			continue
		}
		agent := specialistAgentFromDefinition(employee, def)
		agents = append(agents, agent)
		specialistIDs = append(specialistIDs, agent.ID)
	}

	var allTasks []model.SpecialistTask
	if err := h.db.
		Where("employee_id = ? AND specialist_id IN ?", employee.ID, specialistIDs).
		Order("created_at DESC").
		Find(&allTasks).Error; err != nil {
		captureSpecialistFailure(r.Context(), "list_specialists", err, specialistSentryContext{
			Operation:  "load_recent_tasks",
			OrgID:      uuidValue(employee.OrgID),
			EmployeeID: employee.ID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load tasks"})
		return
	}

	tasksByAgent := make(map[uuid.UUID][]model.SpecialistTask)
	for _, t := range allTasks {
		list := tasksByAgent[t.SpecialistID]
		if len(list) < 3 {
			tasksByAgent[t.SpecialistID] = append(list, t)
		}
	}

	var convIDs []uuid.UUID
	for _, tasks := range tasksByAgent {
		for _, t := range tasks {
			convIDs = append(convIDs, t.ConversationID)
		}
	}
	eventsByConv := h.recentEventsByConversation(convIDs, 10)

	result := make([]specialistWithTasks, 0, len(agents))
	for _, agent := range agents {
		ca := specialistWithTasks{
			ID:           agent.ID.String(),
			Name:         agent.Name,
			SystemPrompt: agent.SystemPrompt,
			Model:        agent.Model,
			Tools:        agent.Tools,
			Skills:       agent.Skills,
			RecentTasks:  []specialistTaskWithEvents{},
		}

		for _, task := range tasksByAgent[agent.ID] {
			tw := specialistTaskWithEvents{
				ID:             task.ID.String(),
				Brief:          task.Brief,
				Metadata:       task.Metadata,
				ConversationID: task.ConversationID.String(),
				SandboxID:      task.SandboxID.String(),
				CreatedAt:      task.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
				RecentEvents:   []specialistEventBrief{},
			}
			for _, ev := range eventsByConv[task.ConversationID] {
				tw.RecentEvents = append(tw.RecentEvents, specialistEventBrief{
					EventType: ev.EventType,
					Data:      ev.Data,
					CreatedAt: ev.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
				})
			}
			ca.RecentTasks = append(ca.RecentTasks, tw)
		}

		result = append(result, ca)
	}

	writeJSON(w, http.StatusOK, map[string]any{"specialists": result})
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

	q := h.db.Where("org_id = ? AND employee_id = ? AND specialist_id = ?", employee.OrgID, employee.ID, agentID)
	q = applyPagination(q, cursor, limit)

	var tasks []model.SpecialistTask
	if err := q.Find(&tasks).Error; err != nil {
		captureSpecialistFailure(r.Context(), "list_tasks", err, specialistSentryContext{
			Operation:    "load_tasks",
			OrgID:        uuidValue(employee.OrgID),
			EmployeeID:   employee.ID,
			SpecialistID: agentID,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load tasks"})
		return
	}

	hasMore := len(tasks) > limit
	if hasMore {
		tasks = tasks[:limit]
	}

	eventsByConv := h.recentEventsByConversation(conversationIDsForTasks(tasks), 10)

	items := make([]specialistTaskResponse, len(tasks))
	for i, t := range tasks {
		items[i] = taskToResponse(t, eventsByConv[t.ConversationID])
	}

	var nextCursor *string
	if hasMore && len(tasks) > 0 {
		last := tasks[len(tasks)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		nextCursor = &c
	}

	writeJSON(w, http.StatusOK, paginatedResponse[specialistTaskResponse]{
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
