package goal

import "testing"

func TestPriorityQueueSnapshotReturnsOrderedGoalsWithoutMutation(t *testing.T) {
	queue := NewPriorityQueue()
	queue.Enqueue(GoalNode{ID: "medium-first", Priority: PriorityMedium})
	queue.Enqueue(GoalNode{ID: "high", Priority: PriorityHigh})
	queue.Enqueue(GoalNode{ID: "medium-second", Priority: PriorityMedium})
	queue.Enqueue(GoalNode{ID: "critical", Priority: PriorityCritical})

	snapshot := queue.Snapshot()
	want := []GoalID{"critical", "high", "medium-first", "medium-second"}
	if len(snapshot) != len(want) {
		t.Fatalf("expected %d goals, got %#v", len(want), snapshot)
	}
	for i, id := range want {
		if snapshot[i].ID != id {
			t.Fatalf("snapshot[%d] = %s, want %s in %#v", i, snapshot[i].ID, id, snapshot)
		}
	}
	if queue.Len() != len(want) {
		t.Fatalf("snapshot should not mutate queue length, got %d", queue.Len())
	}

	next, ok := queue.Dequeue()
	if !ok || next.ID != "critical" {
		t.Fatalf("snapshot should not change dequeue order, got %#v ok=%v", next, ok)
	}
}
