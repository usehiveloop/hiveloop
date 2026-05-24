package model

import (
	"time"

	"github.com/google/uuid"
)

// EmployeeTriggerDelivery records a successful trigger injection into an employee
// runtime. EmployeeTrigger remains the source of truth for trigger configuration;
// this table stores only per-run correlation data.
type EmployeeTriggerDelivery struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`

	OrgID uuid.UUID `gorm:"type:uuid;not null;index:idx_trigger_delivery_org_employee_created;index:idx_trigger_delivery_org_employee_session_created,priority:1"`
	Org   Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`

	EmployeeID uuid.UUID `gorm:"type:uuid;not null;index:idx_trigger_delivery_org_employee_created;index:idx_trigger_delivery_org_employee_session_created,priority:2"`
	Employee   Employee  `gorm:"foreignKey:EmployeeID;constraint:OnDelete:CASCADE"`

	TriggerID uuid.UUID       `gorm:"type:uuid;not null;index"`
	Trigger   EmployeeTrigger `gorm:"foreignKey:TriggerID;constraint:OnDelete:CASCADE"`

	ConnectionID *uuid.UUID    `gorm:"type:uuid;index"`
	Connection   *Connection `gorm:"foreignKey:ConnectionID;constraint:OnDelete:SET NULL"`

	DeliveryID  string `gorm:"type:text;not null;index"`
	EventKey    string `gorm:"type:text;not null;default:'';index"`
	ResourceKey string `gorm:"type:text;not null;default:'';index"`

	ConversationID        uuid.UUID            `gorm:"type:uuid;not null;index"`
	Conversation          EmployeeConversation `gorm:"foreignKey:ConversationID;constraint:OnDelete:CASCADE"`
	RuntimeConversationID string               `gorm:"type:text;not null;default:'';index"`
	RuntimeSessionID      string               `gorm:"type:text;not null;default:'';index;index:idx_trigger_delivery_org_employee_session_created,priority:3"`
	RuntimeStreamID       string               `gorm:"type:text;not null;default:''"`
	RuntimeTraceID        string               `gorm:"type:text;not null;default:''"`
	RuntimeTurnID         string               `gorm:"type:text;not null;default:''"`

	Payload RawJSON `gorm:"type:jsonb;not null;default:'{}'"`

	CreatedAt time.Time `gorm:"index:idx_trigger_delivery_org_employee_created;index:idx_trigger_delivery_org_employee_session_created,priority:4"`
}

func (EmployeeTriggerDelivery) TableName() string { return "employee_trigger_deliveries" }
