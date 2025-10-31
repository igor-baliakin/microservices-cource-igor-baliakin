package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	orderV1 "github.com/igor-baliakin/microservices-cource-igor-baliakin/shared/pkg/openapi/order/v1"
	inventoryV1 "github.com/igor-baliakin/microservices-cource-igor-baliakin/shared/pkg/proto/inventory/v1"
	paymentV1 "github.com/igor-baliakin/microservices-cource-igor-baliakin/shared/pkg/proto/payment/v1"
)

const (
	httpPort                = ":8080"
	httpServerReadTimeout   = 5 * time.Second
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
	TransactionUUID *string
	PaymentMethod   *string
	Status          string
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

func (s *orderServer) CreateOrder(ctx context.Context, req *orderV1.CreateOrderRequest) (orderV1.CreateOrderRes, error) {
	partUuids := make([]string, 0, len(req.PartUuids))
	for _, partUuid := range req.PartUuids {
		partUuids = append(partUuids, partUuid.String())
	}

	res, err := s.inventoryV1Client.ListParts(ctx, &inventoryV1.ListPartsRequest{
		Filter: &inventoryV1.PartFilter{
			Uuids: partUuids,
		},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return &orderV1.ServiceUnavailableError{
				Message: "Inventory service timeout",
			}, nil
		}
		return &orderV1.InternalServerError{
			Message: fmt.Sprintf("Inventory service error: %s", err.Error()),
		}, nil
	}

	if len(res.Parts) != len(req.PartUuids) {
		return &orderV1.BadRequestError{
			Message: "some parts not found",
		}, nil
	}

	var totalPrice float64
	for _, part := range res.Parts {
		totalPrice += part.Price
	}

	newOrderUUID := uuid.NewString()

	order := &orderData{
		UUID:       newOrderUUID,
		UserUUID:   req.UserUUID.String(),
		PartUUIDs:  partUuids,
		TotalPrice: totalPrice,
		CreatedAt:  time.Now(),
		Status:     orderStatusPendingPayment,
	}

	s.mu.Lock()
	s.orders[newOrderUUID] = order
	s.mu.Unlock()

	log.Printf(`
💳 [Order Created]
• 🆔 Order UUID: %s
• 👤 User UUID: %s
• 💰 Part UUID: %v
• 💰 Total Price: %f
• 💰 Status: %s
• 💰 Created At: %v
`, order.UUID, order.UserUUID, order.PartUUIDs, order.TotalPrice, order.Status, order.CreatedAt,
	)

	return &orderV1.CreateOrderResponse{
		Order: orderV1.OrderDto{
			UUID:       uuid.MustParse(newOrderUUID),
			UserUUID:   req.UserUUID,
			PartUuids:  req.PartUuids,
			TotalPrice: order.TotalPrice,
			Status:     orderV1.OrderStatus(order.Status),
			CreatedAt:  order.CreatedAt,
		},
	}, nil
}

func (s *orderServer) PayOrder(ctx context.Context, req *orderV1.PayOrderRequest, params orderV1.PayOrderParams) (orderV1.PayOrderRes, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	order, ok := s.orders[params.OrderUUID.String()]
	if !ok {
		return &orderV1.NotFoundError{
			Message: fmt.Sprintf("order with UUID %s not found", params.OrderUUID.String()),
		}, nil
	}

	if order.Status != orderStatusPendingPayment {
		return &orderV1.ConflictError{
			Message: "order cannot be paid",
		}, nil
	}

	res, err := s.paymentV1Client.PayOrder(ctx, &paymentV1.PayOrderRequest{
		OrderUuid:     order.UUID,
		UserUuid:      order.UserUUID,
		PaymentMethod: paymentV1.PaymentMethod(paymentV1.PaymentMethod_value[string(req.PaymentMethod)]),
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return &orderV1.ServiceUnavailableError{
				Message: "Payment service timeout",
			}, nil
		}
		return &orderV1.InternalServerError{
			Message: fmt.Sprintf("Payment service error: %s", err.Error()),
		}, nil
	}
	paymentMethod := string(req.PaymentMethod)
	order.TransactionUUID = &res.TransactionUuid
	order.PaymentMethod = &paymentMethod
	order.Status = orderStatusPaid

	log.Printf(`
✅ [Order Paid]
• 🆔 Order UUID: %s
• 👤 User UUID: %s
• 💳 Payment Method: %s
• 🆔 Transaction UUID: %s
• 💰 Status: %s
`, order.UUID, order.UserUUID, *order.PaymentMethod, *order.TransactionUUID, order.Status,
	)

	return &orderV1.PayOrderResponse{
		TransactionUUID: uuid.MustParse(res.TransactionUuid),
	}, nil
}

func (s *orderServer) GetOrderByUUID(ctx context.Context, params orderV1.GetOrderByUUIDParams) (orderV1.GetOrderByUUIDRes, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	order, ok := s.orders[params.OrderUUID.String()]
	if !ok {
		return &orderV1.NotFoundError{
			Message: fmt.Sprintf("order with UUID %s not found", params.OrderUUID.String()),
		}, nil
	}

	partUuids := make([]uuid.UUID, 0, len(order.PartUUIDs))
	for _, partUuid := range order.PartUUIDs {
		partUuids = append(partUuids, uuid.MustParse(partUuid))
	}

	var transactionUUID orderV1.OptNilUUID
	if order.TransactionUUID != nil {
		transactionUUID = orderV1.NewOptNilUUID(uuid.MustParse(*order.TransactionUUID))
	}

	var paymentMethod orderV1.OptPaymentMethod
	if order.PaymentMethod != nil {
		paymentMethod = orderV1.NewOptPaymentMethod(orderV1.PaymentMethod(*order.PaymentMethod))
	}

	return &orderV1.GetOrderResponse{
		Order: orderV1.OrderDto{
			UUID:            uuid.MustParse(order.UUID),
			UserUUID:        uuid.MustParse(order.UserUUID),
			PartUuids:       partUuids,
			TotalPrice:      order.TotalPrice,
			Status:          orderV1.OrderStatus(order.Status),
			CreatedAt:       order.CreatedAt,
			TransactionUUID: transactionUUID,
			PaymentMethod:   paymentMethod,
		},
	}, nil
}

func (s *orderServer) CancelOrderByUUID(_ context.Context, params orderV1.CancelOrderByUUIDParams) (orderV1.CancelOrderByUUIDRes, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	order, ok := s.orders[params.OrderUUID.String()]
	if !ok {
		return &orderV1.NotFoundError{
			Message: fmt.Sprintf("order with UUID %s not found", params.OrderUUID.String()),
		}, nil
	}

	if order.Status != orderStatusPendingPayment {
		return &orderV1.ConflictError{
			Message: "only pending payment orders can be cancelled",
		}, nil
	}

	order.Status = orderStatusCancelled

	log.Printf(`
❌ [Order Cancelled]
• 🆔 Order UUID: %s
• 👤 User UUID: %s
• 💰 Status: %s
`, order.UUID, order.UserUUID, order.Status,
	)

	return &orderV1.CancelOrderByUUIDNoContent{}, nil
}

func (s *orderServer) NewError(_ context.Context, err error) *orderV1.GenericErrorStatusCode {
	return &orderV1.GenericErrorStatusCode{
		StatusCode: http.StatusInternalServerError,
		Response: orderV1.GenericError{
			Message: err.Error(),
		},
	}
}

func main() {
	// Implementation of main function to start the server would go here
	inventoryV1Conn, err := grpc.NewClient(
		InventoryServiceAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Printf("failed to connect to inventory service: %v", err)
		return
	}
	defer func() {
		if err := inventoryV1Conn.Close(); err != nil {
			log.Printf("failed to close inventory service connection: %v", err)
		}
	}()

	paymentV1Conn, err := grpc.NewClient(
		PaymentServiceAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Printf("failed to connect to payment service: %v", err)
		return
	}
	defer func() {
		if err := paymentV1Conn.Close(); err != nil {
			log.Printf("failed to close payment service connection: %v", err)
		}
	}()

	inventoryV1Client := inventoryV1.NewInventoryServiceClient(inventoryV1Conn)
	paymentV1Client := paymentV1.NewPaymentServiceClient(paymentV1Conn)

	r := chi.NewRouter()
	srv := NewOrderServer(inventoryV1Client, paymentV1Client)

	handler, err := orderV1.NewServer(srv)
	if err != nil {
		log.Printf("failed to create order server: %v", err)
		return
	}
	r.Mount("/", handler)

	server := &http.Server{
		ReadTimeout: httpServerReadTimeout,
		Addr:        httpPort,
		Handler:     r,
	}

	go func() {
		log.Printf("🚀 Order HTTP server listening at %v", server.Addr)
		if errServer := server.ListenAndServe(); errServer != nil && !errors.Is(errServer, http.ErrServerClosed) {
			log.Printf("failed to start HTTP server: %v", errServer)
			return
		}
	}()
	// graceful shutdown logic would go here

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = server.Shutdown(ctx)
	if err != nil {
		log.Printf("failed to shutdown server: %v", err)
	}

	log.Println("Server gracefully stopped")
}
