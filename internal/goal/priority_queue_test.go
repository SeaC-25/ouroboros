package goal

import "testing"

func TestPriorityQueuePopsHighestPriorityAndPreservesInsertionOrder(t *testing.T) {
	queue := NewPriorityQueue()

	queue.Enqueue(GoalNode{ID: "medium-first", Priority: PriorityMedium})
	queue.Enqueue(GoalNode{ID: "high", Priority: PriorityHigh})
	queue.Enqueue(GoalNode{ID: "medium-second", Priority: PriorityMedium})
	queue.Enqueue(GoalNode{ID: "critical", Priority: PriorityCritical})

	assertNextGoal(t, queue, "critical")
	assertNextGoal(t, queue, "high")
	assertNextGoal(t, queue, "medium-first")
	assertNextGoal(t, queue, "medium-second")

	if _, ok := queue.Dequeue(); ok {
		t.Fatalf("expected empty queue")
	}
}

func TestPriorityQueueUpdateReordersExistingGoal(t *testing.T) {
	queue := NewPriorityQueue()

	queue.Enqueue(GoalNode{ID: "slow", Priority: PriorityLow})
	queue.Enqueue(GoalNode{ID: "steady", Priority: PriorityMedium})

	if err := queue.Update("slow", func(goal *GoalNode) {
		goal.Priority = PriorityCritical
		goal.Title = "escalated"
	}); err != nil {
		t.Fatalf("update priority: %v", err)
	}

	got, ok := queue.Peek()
	if !ok {
		t.Fatalf("expected a queued goal")
	}
	if got.ID != "slow" || got.Title != "escalated" {
		t.Fatalf("expected updated goal first, got %+v", got)
	}
}

func assertNextGoal(t *testing.T, queue *PriorityQueue, want GoalID) {
	t.Helper()

	got, ok := queue.Dequeue()
	if !ok {
		t.Fatalf("expected goal %q, queue was empty", want)
	}
	if got.ID != want {
		t.Fatalf("expected goal %q, got %q", want, got.ID)
	}
}
