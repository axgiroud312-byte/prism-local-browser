package workspace

import (
	"context"
	"errors"

	"github.com/axgiroud312-byte/prism-local-browser/internal/kernel"
	"github.com/axgiroud312-byte/prism-local-browser/internal/proxy"
)

func (s *Service) prepareMigrationNetwork(ctx context.Context, task *migrationTask, record kernel.Record, input kernel.ManagedProfile) (*kernel.ProtectedProxy, error) {
	if s.networkStore == nil {
		return nil, kernel.RequireProxyNetworkBoundary()
	}
	s.mu.Lock()
	bound, ref, err := s.savedProxy(task.plan.Environment.ProxyID)
	var protected []byte
	if err == nil {
		protected, err = s.readProtectedProxyCredentials(ref)
	}
	s.mu.Unlock()
	if err != nil {
		return nil, errors.New("migration proxy configuration unavailable")
	}
	credentials, err := s.decodeProtectedProxyCredentials(ref, protected)
	if credentials != nil {
		defer func() { credentials.Username, credentials.Password = "", "" }()
	}
	if err != nil {
		return nil, &proxy.CheckError{Code: "CREDENTIALS_UNAVAILABLE", Message: "不能读取迁移环境绑定的代理凭据，未直连试用。", Retryable: true}
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	channel, err := s.networkStore.OpenProtectedProxy(ctx, record, input, id(), bound.Configuration, credentials, s.options.ProtectedProxyCheck)
	if err != nil {
		return channel, err
	}
	report := channel.Preflight(ctx, nil)
	report.ProxyID, report.Revision = bound.ID, bound.Revision
	if err = s.migrationUpdate(task, "trial-starting", func(_ *migrationPlan, r *MigrationReport) { r.ProxyReport = &report }); err != nil {
		return channel, err
	}
	if report.Error != nil {
		return channel, report.Error
	}
	return channel, ctx.Err()
}

func (s *Service) recoverMigrationNetwork(ctx context.Context, plan migrationPlan) error {
	if s.networkStore == nil {
		if plan.Environment.ProxyID != "" {
			return kernel.RequireProxyNetworkBoundary()
		}
		return nil
	}
	pending, err := s.networkStore.Pending(ctx)
	if err != nil {
		return err
	}
	intent, exists := pending[plan.WorkID]
	if !exists {
		return nil
	}
	if intent.SessionID != plan.SessionID || intent.DataReference != plan.Move.Incoming {
		return errors.New("migration network resource identity differs")
	}
	if err = s.networkStore.Recover(ctx, intent); err != nil {
		return err
	}
	s.mu.Lock()
	if s.networkPending[plan.WorkID] == intent {
		delete(s.networkPending, plan.WorkID)
		s.releaseProfileUse(plan.WorkID)
	}
	s.mu.Unlock()
	return nil
}
