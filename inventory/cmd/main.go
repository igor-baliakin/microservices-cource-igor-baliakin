package main

import (
	"context"
	"log"
	"math"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/google/uuid"
	inventory_v1 "github.com/igor-baliakin/microservices-cource-igor-baliakin/shared/pkg/proto/inventory/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const grpcAddr = "localhost:50051"

type InventoryServer struct {
	inventory_v1.UnimplementedInventoryServiceServer
	parts map[string]*inventory_v1.Part
}

func NewInventoryServer() *InventoryServer {
	s := &InventoryServer{
		parts: make(map[string]*inventory_v1.Part),
	}
	s.initParts()
	return s
}

func (s *InventoryServer) initParts() {
	parts := generateParts()
	for _, part := range parts {
		s.parts[part.Uuid] = part
	}
}

func generateParts() []*inventory_v1.Part {
	names := []string{
		"Main Engine",
		"Reserve Engine",
		"Thruster",
		"Fuel Tank",
		"Left Wing",
		"Right Wing",
		"Window A",
		"Window B",
		"Control Module",
		"Stabilizer",
	}

	descriptions := []string{
		"Primary propulsion unit",
		"Backup propulsion unit",
		"Thruster for fine adjustments",
		"Main fuel tank",
		"Left aerodynamic wing",
		"Right aerodynamic wing",
		"Front viewing window",
		"Side viewing window",
		"Flight control module",
		"Stabilization fin",
	}

	var parts []*inventory_v1.Part
	for i := 0; i < gofakeit.Number(1, 50); i++ {
		idx := gofakeit.Number(0, len(names)-1)
		parts = append(parts, &inventory_v1.Part{
			Uuid:          uuid.NewString(),
			Name:          names[idx],
			Description:   descriptions[idx],
			Price:         roundTo(gofakeit.Float64Range(100, 10_000)),
			StockQuantity: int64(gofakeit.Number(1, 100)),
			Category:      inventory_v1.Category(gofakeit.Number(1, 4)), //nolint:gosec // safe: gofakeit.Number returns 1..4
			Dimensions:    generateDimensions(),
			Manufacturer:  generateManufacturer(),
			Tags:          generateTags(),
			Metadata:      generateMetadata(),
			CreatedAt:     timestamppb.Now(),
		})
	}

	return parts
}

func generateDimensions() *inventory_v1.Dimensions {
	return &inventory_v1.Dimensions{
		Length: roundTo(gofakeit.Float64Range(1, 1000)),
		Width:  roundTo(gofakeit.Float64Range(1, 1000)),
		Height: roundTo(gofakeit.Float64Range(1, 1000)),
		Weight: roundTo(gofakeit.Float64Range(1, 1000)),
	}
}

func generateManufacturer() *inventory_v1.Manufacturer {
	return &inventory_v1.Manufacturer{
		Name:    gofakeit.Name(),
		Country: gofakeit.Country(),
		Website: gofakeit.URL(),
	}
}

func generateTags() []string {
	var tags []string
	for i := 0; i < gofakeit.Number(1, 10); i++ {
		tags = append(tags, gofakeit.EmojiTag())
	}

	return tags
}

func generateMetadata() map[string]*inventory_v1.Value {
	metadata := make(map[string]*inventory_v1.Value)

	for i := 0; i < gofakeit.Number(1, 10); i++ {
		metadata[gofakeit.Word()] = generateMetadataValue()
	}

	return metadata
}

func generateMetadataValue() *inventory_v1.Value {
	switch gofakeit.Number(0, 3) {
	case 0:
		return &inventory_v1.Value{
			Kind: &inventory_v1.Value_StringValue{
				StringValue: gofakeit.Word(),
			},
		}

	case 1:
		return &inventory_v1.Value{
			Kind: &inventory_v1.Value_Int64Value{
				Int64Value: int64(gofakeit.Number(1, 100)),
			},
		}

	case 2:
		return &inventory_v1.Value{
			Kind: &inventory_v1.Value_DoubleValue{
				DoubleValue: roundTo(gofakeit.Float64Range(1, 100)),
			},
		}

	case 3:
		return &inventory_v1.Value{
			Kind: &inventory_v1.Value_BoolValue{
				BoolValue: gofakeit.Bool(),
			},
		}

	default:
		return nil
	}

}

func roundTo(x float64) float64 {
	return math.Round(x*100) / 100
}

func (s *InventoryServer) GetPart(_ context.Context, req *inventory_v1.GetPartRequest) (*inventory_v1.GetPartResponse, error) {
	part, ok := s.parts[req.Uuid]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "part with UUID %s not found", req.Uuid)
	}
	return &inventory_v1.GetPartResponse{Part: part}, nil
}

func (s *InventoryServer) ListParts(_ context.Context, req *inventory_v1.ListPartsRequest) (*inventory_v1.ListPartsResponse, error) {
	filter := req.GetFilter()
	result := make([]*inventory_v1.Part, 0, len(s.parts))

	uuidSet := make(map[string]struct{}, len(filter.GetUuids()))
	for _, partUuid := range filter.GetUuids() {
		uuidSet[partUuid] = struct{}{}
	}

	nameSet := make(map[string]struct{}, len(filter.GetNames()))
	for _, name := range filter.GetNames() {
		nameSet[strings.ToLower(name)] = struct{}{}
	}

	categorySet := make(map[inventory_v1.Category]struct{}, len(filter.GetCategories()))
	for _, category := range filter.GetCategories() {
		categorySet[category] = struct{}{}
	}

	countrySet := make(map[string]struct{}, len(filter.GetManufacturerCountries()))
	for _, country := range filter.GetManufacturerCountries() {
		countrySet[strings.ToLower(country)] = struct{}{}
	}
	tagSet := make(map[string]struct{}, len(filter.GetTags()))
	for _, tag := range filter.GetTags() {
		tagSet[strings.ToLower(tag)] = struct{}{}
	}

	for _, part := range s.parts {
		if len(uuidSet) > 0 {
			if _, ok := uuidSet[part.Uuid]; !ok {
				continue
			}
		}

		if len(nameSet) > 0 {
			if _, ok := nameSet[strings.ToLower(part.Name)]; !ok {
				continue
			}
		}

		if len(categorySet) > 0 {
			if _, ok := categorySet[part.Category]; !ok {
				continue
			}
		}

		if len(countrySet) > 0 {
			if _, ok := countrySet[strings.ToLower(part.Manufacturer.GetCountry())]; !ok {
				continue
			}
		}

		if len(tagSet) > 0 {
			foundTag := false
			for _, tag := range part.Tags {
				if _, ok := tagSet[strings.ToLower(tag)]; ok {
					foundTag = true
					break
				}
			}
			if !foundTag {
				continue
			}
		}

		result = append(result, part)
	}

	return &inventory_v1.ListPartsResponse{Parts: result}, nil
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
	inventory_v1.RegisterInventoryServiceServer(s, NewInventoryServer())

	go func() {
		log.Printf("🚀 Inventory gRPC server listening at %v", lis.Addr())
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
	log.Println("🛑 Inventory gRPC server shutting down...")
	s.GracefulStop()
	log.Println("✅ Inventory gRPC server stopped")
}
