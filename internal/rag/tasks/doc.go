// Package tasks registers the asynq task handlers that drive the three-loop
// sync architecture: docfetching, docprocessing, pruning,
// doc_permission_syncing, external_group_syncing.
//
// Ports backend/onyx/background/celery/tasks/{docfetching,docprocessing,
// pruning,doc_permission_syncing,external_group_syncing}/. Hiveloop uses
// asynq in place of Celery; task definitions otherwise mirror Onyx 1:1.
package tasks
