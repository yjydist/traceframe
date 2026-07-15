package domain

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestValidateEntityBody(t *testing.T) {
	tests := []struct {
		name    string
		kind    EntityKind
		body    string
		wantErr bool
	}{
		{
			name: "valid goal",
			kind: KindGoal,
			body: `{"outcome":"Students miss fewer deadlines","success_signals":["late work decreases"],"priority":"must"}`,
		},
		{
			name:    "missing required field",
			kind:    KindGoal,
			body:    `{"outcome":"Students miss fewer deadlines","priority":"must"}`,
			wantErr: true,
		},
		{
			name:    "unknown field",
			kind:    KindGoal,
			body:    `{"outcome":"Students miss fewer deadlines","success_signals":[],"priority":"must","feature":"calendar"}`,
			wantErr: true,
		},
		{
			name:    "invalid enum",
			kind:    KindRequirement,
			body:    `{"statement":"Store work","category":"functional","rationale":"Needed","acceptance_conditions":[],"priority":"urgent","stability":"stable"}`,
			wantErr: true,
		},
		{
			name:    "question factor out of range",
			kind:    KindQuestion,
			body:    `{"prompt":"Who uses it?","reason":"Changes scope","answer_type":"text","impact":6,"uncertainty":2,"irreversibility":1,"blocking":true}`,
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateEntityBody(test.kind, json.RawMessage(test.body))
			if (err != nil) != test.wantErr {
				t.Fatalf("ValidateEntityBody() error = %v, wantErr %v", err, test.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestValidateRelation(t *testing.T) {
	projectID := "prj_1"
	verification := Entity{ID: "ver_1", ProjectID: projectID, Kind: KindVerification}
	requirement := Entity{ID: "req_1", ProjectID: projectID, Kind: KindRequirement}
	evidence := Entity{ID: "evidence_1", ProjectID: projectID, Kind: KindEvidence}

	valid := Relation{ID: "rel_1", ProjectID: projectID, FromID: verification.ID, Type: RelationVerifies, ToID: requirement.ID}
	if err := ValidateRelation(valid, verification, requirement); err != nil {
		t.Fatalf("ValidateRelation() error = %v", err)
	}

	invalid := Relation{ID: "rel_2", ProjectID: projectID, FromID: verification.ID, Type: RelationVerifies, ToID: evidence.ID}
	if err := ValidateRelation(invalid, verification, evidence); err == nil {
		t.Fatal("ValidateRelation() expected incompatible relation error")
	}
}
