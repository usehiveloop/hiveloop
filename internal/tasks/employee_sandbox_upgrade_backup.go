package tasks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

type employeeSandboxBackupMetadata struct {
	Key    string `json:"key"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

func (h *EmployeeSandboxUpgradeHandler) runBackup(ctx context.Context, upgrade *model.EmployeeSandboxUpgrade, agent *model.Employee, sb *model.Sandbox) (*employeeSandboxBackupMetadata, error) {
	key := employeeSandboxUpgradeBackupKey(upgrade.OrgID, agent.ID, upgrade.ID)
	uploadURL, err := h.store.PresignedPutURL(ctx, key, time.Hour)
	if err != nil {
		return nil, fmt.Errorf("presign backup upload: %w", err)
	}
	cmd := buildEmployeeSandboxBackupCommand(uploadURL)
	output, err := h.orchestrator.ExecuteCommandWithTimeout(ctx, sb, cmd, employeeSandboxUpgradeCommandTimeout)
	if err != nil {
		return nil, fmt.Errorf("backup command failed: %w", err)
	}
	var meta employeeSandboxBackupMetadata
	if err := json.Unmarshal([]byte(lastJSONLine(output)), &meta); err != nil {
		return nil, fmt.Errorf("parse backup metadata: %w: %s", err, output)
	}
	if meta.SHA256 == "" || meta.Bytes <= 0 {
		return nil, fmt.Errorf("backup metadata missing sha256 or bytes")
	}
	meta.Key = key
	return &meta, nil
}

func (h *EmployeeSandboxUpgradeHandler) verifyAndRecordBackup(ctx context.Context, upgrade *model.EmployeeSandboxUpgrade, meta *employeeSandboxBackupMetadata) error {
	info, err := h.store.Head(ctx, meta.Key)
	if err != nil {
		return fmt.Errorf("verify backup object: %w", err)
	}
	if info.Size != meta.Bytes {
		return fmt.Errorf("backup object size mismatch: command reported %d bytes, S3 has %d bytes", meta.Bytes, info.Size)
	}
	if _, err := hex.DecodeString(meta.SHA256); err != nil || len(meta.SHA256) != sha256.Size*2 {
		return fmt.Errorf("backup sha256 is invalid")
	}
	if err := h.db.WithContext(ctx).Model(upgrade).Updates(map[string]any{
		"backup_key":    meta.Key,
		"backup_sha256": meta.SHA256,
		"backup_bytes":  meta.Bytes,
	}).Error; err != nil {
		return fmt.Errorf("record backup metadata: %w", err)
	}
	upgrade.BackupKey = &meta.Key
	upgrade.BackupSHA256 = &meta.SHA256
	upgrade.BackupBytes = meta.Bytes
	return nil
}

func (h *EmployeeSandboxUpgradeHandler) runRestore(ctx context.Context, meta *employeeSandboxBackupMetadata, sb *model.Sandbox) error {
	url, err := h.store.PresignedURL(ctx, meta.Key, time.Hour)
	if err != nil {
		return fmt.Errorf("presign backup: %w", err)
	}
	cmd := buildEmployeeSandboxRestoreCommand(url, meta.SHA256)
	output, err := h.orchestrator.ExecuteCommandWithTimeout(ctx, sb, cmd, employeeSandboxUpgradeCommandTimeout)
	if err != nil {
		return fmt.Errorf("restore command failed: %w", err)
	}
	if !strings.Contains(lastJSONLine(output), `"status":"ok"`) {
		return fmt.Errorf("restore command did not confirm success: %s", output)
	}
	return nil
}

func employeeSandboxUpgradeBackupKey(orgID, agentID, upgradeID uuid.UUID) string {
	return fmt.Sprintf("employee-sqlite-backups/%s/%s/upgrades/%s.db.gz", orgID, agentID, upgradeID)
}

func buildEmployeeSandboxBackupCommand(uploadURL string) string {
	return strings.Join([]string{
		"set -eu",
		`DB="${DB_PATH:-/app/data/employee-bridge.db}"`,
		`TMP_DIR="$(mktemp -d)"`,
		`trap 'rm -rf "$TMP_DIR"' EXIT`,
		`SNAP="$TMP_DIR/employee-bridge.db"`,
		`GZ="$TMP_DIR/employee-bridge.db.gz"`,
		`sqlite3 "$DB" "PRAGMA wal_checkpoint(FULL);" >/dev/null`,
		`sqlite3 "$DB" "VACUUM main INTO '$SNAP';"`,
		`CHECK="$(sqlite3 "$SNAP" "PRAGMA integrity_check;")"`,
		`[ "$CHECK" = "ok" ] || { echo "integrity_check failed: $CHECK" >&2; exit 2; }`,
		`gzip -c "$SNAP" > "$GZ"`,
		`SHA="$(sha256sum "$GZ" | awk '{print $1}')"`,
		`BYTES="$(wc -c < "$GZ" | tr -d ' ')"`,
		fmt.Sprintf("UPLOAD_URL=%s", shellQuote(uploadURL)),
		`curl -fsS -X PUT --data-binary "@$GZ" "$UPLOAD_URL" >/dev/null`,
		`printf '{"sha256":"%s","bytes":%s}\n' "$SHA" "$BYTES"`,
	}, "\n")
}

func buildEmployeeSandboxRestoreCommand(presignedURL, sha256Hex string) string {
	return strings.Join([]string{
		"set -eu",
		`DB="${DB_PATH:-/app/data/employee-bridge.db}"`,
		`TMP_DIR="$(mktemp -d)"`,
		`trap 'rm -rf "$TMP_DIR"' EXIT`,
		`GZ="$TMP_DIR/employee-bridge.db.gz"`,
		`RESTORE="$TMP_DIR/employee-bridge.db"`,
		fmt.Sprintf("curl -fsSL %s -o \"$GZ\"", shellQuote(presignedURL)),
		fmt.Sprintf("printf '%%s  %%s\\n' %s \"$GZ\" | sha256sum -c -", shellQuote(sha256Hex)),
		`gzip -dc "$GZ" > "$RESTORE"`,
		`CHECK="$(sqlite3 "$RESTORE" "PRAGMA integrity_check;")"`,
		`[ "$CHECK" = "ok" ] || { echo "integrity_check failed: $CHECK" >&2; exit 2; }`,
		`mkdir -p "$(dirname "$DB")"`,
		`rm -f "$DB" "$DB-wal" "$DB-shm"`,
		`install -m 600 "$RESTORE" "$DB"`,
		`printf '{"status":"ok"}\n'`,
	}, "\n")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func lastJSONLine(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
			return line
		}
	}
	return strings.TrimSpace(output)
}
