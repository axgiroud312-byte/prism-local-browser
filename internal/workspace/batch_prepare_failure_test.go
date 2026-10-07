package workspace

import "testing"

// This exercises the production wrapper, not Options.PrepareBatchDirectory.
// Invalid ownership is rejected before any profile directory is touched.
func TestBatchProductionPreparationFailureHasNoLease(t *testing.T) {
	s := &Service{}
	lease, err := s.prepareBatchDirectory(BatchDirectoryInput{Root: t.TempDir(), EnvironmentID: "invalid", PlanID: "invalid"})
	if err == nil || lease != nil {
		t.Fatal("failed production preparation must return a nil interface, not a typed-nil lease")
	}
}
