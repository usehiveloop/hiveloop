package gateway

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

type recordingRuntime struct {
	mu       sync.Mutex
	messages []RuntimeMessage
}

func (r *recordingRuntime) Send(_ context.Context, message RuntimeMessage) (*RuntimeDelivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, message)
	return &RuntimeDelivery{
		SessionID: runtimeSessionID(message.ConversationID),
		StreamID:  "stream-" + message.GatewayExternalMsgID,
		TraceID:   "trace-" + message.GatewayExternalMsgID,
		TurnID:    "turn-" + message.GatewayExternalMsgID,
	}, nil
}

func (r *recordingRuntime) Sent() []RuntimeMessage {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]RuntimeMessage, len(r.messages))
	copy(out, r.messages)
	return out
}

func TestServiceReceiveWebhookCreatesAndReusesGatewaySession(t *testing.T) {
	db := connectGatewayTestDB(t)
	route := seedGatewayRoute(t, db)
	runtime := &recordingRuntime{}
	service := NewService(db, runtime, nil, NewFakeSlackAdapter())

	first, err := service.ReceiveWebhook(t.Context(), WebhookEnvelope{
		RouteID: route.ID,
		Body:    fakeSlackBody("100.000", "", ""),
	})
	if err != nil {
		t.Fatalf("receive first webhook: %v", err)
	}
	if first.Duplicate || first.Ignored {
		t.Fatalf("first webhook should be delivered, got duplicate=%v ignored=%v", first.Duplicate, first.Ignored)
	}
	if first.Session.Source != Source || first.Session.SourceID == nil || *first.Session.SourceID != route.ID {
		t.Fatalf("session source not bound to route: %#v", first.Session)
	}

	second, err := service.ReceiveWebhook(t.Context(), WebhookEnvelope{
		RouteID: route.ID,
		Body:    fakeSlackBody("101.000", "100.000", "Can you expand?"),
	})
	if err != nil {
		t.Fatalf("receive threaded webhook: %v", err)
	}
	if second.Session.ID != first.Session.ID {
		t.Fatalf("threaded message created a new session: first=%s second=%s", first.Session.ID, second.Session.ID)
	}

	duplicate, err := service.ReceiveWebhook(t.Context(), WebhookEnvelope{
		RouteID: route.ID,
		Body:    fakeSlackBody("101.000", "100.000", "Can you expand?"),
	})
	if err != nil {
		t.Fatalf("receive duplicate webhook: %v", err)
	}
	if !duplicate.Duplicate {
		t.Fatalf("expected duplicate webhook to be deduped")
	}

	sent := runtime.Sent()
	if len(sent) != 2 {
		t.Fatalf("runtime sends = %d, want 2", len(sent))
	}
	if sent[0].ConversationID != sent[1].ConversationID {
		t.Fatalf("threaded messages should use same runtime conversation")
	}
	if sent[0].GatewayProvider != FakeSlackProvider || sent[0].GatewayThreadID != "100.000" {
		t.Fatalf("runtime gateway metadata missing: %#v", sent[0])
	}

	var sessions int64
	db.Model(&model.EmployeeSession{}).Where("source = ? AND source_id = ?", Source, route.ID).Count(&sessions)
	if sessions != 1 {
		t.Fatalf("gateway sessions = %d, want 1", sessions)
	}
}

func TestServiceHandleRuntimeFinalSendsAndDedupesProviderReply(t *testing.T) {
	db := connectGatewayTestDB(t)
	route := seedGatewayRoute(t, db)
	runtime := &recordingRuntime{}
	adapter := NewFakeSlackAdapter()
	service := NewService(db, runtime, nil, adapter)

	received, err := service.ReceiveWebhook(t.Context(), WebhookEnvelope{
		RouteID: route.ID,
		Body:    fakeSlackBody("200.000", "", "Please investigate"),
	})
	if err != nil {
		t.Fatalf("receive webhook: %v", err)
	}
	first, err := service.HandleRuntimeFinal(t.Context(), AgentResponse{
		RuntimeSessionID: received.Runtime.SessionID,
		TurnID:           "turn-final-1",
		Text:             "Done with the investigation.",
	})
	if err != nil {
		t.Fatalf("handle runtime final: %v", err)
	}
	if first == nil || first.Status != "sent" {
		t.Fatalf("delivery not sent: %#v", first)
	}

	second, err := service.HandleRuntimeFinal(t.Context(), AgentResponse{
		RuntimeSessionID: received.Runtime.SessionID,
		TurnID:           "turn-final-1",
		Text:             "Done with the investigation.",
	})
	if err != nil {
		t.Fatalf("handle duplicate runtime final: %v", err)
	}
	if second == nil || second.ID != first.ID {
		t.Fatalf("duplicate final should return existing delivery")
	}
	if sent := adapter.SentMessages(); len(sent) != 1 || sent[0].ChannelID != "C123" || sent[0].ThreadID != "200.000" {
		t.Fatalf("provider sends not deduped or missing routing: %#v", sent)
	}
}

