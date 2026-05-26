package specialisttasks

import "github.com/usehivy/hivy/internal/model"

const launchWakeReminder = "System reminder: please use the wake tool if this task is going to take longer than 30 seconds. Notify the user that you will check back in a specific amount of time. You can reuse wake as many times as needed instead of polling."

func newLaunchResponse(task model.SpecialistTask) *LaunchResponse {
	return &LaunchResponse{
		TaskID:            task.ID.String(),
		SpecialistSlug:    task.SpecialistSlug,
		EmployeeSessionID: task.EmployeeSessionID,
		SandboxID:         task.SandboxID.String(),
		Status:            task.Status,
		Message:           "Specialist task launched and the brief was delivered.",
		SystemReminder:    launchWakeReminder,
		NextAction:        "Use specialist_task_status with this task_id for quick checks. " + launchWakeReminder,
	}
}
