package goal

import (
	"container/heap"
	"errors"
	"sort"
	"sync"
)

// PriorityQueue implements a priority queue for GoalNode using container/heap.
// Higher priority values are dequeued first. Within the same priority, FIFO order is preserved.
type PriorityQueue struct {
	mu    sync.RWMutex
	items *priorityHeap
	index map[GoalID]int
	seq   int
}

type heapItem struct {
	goal     GoalNode
	priority Priority
	sequence int
	index    int
}

type priorityHeap []*heapItem

func (h priorityHeap) Len() int { return len(h) }

func (h priorityHeap) Less(i, j int) bool {
	if h[i].priority != h[j].priority {
		return h[i].priority > h[j].priority
	}
	return h[i].sequence < h[j].sequence
}

func (h priorityHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *priorityHeap) Push(x interface{}) {
	n := len(*h)
	item := x.(*heapItem)
	item.index = n
	*h = append(*h, item)
}

func (h *priorityHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*h = old[0 : n-1]
	return item
}

func NewPriorityQueue() *PriorityQueue {
	h := &priorityHeap{}
	heap.Init(h)
	return &PriorityQueue{
		items: h,
		index: make(map[GoalID]int),
	}
}

func (pq *PriorityQueue) Enqueue(goal GoalNode) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if goal.Priority == 0 {
		goal.Priority = PriorityMedium
	}

	item := &heapItem{
		goal:     goal,
		priority: goal.Priority,
		sequence: pq.seq,
	}
	pq.seq++

	heap.Push(pq.items, item)
	// Update index mapping after Push completes (which sets item.index)
	pq.index[goal.ID] = item.index
}

func (pq *PriorityQueue) Dequeue() (GoalNode, bool) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if pq.items.Len() == 0 {
		return GoalNode{}, false
	}

	item := heap.Pop(pq.items).(*heapItem)
	delete(pq.index, item.goal.ID)
	return item.goal, true
}

func (pq *PriorityQueue) Peek() (GoalNode, bool) {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	if pq.items.Len() == 0 {
		return GoalNode{}, false
	}

	return (*pq.items)[0].goal, true
}

func (pq *PriorityQueue) Snapshot() []GoalNode {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	items := append([]*heapItem(nil), (*pq.items)...)
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].priority != items[j].priority {
			return items[i].priority > items[j].priority
		}
		return items[i].sequence < items[j].sequence
	})

	goals := make([]GoalNode, 0, len(items))
	for _, item := range items {
		goals = append(goals, item.goal)
	}
	return goals
}

func (pq *PriorityQueue) Update(id GoalID, updateFn func(*GoalNode)) error {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	// Find the item by ID in the heap
	foundIndex := -1
	for i, item := range *pq.items {
		if item.goal.ID == id {
			foundIndex = i
			break
		}
	}

	if foundIndex == -1 {
		return errors.New("goal not found in queue")
	}

	// Update the item in place
	item := (*pq.items)[foundIndex]
	updateFn(&item.goal)
	item.priority = item.goal.Priority
	(*pq.items)[foundIndex] = item

	heap.Fix(pq.items, foundIndex)
	pq.index[id] = item.index

	return nil
}

func (pq *PriorityQueue) Len() int {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	return pq.items.Len()
}