func TestFakeSlackAdapterIgnoresBotMessages(t *testing.T) {
	db := connectGatewayTestDB(t)
	route := seedGatewayRoute(t, db)
	runtime := &recordingRuntime{}
	service := NewService(db, runtime, nil, NewFakeSlackAdapter())

	result, err := service.ReceiveWebhook(t.Context(), WebhookEnvelope{
		RouteID: route.ID,
		Body:    fakeSlackBody("300.000", "", "bot says hi", `"bot_id":"B123"`),
	})
	if err != nil {
		t.Fatalf("receive bot webhook: %v", err)
	}
	if result == nil || !result.Ignored {
		t.Fatalf("bot message should be ignored: %#v", result)
	}
	if sent := runtime.Sent(); len(sent) != 0 {
		t.Fatalf("bot message should not reach runtime: %#v", sent)
	}
}

func seedGatewayRoute(t *testing.T, db *gorm.DB) model.EmployeeGatewayRoute {
	t.Helper()
	org := model.Org{Name: "Gateway Test " + uuid.NewString()}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	employee := model.Employee{OrgID: &org.ID, Model: "test-model", Status: "active"}
	if err := db.Create(&employee).Error; err != nil {
		t.Fatalf("create employee: %v", err)
	}
	sandbox := model.Sandbox{
		OrgID:                 &org.ID,
		EmployeeID:            &employee.ID,
		ExternalID:            "gateway-test-" + uuid.NewString(),
		RuntimeURL:             "http://localhost:1",
		EncryptedRuntimeSecret: []byte("test-key"),
		Status:                "running",
	}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	route := model.EmployeeGatewayRoute{
		OrgID:      org.ID,
		EmployeeID: employee.ID,
		Provider:   FakeSlackProvider,
		Name:       "Fake Slack",
		Enabled:    true,
		Config:     model.JSON{},
	}
	if err := db.Create(&route).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}
	return route
}

func fakeSlackBody(ts, threadTS, text string, extra ...string) []byte {
	if text == "" {
		text = "Please handle this"
	}
	fields := []string{
		`"type":"app_mention"`,
		`"team_id":"T123"`,
		`"channel":"C123"`,
		`"user":"U123"`,
		`"user_name":"Ada"`,
		fmt.Sprintf(`"text":%q`, text),
		fmt.Sprintf(`"ts":%q`, ts),
	}
	if threadTS != "" {
		fields = append(fields, fmt.Sprintf(`"thread_ts":%q`, threadTS))
	}
	fields = append(fields, extra...)
	body := `{"type":"event_callback","event":{` + strings.Join(fields, ",") + `}}`
	return []byte(body)
}

func connectGatewayTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	baseDSN := os.Getenv("DATABASE_URL")
	if baseDSN == "" {
		baseDSN = testdb.DefaultDatabaseURL
	}
	dbName := "hivy_gateway_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	maintenanceDSN, testDSN := gatewayTestDatabaseDSNs(t, baseDSN, dbName)
	maintenanceDB, err := gorm.Open(postgres.Open(maintenanceDSN), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	maintenanceSQL, err := maintenanceDB.DB()
	if err != nil {
		t.Fatalf("maintenance sql db: %v", err)
	}
	if err := maintenanceDB.Exec(`CREATE DATABASE ` + dbName).Error; err != nil {
		_ = maintenanceSQL.Close()
		t.Fatalf("create isolated test database: %v", err)
	}
	db, err := gorm.Open(postgres.Open(testDSN), &gorm.Config{})
	if err != nil {
		_ = maintenanceDB.Exec(`DROP DATABASE IF EXISTS ` + dbName).Error
		_ = maintenanceSQL.Close()
		t.Fatalf("connect isolated test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		_ = maintenanceDB.Exec(`DROP DATABASE IF EXISTS ` + dbName).Error
		_ = maintenanceSQL.Close()
		t.Fatalf("isolated sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(3)
	sqlDB.SetMaxIdleConns(1)
	t.Cleanup(func() {
		_ = sqlDB.Close()
		_ = maintenanceDB.Exec(`DROP DATABASE IF EXISTS ` + dbName).Error
		_ = maintenanceSQL.Close()
	})
	testdb.ApplyMigrations(t, db)
	return db
}

func gatewayTestDatabaseDSNs(t *testing.T, dsn, dbName string) (string, string) {
	t.Helper()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse database url: %v", err)
	}
	maintenance := *u
	maintenance.Path = "/postgres"
	testDB := *u
	testDB.Path = "/" + dbName
	return maintenance.String(), testDB.String()
}
