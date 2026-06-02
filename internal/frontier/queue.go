package frontier

import "container/heap"

type Queue struct {
	items priorityQueue
}

func NewQueue() *Queue {
	q := &Queue{}
	heap.Init(&q.items)
	return q
}

func (q *Queue) Push(path CandidatePath) {
	heap.Push(&q.items, queueItem{
		path:  path,
		score: Score(path),
	})
}

func (q *Queue) Pop() (CandidatePath, bool) {
	if q.items.Len() == 0 {
		return CandidatePath{}, false
	}
	item := heap.Pop(&q.items).(queueItem)
	return item.path, true
}

func (q *Queue) Len() int {
	return q.items.Len()
}

type queueItem struct {
	path  CandidatePath
	score float64
	index int
}

type priorityQueue []queueItem

func (q priorityQueue) Len() int {
	return len(q)
}

func (q priorityQueue) Less(i int, j int) bool {
	return q[i].score > q[j].score
}

func (q priorityQueue) Swap(i int, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].index = i
	q[j].index = j
}

func (q *priorityQueue) Push(x any) {
	item := x.(queueItem)
	item.index = len(*q)
	*q = append(*q, item)
}

func (q *priorityQueue) Pop() any {
	old := *q
	n := len(old)
	item := old[n-1]
	old[n-1] = queueItem{}
	item.index = -1
	*q = old[:n-1]
	return item
}
