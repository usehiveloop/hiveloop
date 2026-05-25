package model

import (
	"time"

	"github.com/google/uuid"
)

type EmployeeGatewayRoute struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`

	OrgID uuid.UUID `gorm:"type:uuid;not null;index"`
	Org   Org       `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`

	EmployeeID uuid.UUID `gorm:"type:uuid;not null;index"`
	Employee   Employee  `gorm:"foreignKey:EmployeeID;constraint:OnDelete:CASCADE"`

	ConnectionID *uuid.UUID  `gorm:"type:uuid;index"`
	Connection   *Connection `gorm:"foreignKey:ConnectionID;constraint:OnDelete:SET NULL"`

	Provider string `gorm:"not null;size:128;index"`
	Name     string `gorm:"type:text;not null;default:''"`
	Enabled  bool   `gorm:"not null;default:true;index"`
	Config   JSON   `gorm:"type:jsonb;not null;default:'{}'"`

	CreatedAt time.Time
	UpdatedAt time.Time
	RevokedAt *time.Time `gorm:"index"`
}

func (EmployeeGatewayRoute) TableName() string { return "employee_gateway_routes" }

type EmployeeGatewayEvent struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`

	OrgID      uuid.UUID `gorm:"type:uuid;not null;index"`
	EmployeeID uuid.UUID `gorm:"type:uuid;not null;index"`
	RouteID    uuid.UUID `gorm:"type:uuid;not null;index"`

	EmployeeSessionID *uuid.UUID            `gorm:"type:uuid;index"`
	EmployeeSession   *EmployeeConversation `gorm:"foreignKey:EmployeeSessionID;constraint:OnDelete:SET NULL"`

	Provider              string  `gorm:"not null;size:128;index"`
	ExternalMessageID     string  `gorm:"type:text;not null;default:''"`
	DedupeKey             string  `gorm:"type:text;not null;default:'';index"`
	ThreadKey             string  `gorm:"type:text;not null;default:'';index"`
	ChannelID             string  `gorm:"type:text;not null;default:''"`
	ThreadID              string  `gorm:"type:text;not null;default:''"`
	SenderID              string  `gorm:"type:text;not null;default:''"`
	Status                string  `gorm:"not null;default:'received';size:32;index"`
	Error                 string  `gorm:"type:text;not null;default:''"`
	RuntimeConversationID string  `gorm:"type:text;not null;default:''"`
	RuntimeSessionID      string  `gorm:"type:text;not null;default:''"`
	RuntimeStreamID       string  `gorm:"type:text;not null;default:''"`
	RuntimeTraceID        string  `gorm:"type:text;not null;default:''"`
	RuntimeTurnID         string  `gorm:"type:text;not null;default:''"`
	Payload               RawJSON `gorm:"type:jsonb;not null;default:'{}'"`

	ReceivedAt  time.Time `gorm:"not null;index"`
	ProcessedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (EmployeeGatewayEvent) TableName() string { return "employee_gateway_events" }

type EmployeeGatewayDelivery struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`

	OrgID             uuid.UUID `gorm:"type:uuid;not null;index"`
	EmployeeID        uuid.UUID `gorm:"type:uuid;not null;index"`
	RouteID           uuid.UUID `gorm:"type:uuid;not null;index"`
	EmployeeSessionID uuid.UUID `gorm:"type:uuid;not null;index"`

	Provider         string  `gorm:"not null;size:128;index"`
	DedupeKey        string  `gorm:"type:text;not null;default:'';index"`
	RuntimeSessionID string  `gorm:"type:text;not null;default:''"`
	RuntimeTraceID   string  `gorm:"type:text;not null;default:''"`
	RuntimeTurnID    string  `gorm:"type:text;not null;default:''"`
	ThreadKey        string  `gorm:"type:text;not null;default:'';index"`
	ChannelID        string  `gorm:"type:text;not null;default:''"`
	ThreadID         string  `gorm:"type:text;not null;default:''"`
	ResponseText     string  `gorm:"type:text;not null;default:''"`
	ProviderHandles  RawJSON `gorm:"type:jsonb;not null;default:'[]'"`
	Status           string  `gorm:"not null;default:'sent';size:32;index"`
	Error            string  `gorm:"type:text;not null;default:''"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (EmployeeGatewayDelivery) TableName() string { return "employee_gateway_deliveries" }
