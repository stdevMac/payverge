package structs

import "container/heap"

type OperationsPerUser struct {
	Address   string  `json:"address"`
	Amount    float64 `json:"points"`
	Referrals int     `json:"referrals"`
	index     int
}

type PriorityQueue []*OperationsPerUser

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	return pq[i].Amount > pq[j].Amount
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*OperationsPerUser)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*pq = old[0 : n-1]
	return item
}

type OperationSorter struct {
	pq PriorityQueue
	m  map[string]*OperationsPerUser
}

func NewOperationSorter() *OperationSorter {
	return &OperationSorter{
		pq: make(PriorityQueue, 0),
		m:  make(map[string]*OperationsPerUser),
	}
}

func (ds *OperationSorter) Update(address string, amount float64, referrals int) {
	if item, ok := ds.m[address]; ok {
		item.Amount += amount
		heap.Fix(&ds.pq, item.index)
	} else {
		item := &OperationsPerUser{
			Address:   address,
			Amount:    amount,
			Referrals: referrals,
		}
		heap.Push(&ds.pq, item)
		ds.m[address] = item
	}
}

func (ds *OperationSorter) TopN(n int) []*OperationsPerUser {
	result := make([]*OperationsPerUser, 0)
	for i := 0; i < n && i < len(ds.pq); i++ {
		result = append(result, ds.pq[i])
	}
	return result
}
