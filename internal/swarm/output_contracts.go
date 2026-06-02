package swarm

import (
	"encoding/json"
	"errors"
	"fmt"
)

func ValidateStructuredOutput(output StructuredOutput) error {
	var errs []error
	for i, proposal := range output.Proposals {
		if !isAllowedProposalType(proposal.Type) {
			errs = append(errs, fmt.Errorf("proposal %d has unsupported type: %s", i, proposal.Type))
		}
		if len(proposal.Payload) == 0 {
			errs = append(errs, fmt.Errorf("proposal %d payload is required", i))
			continue
		}
		if !json.Valid(proposal.Payload) {
			errs = append(errs, fmt.Errorf("proposal %d payload must be valid JSON", i))
		}
	}
	return errors.Join(errs...)
}

func isAllowedProposalType(proposalType ProposalType) bool {
	switch proposalType {
	case ProposalFact, ProposalIntent, ProposalHint, ProposalFinding, ProposalMutation:
		return true
	default:
		return false
	}
}
