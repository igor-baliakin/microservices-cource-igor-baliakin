package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	payment_v1 "github.com/igor-baliakin/microservices-cource-igor-baliakin/shared/pkg/proto/payment/v1"
)

const grpcAddr = "localhost:50052"

type PaymentServer struct {
	payment_v1.UnimplementedPaymentServiceServer
}

func NewPaymentServer() *PaymentServer {
	return &PaymentServer{}
}

func (s *PaymentServer) PayOrder(_ context.Context, req *payment_v1.PayOrderRequest) (*payment_v1.PayOrderResponse, error) {
	// Here would be the logic to process the payment
	log.Printf(`
💳 [Order Paid]
• 🆔 Order UUID: %s
• 👤 User UUID: %s
• 💰 Payment Method: %s
`, req.OrderUuid, req.UserUuid, req.PaymentMethod.String(),
	)
	transactionsUUID := uuid.NewString()

	return &payment_v1.PayOrderResponse{
		TransactionUuid: transactionsUUID,
	}, nil
}

func main() {
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
		return
	}

	defer func() {
		if err := lis.Close(); err != nil {
			log.Fatalf("failed to close listener: %v", err)
		}
	}()

	s := grpc.NewServer()
	reflection.Register(s)
	payment_v1.RegisterPaymentServiceServer(s, NewPaymentServer())

	go func() {
		log.Printf("🚀 Payment gRPC server listening at %v", lis.Addr())
		err := s.Serve(lis)
		if err != nil {
			log.Fatalf("failed to serve: %v", err)
			return
		}
	}()

	// Gracefully stop the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("🛑 Shutting down Payment gRPC server...")
	s.GracefulStop()
	log.Println("🛑 Payment gRPC server stopped")
}
