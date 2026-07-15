package domain

import (
	"encoding/json"
	"time"
)

type ProjectMode string

const (
	ModeGreenfield ProjectMode = "greenfield"
	ModeFeature    ProjectMode = "feature"
	ModeRefactor   ProjectMode = "refactor"
	ModeSpike      ProjectMode = "spike"
)

type ProjectStage string

const (
	StageIntake       ProjectStage = "INTAKE"
	StageFraming      ProjectStage = "FRAMING"
	StageContext      ProjectStage = "CONTEXT"
	StageScenarios    ProjectStage = "SCENARIOS"
	StageRequirements ProjectStage = "REQUIREMENTS"
	StageShaping      ProjectStage = "SHAPING"
	StageDecisions    ProjectStage = "DECISIONS"
	StageDelivery     ProjectStage = "DELIVERY"
	StageReview       ProjectStage = "REVIEW"
	StageReady        ProjectStage = "READY"
)

type ProjectStatus string

const (
	ProjectActive   ProjectStatus = "active"
	ProjectReady    ProjectStatus = "ready"
	ProjectArchived ProjectStatus = "archived"
	ProjectDeleted  ProjectStatus = "deleted"
)

type Appetite struct {
	Kind        string  `json:"kind"`
	Value       float64 `json:"value"`
	Unit        string  `json:"unit"`
	Flexibility string  `json:"flexibility"`
}

type Project struct {
	ID             string        `json:"id"`
	Name           string        `json:"name"`
	RawRequest     string        `json:"raw_request"`
	Mode           ProjectMode   `json:"mode"`
	OutputLanguage string        `json:"output_language"`
	Stage          ProjectStage  `json:"stage"`
	Status         ProjectStatus `json:"status"`
	Appetite       *Appetite     `json:"appetite,omitempty"`
	Revision       int64         `json:"revision"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

type EntityKind string

const (
	KindGoal            EntityKind = "goal"
	KindStakeholder     EntityKind = "stakeholder"
	KindContext         EntityKind = "context"
	KindScopeItem       EntityKind = "scope_item"
	KindConstraint      EntityKind = "constraint"
	KindAssumption      EntityKind = "assumption"
	KindQuestion        EntityKind = "question"
	KindTerm            EntityKind = "term"
	KindScenario        EntityKind = "scenario"
	KindRequirement     EntityKind = "requirement"
	KindQualityScenario EntityKind = "quality_scenario"
	KindRisk            EntityKind = "risk"
	KindOption          EntityKind = "option"
	KindDecision        EntityKind = "decision"
	KindSystemElement   EntityKind = "system_element"
	KindWorkSlice       EntityKind = "work_slice"
	KindExperiment      EntityKind = "experiment"
	KindEvidence        EntityKind = "evidence"
	KindVerification    EntityKind = "verification"
)

type EntityStatus string

const (
	EntityDraft      EntityStatus = "draft"
	EntityProposed   EntityStatus = "proposed"
	EntityConfirmed  EntityStatus = "confirmed"
	EntityRejected   EntityStatus = "rejected"
	EntitySuperseded EntityStatus = "superseded"
	EntityUnresolved EntityStatus = "unresolved"
)

type Origin string

const (
	OriginUser           Origin = "user"
	OriginRepository     Origin = "repository"
	OriginExternalSource Origin = "external_source"
	OriginExperiment     Origin = "experiment"
	OriginAgent          Origin = "agent"
	OriginPolicy         Origin = "policy"
)

type Freshness string

const (
	FreshnessCurrent          Freshness = "current"
	FreshnessPotentiallyStale Freshness = "potentially_stale"
	FreshnessStale            Freshness = "stale"
)

type Entity struct {
	ID         string          `json:"id"`
	ProjectID  string          `json:"project_id"`
	Kind       EntityKind      `json:"kind"`
	Title      string          `json:"title"`
	Body       json.RawMessage `json:"body"`
	Status     EntityStatus    `json:"status"`
	Origin     Origin          `json:"origin"`
	Confidence float64         `json:"confidence"`
	Freshness  Freshness       `json:"freshness"`
	SourceRefs []string        `json:"source_refs"`
	Tags       []string        `json:"tags"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
	Revision   int64           `json:"revision"`
}

type RelationType string

const (
	RelationMotivates     RelationType = "motivates"
	RelationAffects       RelationType = "affects"
	RelationConstrains    RelationType = "constrains"
	RelationAssumes       RelationType = "assumes"
	RelationAnswers       RelationType = "answers"
	RelationDerivesFrom   RelationType = "derives_from"
	RelationSatisfies     RelationType = "satisfies"
	RelationVerifies      RelationType = "verifies"
	RelationMitigates     RelationType = "mitigates"
	RelationSelects       RelationType = "selects"
	RelationRejects       RelationType = "rejects"
	RelationDependsOn     RelationType = "depends_on"
	RelationConflictsWith RelationType = "conflicts_with"
	RelationSupersedes    RelationType = "supersedes"
	RelationImplements    RelationType = "implements"
	RelationDecomposes    RelationType = "decomposes"
	RelationEvidencedBy   RelationType = "evidenced_by"
)

type Relation struct {
	ID        string       `json:"id"`
	ProjectID string       `json:"project_id"`
	FromID    string       `json:"from_id"`
	Type      RelationType `json:"type"`
	ToID      string       `json:"to_id"`
	Rationale string       `json:"rationale"`
	CreatedBy string       `json:"created_by,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
}

type ProjectRevision struct {
	ProjectID string          `json:"project_id"`
	Revision  int64           `json:"revision"`
	Checksum  string          `json:"checksum"`
	Snapshot  json.RawMessage `json:"snapshot,omitempty"`
	Actor     string          `json:"actor"`
	CreatedAt time.Time       `json:"created_at"`
}

type Event struct {
	Sequence   int64           `json:"sequence"`
	ProjectID  string          `json:"project_id"`
	Type       string          `json:"type"`
	Payload    json.RawMessage `json:"payload"`
	OccurredAt time.Time       `json:"occurred_at"`
}

type Snapshot struct {
	SchemaVersion string     `json:"schema_version"`
	Project       Project    `json:"project"`
	Entities      []Entity   `json:"entities"`
	Relations     []Relation `json:"relations"`
}
