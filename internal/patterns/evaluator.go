package patterns

import (
	"fmt"
)

type ConditionResults map[string]bool

type Transition struct {
	From      string
	To        string
	Condition string
	Terminal  bool
}

func Next(pattern Pattern, currentNode string, results ConditionResults) (Transition, error) {
	if err := Validate(pattern); err != nil {
		return Transition{}, err
	}

	nodes := map[string]Node{}
	for _, node := range pattern.Nodes {
		nodes[node.ID] = node
	}
	current, ok := nodes[currentNode]
	if !ok {
		return Transition{}, fmt.Errorf("current node not found: %s", currentNode)
	}
	if current.Terminal {
		return Transition{From: currentNode, To: currentNode, Terminal: true}, nil
	}

	for _, edge := range pattern.Edges {
		if edge.From != currentNode {
			continue
		}
		if conditionMet(edge.Condition, results) {
			target := nodes[edge.To]
			return Transition{
				From:      edge.From,
				To:        edge.To,
				Condition: edge.Condition,
				Terminal:  target.Terminal,
			}, nil
		}
	}

	return Transition{}, fmt.Errorf("no transition matched from node %s", currentNode)
}

func conditionMet(condition string, results ConditionResults) bool {
	if condition == "" || condition == "always" {
		return true
	}
	return results[condition]
}
