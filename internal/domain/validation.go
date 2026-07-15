package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

var (
	ErrInvalid = errors.New("invalid domain value")

	projectModes    = values(string(ModeGreenfield), string(ModeFeature), string(ModeRefactor), string(ModeSpike))
	projectStages   = values(string(StageIntake), string(StageFraming), string(StageContext), string(StageScenarios), string(StageRequirements), string(StageShaping), string(StageDecisions), string(StageDelivery), string(StageReview), string(StageReady))
	projectStatuses = values(string(ProjectActive), string(ProjectReady), string(ProjectArchived), string(ProjectDeleted))
	entityStatuses  = values(string(EntityDraft), string(EntityProposed), string(EntityConfirmed), string(EntityRejected), string(EntitySuperseded), string(EntityUnresolved))
	origins         = values(string(OriginUser), string(OriginRepository), string(OriginExternalSource), string(OriginExperiment), string(OriginAgent), string(OriginPolicy))
	freshnessValues = values(string(FreshnessCurrent), string(FreshnessPotentiallyStale), string(FreshnessStale))
)

type fieldType int

const (
	fieldAny fieldType = iota
	fieldString
	fieldBool
	fieldNumber
	fieldArray
	fieldObject
)

type fieldRule struct {
	typeOf fieldType
	enum   map[string]struct{}
	min    float64
	max    float64
}

type entitySchema struct {
	required map[string]struct{}
	fields   map[string]fieldRule
}

func schema(required []string, fields map[string]fieldRule) entitySchema {
	return entitySchema{required: values(required...), fields: fields}
}

func text() fieldRule                   { return fieldRule{typeOf: fieldString} }
func boolean() fieldRule                { return fieldRule{typeOf: fieldBool} }
func number(min, max float64) fieldRule { return fieldRule{typeOf: fieldNumber, min: min, max: max} }
func array() fieldRule                  { return fieldRule{typeOf: fieldArray} }
func object() fieldRule                 { return fieldRule{typeOf: fieldObject} }
func anyField() fieldRule               { return fieldRule{typeOf: fieldAny} }
func enumeration(options ...string) fieldRule {
	return fieldRule{typeOf: fieldString, enum: values(options...)}
}

