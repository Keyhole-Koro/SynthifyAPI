package service

import (
	"context"
	"fmt"

	"github.com/synthify/backend/packages/shared/domain"
	"github.com/synthify/backend/packages/shared/repository"
)

type BillingProvider interface {
	CreateCheckoutSession(ctx context.Context, account *domain.Account) (string, error)
	CreatePortalSession(ctx context.Context, account *domain.Account) (string, error)
}

type BillingService struct {
	accounts repository.AccountRepository
	provider BillingProvider
}

func NewBillingService(accounts repository.AccountRepository, provider BillingProvider) *BillingService {
	return &BillingService{
		accounts: accounts,
		provider: provider,
	}
}

func (s *BillingService) CreateCheckoutSession(ctx context.Context, accountID, userID string) (string, error) {
	account, err := s.authorizeAccount(ctx, accountID, userID)
	if err != nil {
		return "", err
	}
	if s.provider == nil {
		return "", fmt.Errorf("%w: billing provider is not configured", domain.ErrNotImplemented)
	}
	return s.provider.CreateCheckoutSession(ctx, account)
}

func (s *BillingService) CreatePortalSession(ctx context.Context, accountID, userID string) (string, error) {
	account, err := s.authorizeAccount(ctx, accountID, userID)
	if err != nil {
		return "", err
	}
	if s.provider == nil {
		return "", fmt.Errorf("%w: billing provider is not configured", domain.ErrNotImplemented)
	}
	return s.provider.CreatePortalSession(ctx, account)
}

func (s *BillingService) authorizeAccount(ctx context.Context, accountID, userID string) (*domain.Account, error) {
	if !s.accounts.IsAccountAccessible(ctx, accountID, userID) {
		return nil, domain.ErrNotFound
	}
	return s.accounts.GetAccount(ctx, accountID)
}
