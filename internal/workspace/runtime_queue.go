package workspace

// Admission order is the durable Runtime.Start acceptance order. Only costly
// startup is serialized; a running browser consumes no queue slot or quota.
// Cancelled waiters remove only their own ticket, never wake a successor past
// another environment that still owns the active startup slot.
func (s *Service) releaseRuntimeTurn(turn chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index, queued := range s.startQueue {
		if queued != turn {
			continue
		}
		s.startQueue = append(s.startQueue[:index], s.startQueue[index+1:]...)
		if index == 0 && len(s.startQueue) > 0 {
			close(s.startQueue[0])
		}
		return
	}
}