var entitySchemas = map[EntityKind]entitySchema{
	KindGoal: schema([]string{"outcome", "success_signals", "priority"}, map[string]fieldRule{
		"outcome": text(), "success_signals": array(), "time_horizon": text(), "priority": enumeration("must", "should", "could"),
	}),
	KindStakeholder: schema([]string{"role", "interests", "authority", "impact", "contact_required"}, map[string]fieldRule{
		"role": text(), "interests": array(), "authority": text(), "impact": text(), "contact_required": boolean(),
	}),
	KindContext: schema([]string{"current_state", "system_boundary", "external_dependencies", "baseline_behavior", "project_mode"}, map[string]fieldRule{
		"current_state": text(), "system_boundary": text(), "external_dependencies": array(), "baseline_behavior": text(), "project_mode": enumeration("greenfield", "feature", "refactor", "spike"),
	}),
	KindScopeItem: schema([]string{"statement", "disposition", "rationale"}, map[string]fieldRule{
		"statement": text(), "disposition": enumeration("in_scope", "out_of_scope", "later"), "rationale": text(), "priority": enumeration("must", "should", "could"), "revisit_trigger": text(),
	}),
	KindConstraint: schema([]string{"category", "statement", "hard", "rationale"}, map[string]fieldRule{
		"category": enumeration("time", "budget", "technology", "compatibility", "legal", "organizational", "operational", "physical"), "statement": text(), "hard": boolean(), "rationale": text(),
	}),
	KindAssumption: schema([]string{"statement", "impact_if_false", "validation_method", "owner"}, map[string]fieldRule{
		"statement": text(), "impact_if_false": enumeration("low", "medium", "high", "critical"), "validation_method": text(), "expires_at": text(), "owner": text(),
	}),
	KindQuestion: schema([]string{"prompt", "reason", "answer_type", "impact", "uncertainty", "irreversibility", "blocking"}, map[string]fieldRule{
		"prompt": text(), "reason": text(), "answer_type": text(), "options": array(), "impact": number(1, 5), "uncertainty": number(1, 5), "irreversibility": number(1, 5), "blocking": boolean(), "answer": anyField(), "disposition": enumeration("open", "answered", "deferred", "rejected"),
	}),
	KindTerm: schema([]string{"name", "definition", "aliases", "scope"}, map[string]fieldRule{
		"name": text(), "definition": text(), "aliases": array(), "scope": text(), "source_ref": text(),
	}),
	KindScenario: schema([]string{"actor", "trigger", "preconditions", "main_flow", "alternative_flows", "failure_flows", "postconditions", "importance"}, map[string]fieldRule{
		"actor": text(), "trigger": text(), "preconditions": array(), "main_flow": array(), "alternative_flows": array(), "failure_flows": array(), "postconditions": array(), "frequency": text(), "importance": anyField(),
	}),
	KindRequirement: schema([]string{"statement", "category", "rationale", "acceptance_conditions", "priority", "stability"}, map[string]fieldRule{
		"statement": text(), "category": enumeration("functional", "constraint", "interface", "data", "operational"), "rationale": text(), "acceptance_conditions": array(), "priority": enumeration("must", "should", "could"), "stability": enumeration("stable", "evolving", "volatile"),
	}),
	KindQualityScenario: schema([]string{"characteristic", "source", "stimulus", "environment", "artifact", "response", "measure"}, map[string]fieldRule{
		"characteristic": text(), "source": text(), "stimulus": text(), "environment": text(), "artifact": text(), "response": text(), "measure": text(),
	}),
	KindRisk: schema([]string{"category", "cause", "event", "impact", "likelihood", "severity", "mitigation", "evidence_needed", "residual_risk"}, map[string]fieldRule{
		"category": enumeration("value", "usability", "feasibility", "architecture", "security", "privacy", "delivery", "operational", "compliance"), "cause": text(), "event": text(), "impact": text(), "likelihood": anyField(), "severity": anyField(), "mitigation": text(), "evidence_needed": anyField(), "residual_risk": anyField(),
	}),
	KindOption: schema([]string{"decision_topic", "description", "benefits", "costs", "risks", "fit_to_constraints", "evidence_refs"}, map[string]fieldRule{
		"decision_topic": text(), "description": text(), "benefits": array(), "costs": array(), "risks": array(), "fit_to_constraints": anyField(), "evidence_refs": array(),
	}),
	KindDecision: schema([]string{"question", "selected_option_id", "rationale", "consequences", "alternatives_considered", "revisit_triggers", "significance", "approval_required"}, map[string]fieldRule{
		"question": text(), "selected_option_id": text(), "rationale": text(), "consequences": array(), "alternatives_considered": array(), "revisit_triggers": array(), "significance": enumeration("local", "cross_cutting", "architectural"), "approval_required": boolean(),
	}),
	KindSystemElement: schema([]string{"element_type", "responsibilities", "boundary", "lifecycle"}, map[string]fieldRule{
		"element_type": enumeration("person", "software_system", "container", "component", "interface", "datastore", "external_system"), "responsibilities": array(), "boundary": text(), "technology": text(), "lifecycle": text(), "trust_zone": text(),
	}),
	KindWorkSlice: schema([]string{"outcome", "included", "excluded", "dependencies", "verification_refs", "risk_reduction", "completion_conditions", "order_hint"}, map[string]fieldRule{
		"outcome": text(), "included": array(), "excluded": array(), "dependencies": array(), "verification_refs": array(), "risk_reduction": text(), "completion_conditions": array(), "order_hint": number(0, math.MaxFloat64),
	}),
	KindExperiment: schema([]string{"hypothesis", "decision_criteria", "method", "inputs", "time_box", "safety_constraints", "expected_evidence", "result_evidence_refs"}, map[string]fieldRule{
		"hypothesis": text(), "decision_criteria": anyField(), "method": text(), "inputs": array(), "time_box": text(), "safety_constraints": array(), "expected_evidence": array(), "result_evidence_refs": array(), "conclusion": text(),
	}),
	KindEvidence: schema([]string{"evidence_type", "summary", "locator", "captured_at", "freshness", "trust_notes"}, map[string]fieldRule{
		"evidence_type": enumeration("user_statement", "repository_fact", "external_source", "experiment_result", "measurement"), "summary": text(), "locator": text(), "captured_at": text(), "freshness": enumeration("current", "potentially_stale", "stale"), "trust_notes": text(),
	}),
	KindVerification: schema([]string{"target_ref", "method", "procedure", "expected_result", "environment", "owner"}, map[string]fieldRule{
		"target_ref": text(), "method": enumeration("test", "review", "analysis", "demonstration", "measurement", "experiment"), "procedure": text(), "expected_result": text(), "environment": text(), "owner": text(),
	}),
}

