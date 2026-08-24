package grpc

import (
	"errors"

	"myGuy/internal/domain"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// toStatusError maps a domain error onto the gRPC status code that best
// describes it. This mapping is the transport layer's job precisely because
// gRPC codes are a wire-protocol concept: the services below know nothing
// about them, which is what would let the same services sit behind an HTTP
// or CLI front end unchanged.
func toStatusError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domain.ErrEmptyCredentials):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrUserExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, domain.ErrInvalidCredentials):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, domain.ErrNotLoggedIn):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, domain.ErrUserNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrIngestorStopped):
		// Unavailable tells a well-behaved client this is worth retrying
		// against another server, unlike an Internal error.
		return status.Error(codes.Unavailable, err.Error())
	default:
		// Anything unrecognised is an infrastructure failure, not something
		// the caller did wrong.
		return status.Error(codes.Internal, err.Error())
	}
}
