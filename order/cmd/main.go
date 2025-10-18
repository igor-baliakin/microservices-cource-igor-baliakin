package main

import (
	"context"
	"sync"
	"time"

	orderV1 "github.com/igor-baliakin/microservices-cource-igor-baliakin/shared/pkg/openapi/order/v1"
	inventoryV1 "github.com/igor-baliakin/microservices-cource-igor-baliakin/shared/pkg/proto/inventory/v1"
	paymentV1 "github.com/igor-baliakin/microservices-cource-igor-baliakin/shared/pkg/proto/payment/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	httpPort           = "8080"
	httpsServerTimeout = 5 * time.Second

	InventoryServiceAddress = "localhost:50051"
	PaymentServiceAddress   = "localhost:50052"
)

const (
	orderStatusPendingPayment = "PENDING_PAYMENT"
	orderStatusPaid           = "PAID"
	orderStatusCancelled      = "CANCELLED"
)

type orderServer struct {
	mu     sync.RWMutex
	orders map[string]*orderData

	inventoryV1Client inventoryV1.InventoryServiceClient
	paymentV1Client   paymentV1.PaymentServiceClient
}

type orderData struct {
	UUID            string
	UserUUID        string
	PartUUIDs       []string
	TotalPrice      float64
	TransactionUUID string
	PaymentMethod   *string
	Status          *string
	CreatedAt       time.Time
}

func NewOrderServer(
	inventoryV1Client inventoryV1.InventoryServiceClient,
	paymentV1Client paymentV1.PaymentServiceClient,
) *orderServer {
	return &orderServer{
		orders:            make(map[string]*orderData),
		inventoryV1Client: inventoryV1Client,
		paymentV1Client:   paymentV1Client,
	}
}

func (s *orderServer) CreateOrder(ctx context.Context, req *orderV1.CreateOrderRequest) (*orderV1.CreateOrderResponse, error) {
	partaUuids := make([]string,0, len(req.PartUuids))
	for _, partUUID := range req.PartUuids {
		partaUuids = append(partaUuids, partUUID.String())
	}

	res, err := s.inventoryV1Client.ListParts(ctx, &inventoryV1.ListPartsRequest{
		Filter: &inventoryV1.PartFilter{
			Uuids: partaUuids,
		},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || error.Is(err, context.Canceled) {
			return &orderv1.ServiceUnavailableError{
			Message: "Inventory service timeout",

	    }, nil
    }
	    return &orderV1.InternalServerError{
			Message: fmt.Sprintf("Inventory service error: %s", err.Error()),
		}, nil
    }

	if len(res.Parts) != len(req.PartUuids){
		return &orderV1.BadRequestError{
			
		}
	}

	