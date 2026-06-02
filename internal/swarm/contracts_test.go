package swarm

import (
	"encoding/json"
	"testing"
)

func TestAgentRolesIncludeRequiredRoles(t *testing.T) {
	required := []AgentRole{
		RolePlanner,
		RoleWebAPI,
		RoleInfra,
		RoleCTF,
		RoleVerifier,
		RoleEvidence,
		RoleReporter,
		RoleContext,
		RoleGraph,
	}

	known := KnownRoles()
	for _, role := range required {
		if !known[role] {
			t.Fatalf("expected role %s", role)
		}
	}
}

func TestValidateStructuredOutputAcceptsAllowedProposalTypes(t *testing.T) {
	output := StructuredOutput{
		Proposals: []Proposal{
			{Type: ProposalFact, Payload: json.RawMessage(`{"statement":"target observed"}`)},
			{Type: ProposalIntent, Payload: json.RawMessage(`{"goal":"enumerate routes"}`)},
			{Type: ProposalHint, Payload: json.RawMessage(`{"message":"prioritize auth"}`)},
			{Type: ProposalFinding, Payload: json.RawMessage(`{"title":"access control candidate"}`)},
			{Type: ProposalMutation, Payload: json.RawMessage(`{"kind":"reduce_scope"}`)},
		},
	}

	if err := ValidateStructuredOutput(output); err != nil {
		t.Fatalf("expected valid output: %v", err)
	}
}

func TestValidateStructuredOutputRejectsMalformedOutput(t *testing.T) {
	output := StructuredOutput{
		Proposals: []Proposal{
			{Type: ProposalType("shell_command"), Payload: json.RawMessage(`{"cmd":"nmap target"}`)},
		},
	}

	if err := ValidateStructuredOutput(output); err == nil {
		t.Fatalf("expected unknown proposal type to fail")
	}
}

func TestValidateStructuredOutputRejectsInvalidJSONPayload(t *testing.T) {
	output := StructuredOutput{
		Proposals: []Proposal{
			{Type: ProposalFact, Payload: json.RawMessage(`{invalid-json`)},
		},
	}

	if err := ValidateStructuredOutput(output); err == nil {
		t.Fatalf("expected invalid JSON payload to fail")
	}
}