func ValidateProject(project Project) error {
	if strings.TrimSpace(project.ID) == "" || strings.TrimSpace(project.Name) == "" || strings.TrimSpace(project.RawRequest) == "" || strings.TrimSpace(project.OutputLanguage) == "" {
		return fmt.Errorf("%w: project id, name, raw_request, and output_language are required", ErrInvalid)
	}
	if !contains(projectModes, string(project.Mode)) {
		return fmt.Errorf("%w: unsupported project mode %q", ErrInvalid, project.Mode)
	}
	if !contains(projectStages, string(project.Stage)) {
		return fmt.Errorf("%w: unsupported project stage %q", ErrInvalid, project.Stage)
	}
	if !contains(projectStatuses, string(project.Status)) {
		return fmt.Errorf("%w: unsupported project status %q", ErrInvalid, project.Status)
	}
	if project.Revision < 1 {
		return fmt.Errorf("%w: project revision must be positive", ErrInvalid)
	}
	return nil
}

func ValidateEntity(entity Entity) error {
	if strings.TrimSpace(entity.ID) == "" || strings.TrimSpace(entity.ProjectID) == "" || strings.TrimSpace(entity.Title) == "" {
		return fmt.Errorf("%w: entity id, project_id, and title are required", ErrInvalid)
	}
	if !contains(entityStatuses, string(entity.Status)) {
		return fmt.Errorf("%w: unsupported entity status %q", ErrInvalid, entity.Status)
	}
	if !contains(origins, string(entity.Origin)) {
		return fmt.Errorf("%w: unsupported origin %q", ErrInvalid, entity.Origin)
	}
	if !contains(freshnessValues, string(entity.Freshness)) {
		return fmt.Errorf("%w: unsupported freshness %q", ErrInvalid, entity.Freshness)
	}
	if entity.Confidence < 0 || entity.Confidence > 1 || math.IsNaN(entity.Confidence) {
		return fmt.Errorf("%w: confidence must be between 0 and 1", ErrInvalid)
	}
	if entity.Revision < 1 {
		return fmt.Errorf("%w: entity revision must be positive", ErrInvalid)
	}
	if err := ValidateEntityBody(entity.Kind, entity.Body); err != nil {
		return err
	}
	return nil
}

func ValidateEntityBody(kind EntityKind, body json.RawMessage) error {
	schema, ok := entitySchemas[kind]
	if !ok {
		return fmt.Errorf("%w: unsupported entity kind %q", ErrInvalid, kind)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var fields map[string]any
	if err := decoder.Decode(&fields); err != nil {
		return fmt.Errorf("%w: body must be a JSON object: %v", ErrInvalid, err)
	}
	if fields == nil {
		return fmt.Errorf("%w: body must be a JSON object", ErrInvalid)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("%w: body must contain one JSON object", ErrInvalid)
	}

	for required := range schema.required {
		value, exists := fields[required]
		if !exists || value == nil {
			return fmt.Errorf("%w: %s body requires field %q", ErrInvalid, kind, required)
		}
	}
	for name, value := range fields {
		rule, allowed := schema.fields[name]
		if !allowed {
			return fmt.Errorf("%w: %s body contains unknown field %q", ErrInvalid, kind, name)
		}
		if value == nil {
			continue
		}
		if err := validateField(name, value, rule); err != nil {
			return err
		}
	}
	return nil
}

func validateField(name string, value any, rule fieldRule) error {
	valid := true
	switch rule.typeOf {
	case fieldString:
		_, valid = value.(string)
	case fieldBool:
		_, valid = value.(bool)
	case fieldNumber:
		number, ok := value.(json.Number)
		valid = ok
		if ok {
			parsed, err := number.Float64()
			valid = err == nil && parsed >= rule.min && parsed <= rule.max
		}
	case fieldArray:
		_, valid = value.([]any)
	case fieldObject:
		_, valid = value.(map[string]any)
	case fieldAny:
		valid = true
	}
	if !valid {
		return fmt.Errorf("%w: field %q has an invalid type or range", ErrInvalid, name)
	}
	if len(rule.enum) > 0 {
		valueString := value.(string)
		if !contains(rule.enum, valueString) {
			return fmt.Errorf("%w: field %q has unsupported value %q", ErrInvalid, name, valueString)
		}
	}
	return nil
}

func values(items ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(items))
	for _, item := range items {
		result[item] = struct{}{}
	}
	return result
}

func contains(set map[string]struct{}, value string) bool {
	_, ok := set[value]
	return ok
}
