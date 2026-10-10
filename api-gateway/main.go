// api-gateway is the single REST entry point. Postman talks only to this
// service; it forwards everything to equipment-service / booking-service
// over gRPC and translates gRPC status codes to HTTP status codes.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	bookingspb "equipbooking/proto/bookings"
	equipmentpb "equipbooking/proto/equipment"
)

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func grpcToHTTP(err error) (int, string) {
	st, ok := status.FromError(err)
	if !ok {
		return http.StatusInternalServerError, err.Error()
	}
	switch st.Code() {
	case codes.OK:
		return http.StatusOK, ""
	case codes.InvalidArgument:
		return http.StatusBadRequest, st.Message()
	case codes.NotFound:
		return http.StatusNotFound, st.Message()
	case codes.AlreadyExists:
		return http.StatusConflict, st.Message()
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout, st.Message()
	case codes.Unavailable:
		return http.StatusServiceUnavailable, st.Message()
	default:
		return http.StatusInternalServerError, st.Message()
	}
}

func writeGRPCError(w http.ResponseWriter, err error) {
	code, msg := grpcToHTTP(err)
	writeJSON(w, code, apiError{Error: msg})
}

func pathID(r *http.Request, prefix string) string {
	return strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, prefix), "/")
}

func equipmentHandler(client equipmentpb.EquipmentServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		id := pathID(r, "/equipment")

		switch {
		case r.Method == http.MethodPost && id == "":
			var req equipmentpb.CreateEquipmentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, apiError{Error: "invalid JSON body"})
				return
			}
			e, err := client.CreateEquipment(ctx, &req)
			if err != nil {
				writeGRPCError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, e)

		case r.Method == http.MethodGet && id == "":
			statusFilter := r.URL.Query().Get("status")
			res, err := client.ListEquipment(ctx, &equipmentpb.ListEquipmentRequest{Status: statusFilter})
			if err != nil {
				writeGRPCError(w, err)
				return
			}
			list := res.Equipment
			if list == nil {
				list = []*equipmentpb.Equipment{}
			}
			writeJSON(w, http.StatusOK, list)

		case r.Method == http.MethodGet && id != "":
			e, err := client.GetEquipment(ctx, &equipmentpb.GetEquipmentRequest{Id: id})
			if err != nil {
				writeGRPCError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, e)

		case r.Method == http.MethodPut && id != "":
			var req equipmentpb.UpdateEquipmentRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, apiError{Error: "invalid JSON body"})
				return
			}
			req.Id = id
			e, err := client.UpdateEquipment(ctx, &req)
			if err != nil {
				writeGRPCError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, e)

		case r.Method == http.MethodDelete && id != "":
			res, err := client.DeleteEquipment(ctx, &equipmentpb.DeleteEquipmentRequest{Id: id})
			if err != nil {
				writeGRPCError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, res)

		default:
			writeJSON(w, http.StatusMethodNotAllowed, apiError{Error: "method not allowed on this path"})
		}
	}
}

func bookingsHandler(client bookingspb.BookingServiceClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()

		id := pathID(r, "/bookings")

		switch {
		case r.Method == http.MethodPost && id == "":
			var req bookingspb.CreateBookingRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, apiError{Error: "invalid JSON body"})
				return
			}
			b, err := client.CreateBooking(ctx, &req)
			if err != nil {
				writeGRPCError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, b)

		case r.Method == http.MethodGet && id == "":
			statusFilter := r.URL.Query().Get("status")
			res, err := client.ListBookings(ctx, &bookingspb.ListBookingsRequest{Status: statusFilter})
			if err != nil {
				writeGRPCError(w, err)
				return
			}
			list := res.Bookings
			if list == nil {
				list = []*bookingspb.Booking{}
			}
			writeJSON(w, http.StatusOK, list)

		case r.Method == http.MethodGet && id != "":
			b, err := client.GetBooking(ctx, &bookingspb.GetBookingRequest{Id: id})
			if err != nil {
				writeGRPCError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, b)

		case r.Method == http.MethodPut && id != "":
			var req bookingspb.UpdateBookingRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, apiError{Error: "invalid JSON body"})
				return
			}
			req.Id = id
			b, err := client.UpdateBooking(ctx, &req)
			if err != nil {
				writeGRPCError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, b)

		case r.Method == http.MethodDelete && id != "":
			res, err := client.DeleteBooking(ctx, &bookingspb.DeleteBookingRequest{Id: id})
			if err != nil {
				writeGRPCError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, res)

		default:
			writeJSON(w, http.StatusMethodNotAllowed, apiError{Error: "method not allowed on this path"})
		}
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	port := getenv("GATEWAY_PORT", "8080")
	equipmentServiceAddr := getenv("EQUIPMENT_SERVICE_ADDR", "localhost:50051")
	bookingServiceAddr := getenv("BOOKING_SERVICE_ADDR", "localhost:50052")

	equipmentConn, err := grpc.NewClient(equipmentServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("could not create equipment-service client: %v", err)
	}
	bookingConn, err := grpc.NewClient(bookingServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("could not create booking-service client: %v", err)
	}

	equipmentClient := equipmentpb.NewEquipmentServiceClient(equipmentConn)
	bookingClient := bookingspb.NewBookingServiceClient(bookingConn)

	mux := http.NewServeMux()
	mux.HandleFunc("/equipment", equipmentHandler(equipmentClient))
	mux.HandleFunc("/equipment/", equipmentHandler(equipmentClient))
	mux.HandleFunc("/bookings", bookingsHandler(bookingClient))
	mux.HandleFunc("/bookings/", bookingsHandler(bookingClient))

	log.Printf("api-gateway listening on :%s (equipment: %s, bookings: %s)", port, equipmentServiceAddr, bookingServiceAddr)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
