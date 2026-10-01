package workspace

// Runtime/Cookie release cannot discard a later or independent batch owner.
func (s *Service) releaseProfileUse(environmentID string) {
	if s.batchUses[environmentID] == nil {
		delete(s.profileUses, environmentID)
	}
}
func (s *Service) acquireBatchProfileUse(task *batchTask, environmentID string) bool {
	if s.profileUses[environmentID] || s.batchUses[environmentID] != nil || s.runtimeOwnsProfileUse(environmentID) {
		return false
	}
	s.batchUses[environmentID] = task
	s.profileUses[environmentID] = true
	return true
}
func (s *Service) releaseBatchProfileUse(task *batchTask, environmentID string) {
	if s.batchUses[environmentID] != task {
		return
	}
	delete(s.batchUses, environmentID)
	if !s.runtimeOwnsProfileUse(environmentID) && s.cookieTasks[environmentID] == nil {
		delete(s.profileUses, environmentID)
	}
}
