package domain

import "time"

type Side string

const (
	SideBuy  Side = "buy"
	SideSell Side = "sell"
)

type OrderType string

const (
	OrderTypeMarket OrderType = "market"
	OrderTypeLimit  OrderType = "limit"
)

type OrderStatus string

const (
	OrderStatusNew       OrderStatus = "new"
	OrderStatusAccepted  OrderStatus = "accepted"
	OrderStatusPartially OrderStatus = "partially_filled"
	OrderStatusFilled    OrderStatus = "filled"
	OrderStatusCanceled  OrderStatus = "canceled"
	OrderStatusRejected  OrderStatus = "rejected"
)

type Mode string

const (
	ModeManual Mode = "manual"
	ModeAssist Mode = "assist"
	ModeAuto   Mode = "auto"
)

type Account struct {
	Cash        float64
	BuyingPower float64
	Equity      float64
}

type Position struct {
	Symbol    string
	Qty       float64
	AvgCost   float64
	LastPrice float64
}

type OrderRequest struct {
	Symbol        string
	Side          Side
	Qty           float64
	Type          OrderType
	LimitPrice    *float64
	ClientOrderID string
}

type Order struct {
	ID         string
	Symbol     string
	Side       Side
	Qty        float64
	FilledQty  float64
	Type       OrderType
	LimitPrice *float64
	Status     OrderStatus
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Quote struct {
	Symbol string
	Bid    float64
	Ask    float64
	Last   float64
	Time   time.Time
}

type TradeUpdate struct {
	OrderID   string
	Status    OrderStatus
	FillQty   float64
	FillPrice *float64
	Time      time.Time
}

type TradeIntent struct {
	Symbol     string
	Side       Side
	Qty        float64
	OrderType  OrderType
	LimitPrice *float64
	Rationale  string
	Confidence float64
}

type Event struct {
	Time    time.Time
	Type    string
	Details string
}

type Snapshot struct {
	Account   Account
	Positions []Position
	Orders    []Order
	Events    []Event
}
