package framework

import (
	"fmt"
	"slices"

	"github.com/quobyte/api/v4/quobyte"
)

// DeleteFilesTaskScheduled reports whether Quobyte has a DELETE_FILES_IN_VOLUMES task
// restricted to volumeUuid whose DeleteFilesSettings.DirectoryPath matches directoryPath --
// what DeleteVolume schedules for a "tenant|volume|subdir" handle instead of erasing the
// volume itself, when UseDeleteFilesTask is set (see DeleteVolume in
// src/driver/controller.go).
//
// directoryPath is the value the driver puts in DeleteFilesSettings.DirectoryPath, "/" plus
// the subdirectory name -- not a filesystem path on this machine. volumeUuid is matched
// against the task's Scope, which is how the API echoes back CreateTaskRequest's
// RestrictToVolumes.
func DeleteFilesTaskScheduled(client *quobyte.QuobyteClient, volumeUuid, directoryPath string) (bool, error) {
	taskType := quobyte.TaskType_DELETE_FILES_IN_VOLUMES
	resp, err := client.GetTaskList(&quobyte.GetTaskListRequest{
		TaskType: []*quobyte.TaskType{&taskType},
	})
	if err != nil {
		return false, fmt.Errorf("listing delete-files tasks: %w", err)
	}

	for _, task := range resp.Tasks {
		if task == nil || task.DeleteFilesSettings.DirectoryPath != directoryPath {
			continue
		}
		if taskScopedToVolume(task, volumeUuid) {
			return true, nil
		}
	}

	return false, nil
}

// taskScopedToVolume reports whether task's Scope names volumeUuid as one of the volumes it
// is restricted to.
func taskScopedToVolume(task *quobyte.TaskInfo, volumeUuid string) bool {
	for _, subject := range task.Scope {
		if subject != nil && subject.Type == quobyte.SubjectList_Type_VOLUME &&
			slices.Contains(subject.SubjectId, volumeUuid) {
			return true
		}
	}

	return false
}
