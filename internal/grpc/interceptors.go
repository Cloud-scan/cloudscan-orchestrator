package grpc

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// loggingInterceptor logs all gRPC requests
func loggingInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()

		logger := log.WithFields(log.Fields{
			"method": info.FullMethod,
		})

		logger.Debug("gRPC request started")

		// Call the handler
		resp, err := handler(ctx, req)

		// Log the result
		duration := time.Since(start)
		logger = logger.WithField("duration_ms", duration.Milliseconds())

		if err != nil {
			st, _ := status.FromError(err)
			logger.WithFields(log.Fields{
				"error":  err.Error(),
				"code":   st.Code(),
				"status": st.Message(),
			}).Error("gRPC request failed")
		} else {
			logger.Info("gRPC request completed")
		}

		return resp, err
	}
}

// errorHandlingInterceptor handles panics and converts them to gRPC errors
func errorHandlingInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		// Recover from panics
		defer func() {
			if r := recover(); r != nil {
				log.WithFields(log.Fields{
					"method": info.FullMethod,
					"panic":  r,
				}).Error("Panic recovered in gRPC handler")

				err = status.Errorf(codes.Internal, "internal server error: %v", r)
			}
		}()

		return handler(ctx, req)
	}
}
