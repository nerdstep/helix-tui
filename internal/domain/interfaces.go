package domain

import "context"

type Broker interface {
	GetAccount(ctx context.Context) (Account, error)
	GetPositions(ctx context.Context) ([]Position, error)
	GetOpenOrders(ctx context.Context) ([]Order, error)
	GetQuote(ctx context.Context, symbol string) (Quote, error)
	PlaceOrder(ctx context.Context, req OrderRequest) (Order, error)
	CancelOrder(ctx context.Context, orderID string) error
	StreamTradeUpdates(ctx context.Context) (<-chan TradeUpdate, error)
}

type Agent interface {
	ProposeTrades(ctx context.Context, input AgentInput) ([]TradeIntent, error)
}
